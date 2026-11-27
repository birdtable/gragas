package plugins

// The recorder. A recording is one read, run
// through the same rounds as every other read, with a tap on the Runner that
// keeps what each step gave. What the tap kept is the raw output a bundle files
// under testdata/recordings, and, with the request the reading call was made
// with and the answer it gave, the case a bundle files under testdata/cases.
// The scratch folder is rewritten to a fixed word in all of them, so no
// machine path is checked in and a replay sees the same word the plugin was
// given.

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// ScratchWord is what the scratch folder is rewritten to in a recorded case
// and in the raw output beside it.
const ScratchWord = "/scratch"

// Recorded is one read as the recorder kept it, with the scratch folder
// already rewritten to ScratchWord.
type Recorded struct {
	// Op is the reading call, entries or findings.
	Op plugin.Op
	// Request is the body of the reading call: the tool version, the scratch
	// word, the config and every result the steps gave.
	Request canon.Object
	// Response is the plugin's answer to it.
	Response plugin.Response
	// Results are what each step gave, in order, as the request's have holds
	// them.
	Results []plugin.Result
}

// tap is a Runner that keeps what each step of the read gave.
type tap struct {
	Runner
	got []plugin.Result
}

func (t *tap) Run(ctx context.Context, round, step int, s plugin.Step) (plugin.Result, error) {
	res, err := t.Runner.Run(ctx, round, step, s)
	if err == nil {
		// The place of a result is the engine's to say, as runAll says it.
		res.Round, res.Step, res.Kind = round, step, kindOf(s)
		t.got = append(t.got, res)
	}
	return res, err
}

// Record reads entries (op entries) or findings (op findings) as Entries and
// Findings do, through the same rounds, and answers what the read gave and the
// case it makes. in is the first plan's request, as those take it. A read that
// does not finish is the error they answer, and nothing is recorded.
func Record(ctx context.Context, p port.Plugin, r Runner, in plugin.PlanRequest, op plugin.Op) (Recorded, error) {
	t := &tap{Runner: r}
	var (
		body canon.Value
		ok   canon.Value
	)
	switch op {
	case plugin.OpEntries:
		got, err := Entries(ctx, p, t, in)
		if err != nil {
			return Recorded{}, err
		}
		in.For = plugin.ForConfig
		body, ok = entriesRequest(in, t.got).ToCanon(), got.ToCanon()
	case plugin.OpFindings:
		got, err := Findings(ctx, p, t, in)
		if err != nil {
			return Recorded{}, err
		}
		in.For = plugin.ForRun
		body, ok = findingsRequest(in, t.got).ToCanon(), got.ToCanon()
	default:
		return Recorded{}, fmt.Errorf("%q is not an operation a recording reads: entries or findings", op)
	}
	scratch := ""
	if in.Scratch != nil {
		scratch = *in.Scratch
	}
	body = cut(body, scratch)
	bo, bok := body.(canon.Object)
	oo, ook := cut(ok, scratch).(canon.Object)
	if !bok || !ook {
		return Recorded{}, fmt.Errorf("the %s call's request and answer are not objects", op)
	}
	rec := Recorded{Op: op, Request: bo, Response: plugin.Response{Ok: oo}}
	for _, res := range t.got {
		rec.Results = append(rec.Results, cutResult(res, scratch))
	}
	return rec, nil
}

// cutResult is res with the scratch folder rewritten in every text it holds.
func cutResult(res plugin.Result, scratch string) plugin.Result {
	out := res
	out.Stdout, out.Stderr = cutBytes(res.Stdout, scratch), cutBytes(res.Stderr, scratch)
	if res.Files != nil {
		out.Files = make([]plugin.File, len(res.Files))
		for i, f := range res.Files {
			out.Files[i] = plugin.File{Path: f.Path, Data: *cutBytes(&f.Data, scratch)}
		}
	}
	return out
}

// cutBytes is b with the scratch folder rewritten in its text.
func cutBytes(b *plugin.Bytes, scratch string) *plugin.Bytes {
	if b == nil || scratch == "" {
		return b
	}
	if b.Text != nil {
		s := strings.ReplaceAll(*b.Text, scratch, ScratchWord)
		return &plugin.Bytes{Text: &s}
	}
	// Bytes that are not text are not read for a path: a path in them is
	// not one a plugin cuts.
	return b
}

// cut is v with the scratch folder rewritten in each string of it.
func cut(v canon.Value, scratch string) canon.Value {
	if scratch == "" {
		return v
	}
	switch x := v.(type) {
	case canon.String:
		return canon.String(strings.ReplaceAll(string(x), scratch, ScratchWord))
	case canon.Array:
		out := make(canon.Array, len(x))
		for i, e := range x {
			out[i] = cut(e, scratch)
		}
		return out
	case canon.Object:
		out := make(canon.Object, len(x))
		for k, e := range x {
			out[k] = cut(e, scratch)
		}
		return out
	}
	return v
}

// RawFile is one file of raw tool output, by its path below the folder of
// the tool version it was recorded at.
type RawFile struct {
	Path string
	Data []byte
}

// Raw is the raw output of the steps of a recording, as files: a run's stdout
// and stderr (the stderr only when it holds something) and exit code, and the
// files a step gave, each under <round>-<step>.files. The scratch folder is
// already rewritten to ScratchWord.
func Raw(results []plugin.Result) ([]RawFile, error) {
	var out []RawFile
	for _, res := range results {
		stem := fmt.Sprintf("%d-%d", res.Round, res.Step)
		if res.Kind == plugin.KindRun {
			stdout, err := bytesOf(res.Stdout)
			if err != nil {
				return nil, fmt.Errorf("the stdout of step %s: %w", stem, err)
			}
			out = append(out, RawFile{stem + ".stdout", stdout})
			stderr, err := bytesOf(res.Stderr)
			if err != nil {
				return nil, fmt.Errorf("the stderr of step %s: %w", stem, err)
			}
			if len(stderr) > 0 {
				out = append(out, RawFile{stem + ".stderr", stderr})
			}
			if res.Exit != nil {
				out = append(out, RawFile{stem + ".exit", []byte(fmt.Sprintf("%d\n", *res.Exit))})
			}
		}
		for _, f := range res.Files {
			data, err := bytesOf(&f.Data)
			if err != nil {
				return nil, fmt.Errorf("the file %s of step %s: %w", f.Path, stem, err)
			}
			out = append(out, RawFile{stem + ".files/" + f.Path, data})
		}
	}
	return out, nil
}

// bytesOf is the bytes b holds: its text, or its base64 decoded. None is empty.
func bytesOf(b *plugin.Bytes) ([]byte, error) {
	switch {
	case b == nil:
		return nil, nil
	case b.Text != nil:
		return []byte(*b.Text), nil
	case b.Base64 != nil:
		return base64.StdEncoding.DecodeString(*b.Base64)
	}
	return nil, nil
}
