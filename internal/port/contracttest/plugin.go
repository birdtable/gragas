package contracttest

import (
	"embed"
	"errors"
	"io/fs"
	"path"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// pluginCases are the cases every implementation of port.Plugin that serves the
// standin's behaviour answers the same: one folder each,
// testdata/plugin/<n>-<op>-<what>, holding the request document as the wire
// sends it and the response document as the wire answers it. The in-memory
// fake, the Go source of standin.wasm called in-process, and the wasm host on
// the module built from it each run them.
//
//go:embed testdata/plugin
var pluginCases embed.FS

// PluginMaker makes the plugin for one case: a fresh opened plugin that is the
// standin or answers as it does.
type PluginMaker func(t *testing.T) port.Plugin

// Plugin runs the Plugin suite against the plugin build answers: for each case,
// the request is read into its typed form, the operation is called through the
// port, and the answer is held equal to the case's in canonical form, or, for an
// error, the plugin's own error has the case's code and path. A message is
// display only and is not compared.
func Plugin(t *testing.T, build PluginMaker) {
	t.Helper()
	dirs, err := fs.ReadDir(pluginCases, "testdata/plugin")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("the plugin cases are not there: %v", err)
	}
	for _, d := range dirs {
		t.Run(d.Name(), func(t *testing.T) {
			pluginCase(t, build(t), path.Join("testdata/plugin", d.Name()))
		})
	}
}

// member is the member k of o as a T, and T's zero value where it is none.
func member[T any](o canon.Object, k string) T {
	v, ok := o[k].(T)
	if !ok {
		var zero T
		return zero
	}
	return v
}

// pluginCase runs one case folder against p.
func pluginCase(t *testing.T, p port.Plugin, dir string) {
	t.Helper()
	req := pluginDoc(t, path.Join(dir, "request.json"))
	want := pluginDoc(t, path.Join(dir, "response.json"))
	op := plugin.Op(member[canon.String](req, "op"))
	got, err := pluginCall(t, p, op, member[canon.Object](req, "body"))
	if want["error"] != nil {
		wantErr := member[canon.Object](want, "error")
		var ce *plugin.CallError
		if !errors.As(err, &ce) {
			t.Fatalf("%s = %v, want the plugin's error %v", op, err, wantErr)
		}
		code, at := member[canon.String](wantErr, "code"), member[canon.String](wantErr, "path")
		if string(ce.Code) != string(code) || ce.Path != string(at) {
			t.Errorf("%s answered %s at %q, want %s at %q", op, ce.Code, ce.Path, code, at)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s = %v, want the answer of the case", op, err)
	}
	gotBytes, err1 := canon.Encode(got, canon.Canonical)
	wantBytes, err2 := canon.Encode(want["ok"], canon.Canonical)
	if err1 != nil || err2 != nil {
		t.Fatalf("the answers cannot be written: %v, %v", err1, err2)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Errorf("%s answered\n%s\nwant\n%s", op, gotBytes, wantBytes)
	}
}

// pluginDoc is the JSON object of a case file.
func pluginDoc(t *testing.T, name string) canon.Object {
	t.Helper()
	b, err := pluginCases.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	v, err := canon.Decode(b)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	o, ok := v.(canon.Object)
	if !ok {
		t.Fatalf("%s is no object", name)
	}
	return o
}

// pluginCall calls op on p with the typed form of body and answers the
// response's canonical tree.
func pluginCall(t *testing.T, p port.Plugin, op plugin.Op, body canon.Object) (canon.Value, error) {
	t.Helper()
	ctx := t.Context()
	read := func(x interface{ FromCanon(canon.Value) error }) {
		t.Helper()
		if err := x.FromCanon(body); err != nil {
			t.Fatalf("the request of the case is no %s request: %v", op, err)
		}
	}
	switch op {
	case plugin.OpPlan:
		var in plugin.PlanRequest
		read(&in)
		r, err := p.Plan(ctx, in)
		return r.ToCanon(), err
	case plugin.OpEntries:
		var in plugin.EntriesRequest
		read(&in)
		r, err := p.Entries(ctx, in)
		return r.ToCanon(), err
	case plugin.OpFindings:
		var in plugin.FindingsRequest
		read(&in)
		r, err := p.Findings(ctx, in)
		return r.ToCanon(), err
	case plugin.OpRank:
		var in plugin.RankRequest
		read(&in)
		r, err := p.Rank(ctx, in)
		return r.ToCanon(), err
	case plugin.OpPreset:
		var in plugin.PresetRequest
		read(&in)
		r, err := p.Preset(ctx, in)
		return r.ToCanon(), err
	}
	t.Fatalf("the case names the operation %q, which is none", op)
	return nil, nil
}
