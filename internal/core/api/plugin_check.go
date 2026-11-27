package coreapi

// The operation behind gragas plugin test: it reads one plugin bundle through
// the port and holds it to checks C1 to C23 of the plugin design. The checks
// are core/plugintest's; the contract number, the features the engine speaks
// and the codec that writes each request and reads each answer are
// core/wire's, and the two contexts meet here. The module is the host's to
// try (port.Plugins.Try), this operation's alone: it is run in
// the sandbox with no approval, and nothing it answers is run.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/core/plugintest"
	"github.com/birdtable/gragas/internal/core/wire"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// PluginTest reads the bundle in dir, plugins/<name>/<version>, and runs the
// checks over it. It reads and writes nothing else. An error is a folder that
// cannot be opened or a file that cannot be read; a bundle that fails a check
// is no error, it is the answer.
func PluginTest(ctx context.Context, p port.Ports, dir string) (_ model.PluginTest, err error) {
	dir = path.Clean(dir)
	out := model.PluginTest{Schema: model.PluginTestSchema, Bundle: dir, Cases: []string{}, Checks: []model.PluginCheck{}}
	root, err := p.Tree.Open(dir)
	if err != nil {
		return out, fmt.Errorf("the bundle %s could not be opened: %w", dir, err)
	}
	b := plugintest.Bundle{Version: path.Base(dir), Name: path.Base(path.Dir(dir))}
	if b.Manifest, err = readOptional(p, root, "plugin.yaml"); err != nil {
		return out, err
	}
	if b.Wasm, err = readOptional(p, root, "plugin.wasm"); err != nil {
		return out, err
	}
	b.HasWasm = b.Wasm != nil
	if b.Cases, err = pluginCases(p, root, dir); err != nil {
		return out, err
	}
	for _, c := range b.Cases {
		out.Cases = append(out.Cases, c.Name)
	}
	if b.Recordings, err = pluginRecordings(p, root, dir); err != nil {
		return out, err
	}
	if b.HasWasm {
		trial, refused, err := tryModule(ctx, p, b)
		if err != nil {
			return out, err
		}
		b.Refused = refused
		if trial != nil {
			defer func() { err = errors.Join(err, trial.Close()) }()
			b.Plugin = trial
		}
	}
	codec := wire.New(p.FileNames)
	e := plugintest.Engine{Contract: wire.Contract, Features: wire.Features(), Ends: rangeEnds, Encode: codec.EncodeRequest, Rules: rulesOf(codec)}
	out.Checks = plugintest.Run(ctx, e, b)
	if err := ctx.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// tryModule has the host try the bundle's module (port.Plugins.Try) with its
// manifest as C1 reads it, the zero manifest where C1 reads none: the trial,
// or, where the host refused the module (port.ErrModule), why, which is C5's
// answer. Nothing a trial answers is run here: plugin test runs no step. Any
// other error is the operation's.
func tryModule(ctx context.Context, p port.Ports, b plugintest.Bundle) (port.Trial, string, error) {
	m, _ := plugintest.ManifestOf(b.Manifest)
	t, err := p.Plugins.Try(ctx, m, b.Wasm)
	switch {
	case errors.Is(err, port.ErrModule):
		return nil, err.Error(), nil
	case err != nil:
		return nil, "", fmt.Errorf("the module of the bundle could not be tried: %w", err)
	}
	return t, "", nil
}

// rulesOf is the codec's reading of a call's answer, as a typed call of r
// reads it: C9's and C13's rules, past the response schema (C8).
func rulesOf(c *wire.Codec) func(r plugin.Request, answer []byte, grants []string) error {
	return func(r plugin.Request, answer []byte, grants []string) error {
		var err error
		switch r.Op {
		case plugin.OpPlan:
			var in plugin.PlanRequest
			if err = in.FromCanon(r.Body); err == nil {
				_, err = c.DecodePlan(answer, in.Round, grants)
			}
		case plugin.OpEntries:
			_, err = c.DecodeEntries(answer)
		case plugin.OpFindings:
			_, err = c.DecodeFindings(answer)
		case plugin.OpRank:
			var in plugin.RankRequest
			if err = in.FromCanon(r.Body); err == nil {
				_, err = c.DecodeRank(answer, len(in.Pairs))
			}
		case plugin.OpPreset:
			var in plugin.PresetRequest
			if err = in.FromCanon(r.Body); err == nil {
				mark := ""
				if in.Mark != nil {
					mark = *in.Mark
				}
				_, err = c.DecodePreset(answer, mark)
			}
		default:
			err = fmt.Errorf("the request asks for %s, which is no operation", plugin.Shown(string(r.Op)))
		}
		return err
	}
}

// pluginCases reads testdata/cases: one folder to a case, by name.
func pluginCases(p port.Ports, root model.Root, dir string) ([]plugintest.Case, error) {
	listed, err := p.Tree.List(root, "testdata/cases", model.Every)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("testdata/cases of %s could not be listed: %w", dir, err)
	}
	have := map[string]bool{}
	var names []string
	for _, e := range listed {
		rel := strings.TrimPrefix(e.Path, "testdata/cases/")
		name, file, ok := strings.Cut(rel, "/")
		if e.Dir || !ok {
			continue
		}
		if !have[name] {
			names = append(names, name)
		}
		have[name+"/"+file] = true
		have[name] = true
	}
	slices.Sort(names)
	var cases []plugintest.Case
	for _, name := range names {
		c := plugintest.Case{Name: name}
		switch {
		case !have[name+"/request.json"]:
			c.Missing = "request.json"
		case !have[name+"/response.json"]:
			c.Missing = "response.json"
		default:
			if c.Request, err = p.Tree.Read(root, "testdata/cases/"+name+"/request.json"); err != nil {
				return nil, fmt.Errorf("%s/request.json could not be read: %w", name, err)
			}
			if c.Response, err = p.Tree.Read(root, "testdata/cases/"+name+"/response.json"); err != nil {
				return nil, fmt.Errorf("%s/response.json could not be read: %w", name, err)
			}
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// pluginRecordings lists testdata/recordings: one folder to a tool version,
// each holding the files of raw tool output.
func pluginRecordings(p port.Ports, root model.Root, dir string) ([]plugintest.Recording, error) {
	listed, err := p.Tree.List(root, "testdata/recordings", model.Every)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("testdata/recordings of %s could not be listed: %w", dir, err)
	}
	var out []plugintest.Recording
	for _, e := range listed {
		version, file, ok := strings.Cut(strings.TrimPrefix(e.Path, "testdata/recordings/"), "/")
		if !ok || e.Dir {
			continue
		}
		i := slices.IndexFunc(out, func(r plugintest.Recording) bool { return r.Version == version })
		if i < 0 {
			out = append(out, plugintest.Recording{Version: version})
			i = len(out) - 1
		}
		out[i].Files = append(out[i].Files, file)
	}
	return out, nil
}

// nearby is the bound itself and the versions one patch (or, at patch 0, one
// minor or major) above and below it: the candidates a range may serve at that
// end, of which the caller keeps those the range holds.
func nearby(v string) []string {
	var n [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return []string{v}
	}
	for i, p := range parts {
		if _, err := fmt.Sscanf(p, "%d", &n[i]); err != nil {
			return []string{v}
		}
	}
	out := []string{v, fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2]+1)}
	switch {
	case n[2] > 0:
		out = append(out, fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2]-1))
	case n[1] > 0:
		out = append(out, fmt.Sprintf("%d.%d.999", n[0], n[1]-1))
	case n[0] > 0:
		out = append(out, fmt.Sprintf("%d.999.999", n[0]-1))
	}
	return out
}

