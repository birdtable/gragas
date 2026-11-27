package config_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/appdirs"

	"github.com/birdtable/gragas/internal/config"
)

// TestTheNamesFilesPathIsNeverInItsRefusal: a names-file key or a
// GRAGAS_NAMES_FILE that is not an absolute, clean path is refused, saying
// why from the closed set and naming the key or the variable, and no part of
// the value is in the refusal: the path can hold a login that is on the
// list. Each value carries the made-up folder zebra-hidden, which the
// refusal's every word, its next line included, must not hold. Planted: unclean's
// words back for the names file (the value quoted, red on the relative and
// the climbing rows).
func TestTheNamesFilesPathIsNeverInItsRefusal(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	home := map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}
	file := filepath.Join(folders(t, home).Config, "settings.yaml")
	values := map[string]string{
		"a relative path":           filepath.Join("zebra-hidden", "list.txt"),
		"a path that climbs":        filepath.Join(dir, "zebra-hidden") + "/../list.txt",
		"a path that starts with -": "-zebra-hidden",
		"a path with a newline":     filepath.Join(dir, "zebra-hidden", "list\nx"),
	}
	for name, v := range values {
		t.Run("the key, "+name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{file: fmt.Sprintf("version: 1\nnames-file: %q\n", v)}
			_, err := config.Load(env(home), "", reader(files, nil))
			refusedWithoutThePath(t, err, "names-file")
		})
		t.Run("the variable, "+name, func(t *testing.T) {
			t.Parallel()
			e := map[string]string{appdirs.HomeVar: home[appdirs.HomeVar], config.NamesFileVar: v}
			_, err := config.Load(env(e), "", reader(nil, nil))
			refusedWithoutThePath(t, err, config.NamesFileVar)
		})
	}
	t.Run("the key, a value that is no string", func(t *testing.T) {
		t.Parallel()
		files := map[string]string{file: "version: 1\nnames-file: [/work/zebra-hidden/list.txt]\n"}
		_, err := config.Load(env(home), "", reader(files, nil))
		refusedWithoutThePath(t, err, "names-file")
	})
	t.Run("the control, an absolute, clean path", func(t *testing.T) {
		t.Parallel()
		want := filepath.Join(dir, "zebra-hidden", "list.txt")
		e := map[string]string{appdirs.HomeVar: home[appdirs.HomeVar], config.NamesFileVar: want}
		c, err := config.Load(env(e), "", reader(nil, nil))
		if err != nil || c.NamesFile != want {
			t.Errorf("Load = %q, %v; want %q read", c.NamesFile, err, want)
		}
	})
}

// refusedWithoutThePath fails unless err is a refusal that names by, and no
// word of it, What or Next, holds the made-up folder.
func refusedWithoutThePath(t *testing.T, err error, by string) {
	t.Helper()
	p, ok := errors.AsType[*config.Problem](err)
	if !ok || p.Kind != config.Refused || !strings.Contains(p.What, by) {
		t.Fatalf("%v; want a refusal naming %s", err, by)
	}
	for what, words := range map[string]string{"the error": err.Error(), "What": p.What, "Next": p.Next} {
		if strings.Contains(words, "zebra-hidden") || strings.Contains(words, "list.txt") {
			t.Errorf("%s %q holds the path", what, words)
		}
	}
}
