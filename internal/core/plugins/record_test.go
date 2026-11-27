package plugins_test

import (
	"context"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// The recorder: what a read leaves behind, with the scratch
// folder rewritten, and the raw files of it.

// outRunner gives each run step stdout that names the scratch folder.
type outRunner struct{ runner }

func (o *outRunner) Run(_ context.Context, round, i int, s plugin.Step) (plugin.Result, error) {
	if s.Run == nil {
		return plugin.Result{Files: []plugin.File{{Path: "a/b.txt", Data: plugin.Bytes{Text: new("in scratch/a")}}}}, nil
	}
	code := 3
	return plugin.Result{
		Exit:   &code,
		Stdout: &plugin.Bytes{Text: new("wrote scratch/out")},
		Stderr: &plugin.Bytes{Text: new("")},
	}, nil
}

func TestRecordKeepsEachStepsResultAndRewritesTheScratchFolder(t *testing.T) {
	t.Parallel()
	script := []answer{steps(2, true)}
	tr := &trace{}
	r := &outRunner{runner{tr: tr, script: script}}
	rec, err := plugins.Record(t.Context(), scripted(t, tr, script, nil), r, seed(), plugin.OpEntries)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Op != plugin.OpEntries || len(rec.Results) != 2 {
		t.Fatalf("recorded %v with %d results", rec.Op, len(rec.Results))
	}
	for i, res := range rec.Results {
		if res.Round != 0 || res.Step != i || res.Kind != kindAt(i) {
			t.Errorf("result %d sits at %d.%d as %s", i, res.Round, res.Step, res.Kind)
		}
	}
	if got := *rec.Results[0].Stdout.Text; got != "wrote "+plugins.ScratchWord+"/out" {
		t.Errorf("stdout is %q", got)
	}
	if got := *rec.Results[1].Files[0].Data.Text; got != "in "+plugins.ScratchWord+"/a" {
		t.Errorf("file is %q", got)
	}
}

func TestRecordOfFindingsIsTheFindingsCall(t *testing.T) {
	t.Parallel()
	script := []answer{steps(0, true)}
	tr := &trace{}
	rec, err := plugins.Record(t.Context(), scripted(t, tr, script, nil), &runner{tr: tr, script: script}, seed(), plugin.OpFindings)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Op != plugin.OpFindings || tr.calls("read") != 1 || len(tr.found) != 1 || len(tr.reads) != 0 {
		t.Fatalf("recorded %v: %s", rec.Op, tr)
	}
	if rec.Response.Ok["ran"] == nil {
		t.Errorf("the answer is %v", rec.Response.Ok)
	}
}

func TestRecordOfAnUnknownOperationReadsNothing(t *testing.T) {
	t.Parallel()
	tr := &trace{}
	_, err := plugins.Record(t.Context(), scripted(t, tr, nil, nil), &runner{tr: tr}, seed(), plugin.Op("config"))
	if err == nil || !strings.Contains(err.Error(), "entries or findings") {
		t.Fatalf("answered %v", err)
	}
	if len(tr.events) != 0 {
		t.Errorf("the plugin was called: %s", tr)
	}
}

func TestRecordOfAReadThatDoesNotFinishRecordsNothing(t *testing.T) {
	t.Parallel()
	script := []answer{steps(0, true)}
	tr := &trace{}
	rec, err := plugins.Record(t.Context(), scripted(t, tr, script, refused()), &runner{tr: tr, script: script}, seed(), plugin.OpEntries)
	ends(t, err, plugins.Failed)
	if !reflect.DeepEqual(rec, plugins.Recorded{}) {
		t.Errorf("recorded %v", rec)
	}
}

func TestRawNamesEachFileByTheStepThatGaveIt(t *testing.T) {
	t.Parallel()
	zero, one := 0, 1
	b64 := base64.StdEncoding.EncodeToString([]byte{0, 1, 2})
	got, err := plugins.Raw([]plugin.Result{
		{Round: 0, Step: 0, Kind: plugin.KindRun, Exit: &zero, Stdout: &plugin.Bytes{Text: new("out")}, Stderr: &plugin.Bytes{Text: new("")}},
		{Round: 0, Step: 1, Kind: plugin.KindRead, Files: []plugin.File{{Path: "d/f", Data: plugin.Bytes{Base64: &b64}}}},
		{Round: 1, Step: 0, Kind: plugin.KindRun, Exit: &one, Stdout: &plugin.Bytes{Text: new("")}, Stderr: &plugin.Bytes{Text: new("bad")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	data := map[string]string{}
	for _, f := range got {
		names = append(names, f.Path)
		data[f.Path] = string(f.Data)
	}
	want := []string{"0-0.stdout", "0-0.exit", "0-1.files/d/f", "1-0.stdout", "1-0.stderr", "1-0.exit"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("files %v, want %v", names, want)
	}
	if data["0-1.files/d/f"] != "\x00\x01\x02" || data["1-0.exit"] != "1\n" || data["1-0.stderr"] != "bad" {
		t.Errorf("contents %q", data)
	}
}

func TestRawRefusesBytesThatAreNotBase64(t *testing.T) {
	t.Parallel()
	bad := "!!"
	_, err := plugins.Raw([]plugin.Result{{Kind: plugin.KindRun, Stdout: &plugin.Bytes{Base64: &bad}}})
	if err == nil || !strings.Contains(err.Error(), "stdout of step 0-0") {
		t.Fatalf("answered %v", err)
	}
}