// rangeEnds is, for a tool version range, the versions it serves at its ends
// and versions just outside it: the bound itself where the range leaves it
// out, else a version far below or above.
func rangeEnds(spec string) (served, outside []string, err error) {
	vr, err := link.ParseRange(spec)
	if err != nil {
		return nil, nil, err
	}
	for _, s := range []struct {
		bound model.VersionBound
		far   string
	}{{vr.Lower, "0.0.0"}, {vr.Upper, "999999.0.0"}} {
		v := s.bound.Version
		if v == "" {
			continue
		}
		for _, c := range nearby(v) {
			if in, err := link.InRange(vr, c); err == nil && in && !slices.Contains(served, c) {
				served = append(served, c)
			}
		}
		if !s.bound.Included {
			outside = append(outside, v)
		}
		if in, err := link.InRange(vr, s.far); err == nil && !in {
			outside = append(outside, s.far)
		}
	}
	return served, outside, nil
}

// readOptional is the file at name, or nil when the bundle has none. An empty
// file is a non-nil empty slice, so a file that is there is told from one that
// is not.
func readOptional(p port.Ports, root model.Root, name string) ([]byte, error) {
	data, err := p.Tree.Read(root, name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%s could not be read: %w", name, err)
	case data == nil:
		return []byte{}, nil
	}
	return data, nil
}
