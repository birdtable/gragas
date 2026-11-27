package config_test

import (
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
)

// TestAToolPathIsCheckedAsWrittenAndHeldInTheSlashForm: a root and a tool's
// path are absolute and clean as the person writes them, by the system's own
// rules, and are held in the slash form the Steps port compares in, the form of
// a model.Root. One that is not absolute, not clean or climbs out with
// .. is still refused, whatever form it is held in. Risk: a native row that
// never matches its root, or a spelling that reaches outside the root
// unchecked.
func TestAToolPathIsCheckedAsWrittenAndHeldInTheSlashForm(t *testing.T) {
	t.Parallel()
	root, tool := filepath.Join(t.TempDir(), "app"), filepath.Join(t.TempDir(), "bin", "lint")
	s, err := config.Parse("/repo/c/settings.yaml", []byte("version: 1\ntool-paths:\n  '"+root+"':\n    lint@1.2.3: '"+tool+"'\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]string{filepath.ToSlash(root): {"lint@1.2.3": filepath.ToSlash(tool)}}
	if !reflect.DeepEqual(s.ToolPaths, want) {
		t.Errorf("tool-paths %v, want %v", s.ToolPaths, want)
	}
	for name, bad := range map[string]string{
		"a relative path":  "bin/lint",
		"a path with ..":   tool + "/" + ".." + "/" + "x",
		"an unclean path":  tool + "//x",
		"a path to a flag": "-lint",
	} {
		_, err := config.Parse("/repo/c/settings.yaml", []byte("version: 1\ntool-paths:\n  '"+root+"':\n    lint@1.2.3: '"+bad+"'\n"))
		if err == nil || !strings.Contains(err.Error(), "path") {
			t.Errorf("%s: %v; want a refusal naming the path", name, err)
		}
	}
}

// TestAProgramInTheModelFormIsClean: a setting that names a path is in the
// model's slash form, so it is clean when it is its own slash form, on every
// system; one that climbs with .. or doubles a slash is refused all the same.
// Risk: Clean's backslash on Windows refusing every slash-form
// setting, or a slash form letting a climb through.
func TestAProgramInTheModelFormIsClean(t *testing.T) {
	t.Parallel()
	slash := filepath.ToSlash(filepath.Join(t.TempDir(), "usr", "git"))
	for name, c := range map[string]struct {
		path string
		ok   bool
	}{
		"the slash form":    {slash, true},
		"the system's form": {filepath.FromSlash(slash), true},
		"a climb":           {slash + "/../x", false},
		"a doubled slash":   {path.Dir(slash) + "//" + path.Base(slash), false},
	} {
		_, err := config.Parse("/repo/c/settings.yaml", []byte("version: 1\nprograms: {git: '"+c.path+"'}\n"))
		if (err == nil) != c.ok {
			t.Errorf("%s: programs.git %q: %v; want accepted %v", name, c.path, err, c.ok)
		}
	}
}
