package importer_test

import (
	"context"
	"flag"
	"os"
	"reflect"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/importer"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain registers -update, which this package keeps no goldens for, so
// go test -update over internal/cli runs here too, and starts from the
// shared test setup.
func TestMain(m *testing.M) {
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "write the goldens: this package keeps none")
	}
	os.Exit(configtest.Main(m))
}

// session keeps the request the word hands the run, and plans nothing.
type session struct {
	kit.Session // every other method is the run's, which these tests never reach
	got         *model.ImportInput
}

func (s *session) Import(_ context.Context, _ kit.Cmd, in model.ImportInput) int {
	s.got = &in
	return 0
}

func (s *session) Exit(int) error { return nil }

func exec(t *testing.T, s *session, args ...string) error {
	t.Helper()
	c := importer.New(s)
	c.SetArgs(append([]string{}, args...)) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	return c.ExecuteContext(context.Background())
}

// TestImportHandsTheRunWhatWasGiven: the folder and each flag land in their
// field of the request, a config flag repeats, and a bare import hands the
// empty request for the run to ask. Risk: a flag wired to the wrong field.
func TestImportHandsTheRunWhatWasGiven(t *testing.T) {
	t.Parallel()
	s := &session{}
	err := exec(t, s, "docs/guide", "--into-part", "go", "--into-concern", "style", "--guide-slug", "handlers", "--guide-title", "Handler guide",
		"--guide-url", "https://example.org/handlers", "--guide-tag", "v1.2.0", "--guide-attribution", "the authors",
		"--tool", "mytool", "--target", "source", "--config", "a.yml", "--config", "b.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := model.ImportInput{Part: "go", Concern: "style", GuideDir: "docs/guide", Tool: "mytool", Target: "source", Configs: []string{"a.yml", "b.yml"},
		Guide: model.GuideRef{Slug: "handlers", Title: "Handler guide", URL: "https://example.org/handlers", Tag: "v1.2.0", Attribution: "the authors"}}
	if s.got == nil || !reflect.DeepEqual(*s.got, want) {
		t.Errorf("request %+v, want %+v", s.got, want)
	}
	bare := &session{}
	if err := exec(t, bare); err != nil {
		t.Fatal(err)
	}
	if bare.got == nil || !reflect.DeepEqual(*bare.got, model.ImportInput{}) {
		t.Errorf("a bare import hands %+v, want the empty request", bare.got)
	}
}
