package plugins_test

import (
	"context"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// The entries call leaves the files a read returned out of its Config, so a
// preset a read matched counts once against the entries cap (gragas#2375).

// reading is the runner of a read whose read steps return the files it holds.
type reading struct {
	*runner
	files []plugin.File
}

func (r reading) Run(ctx context.Context, round, i int, s plugin.Step) (plugin.Result, error) {
	got, err := r.runner.Run(ctx, round, i, s)
	if s.Read != nil {
		got.Files = r.files
	}
	return got, err
}

func text(s string) plugin.Bytes { return plugin.Bytes{Text: &s} }

func TestAFileAReadReturnedLeavesTheEntriesConfig(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a: 1\n", 40*1024/5)
	in := seed()
	in.Config = []plugin.Config{{Path: "lintel.config", Data: &plugin.Bytes{Text: &big}}, {Path: "other.yml", Data: &plugin.Bytes{Text: new("x")}}}
	script := []answer{steps(2, true)}
	tr := &trace{}
	r := reading{runner: &runner{tr: tr, script: script}, files: []plugin.File{{Path: "lintel.config", Data: text(big)}}}
	if _, err := plugins.Entries(t.Context(), scripted(t, tr, script, nil), r, in); err != nil {
		t.Fatal(err)
	}
	if len(tr.reads) != 1 {
		t.Fatalf("entries calls = %d, want 1", len(tr.reads))
	}
	cfg := tr.reads[0].Config
	if len(cfg) != 1 || cfg[0].Path != "other.yml" {
		t.Errorf("entries Config = %v, want only other.yml: the file the read returned is in have", cfg)
	}
	if len(tr.plans[0].Config) != 2 {
		t.Errorf("the plan was asked with %d config files, want both", len(tr.plans[0].Config))
	}
}

func TestAReadThatReturnedNoFileLeavesTheWholeConfig(t *testing.T) {
	t.Parallel()
	in := seed()
	in.Config = []plugin.Config{{Path: "lintel.config"}, {Path: "other.yml"}}
	script := []answer{steps(2, true)}
	tr := &trace{}
	r := reading{runner: &runner{tr: tr, script: script}}
	if _, err := plugins.Entries(t.Context(), scripted(t, tr, script, nil), r, in); err != nil {
		t.Fatal(err)
	}
	if got := len(tr.reads[0].Config); got != 2 {
		t.Errorf("entries Config holds %d files, want 2", got)
	}
}

func TestAPluginThatReadsNothingGetsTheWholeConfig(t *testing.T) {
	t.Parallel()
	in := seed()
	in.Config = []plugin.Config{{Path: "lintel.config"}, {Path: "other.yml"}}
	script := []answer{steps(0, true)}
	tr := &trace{}
	if _, err := plugins.Entries(t.Context(), scripted(t, tr, script, nil), &runner{tr: tr, script: script}, in); err != nil {
		t.Fatal(err)
	}
	if got := len(tr.reads[0].Config); got != 2 {
		t.Errorf("entries Config holds %d files, want 2", got)
	}
}
