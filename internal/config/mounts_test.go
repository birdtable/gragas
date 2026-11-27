package config_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/appdirs"

	"github.com/birdtable/gragas/internal/config"
)

// TestTheMountsAreTheVariablesOverTheKeysAndTheFilesAreReadWithTheVariableAlone:
// the mounts key's folders, else GRAGAS_MOUNTS's over them; the
// settings and tools files are read with the variable's alone, since the key is
// in the file the read would let in; and an entry of the variable that is not
// an absolute, clean path, an empty one among them, refuses the run by name
// before any file is read. Planted: the key handed to the read; the variable
// passed over; a bad entry left out in silence.
func TestTheMountsAreTheVariablesOverTheKeysAndTheFilesAreReadWithTheVariableAlone(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	home := filepath.Join(dir, "h")
	// appdirs' answer for the config folder, never a shape written here:
	// .config/gragas is one system's, and another's reads elsewhere.
	cfg := folders(t, map[string]string{appdirs.HomeVar: home}).Config
	file := filepath.Join(cfg, "settings.yaml")
	tools := filepath.Join(cfg, "tools.yaml")
	key, a, b := filepath.Join(dir, "srv", "key"), filepath.Join(dir, "srv", "a"), filepath.Join(dir, "srv", "b")
	files := map[string]string{file: "version: 1\nmounts: [" + filepath.ToSlash(key) + "]\n", tools: "version: 1\n"}
	read := func(seen map[string][]string) func(string, []string) ([]byte, error) {
		return func(p string, mounts []string) ([]byte, error) {
			seen[p] = mounts
			if data, ok := files[p]; ok {
				return []byte(data), nil
			}
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		}
	}

	seen := map[string][]string{}
	c, err := config.Load(env(map[string]string{appdirs.HomeVar: home}), "", read(seen))
	if err != nil || !slices.Equal(c.Mounts, []string{key}) {
		t.Errorf("the key alone: mounts %q, %v; want [%s]", c.Mounts, err, key)
	}
	if len(seen) != 2 || seen[file] != nil || seen[tools] != nil {
		t.Errorf("the key alone: the files were read with %q; want both read, with no mounts", seen)
	}

	seen = map[string][]string{}
	both := a + string(filepath.ListSeparator) + b
	c, err = config.Load(env(map[string]string{appdirs.HomeVar: home, config.MountsVar: both}), "", read(seen))
	if err != nil || !slices.Equal(c.Mounts, []string{a, b}) {
		t.Errorf("the variable: mounts %q, %v; want [%s %s] over the key", c.Mounts, err, a, b)
	}
	if !slices.Equal(seen[file], []string{a, b}) || !slices.Equal(seen[tools], []string{a, b}) {
		t.Errorf("the variable: the files were read with %q; want the variable's mounts for each", seen)
	}

	sep := string(filepath.ListSeparator)
	for _, tc := range []struct{ name, value, says string }{
		{"a relative entry", a + sep + "srv", `"srv"`},
		{"an unclean entry", a + "/../b", "clean"},
		{"an empty entry", a + sep + sep + b, "empty entry"},
		{"a stray separator", a + sep, "empty entry"},
		{"a dash", "-" + a, "absolute"},
	} {
		seen = map[string][]string{}
		_, err := config.Load(env(map[string]string{appdirs.HomeVar: home, config.MountsVar: tc.value}), "", read(seen))
		p, ok := errors.AsType[*config.Problem](err)
		if !ok || p.Kind != config.Refused || !strings.HasPrefix(p.What, config.MountsVar+" ") || !strings.Contains(p.What, tc.says) {
			t.Errorf("%s, %q: %v; want refused, naming %s and saying %s", tc.name, tc.value, err, config.MountsVar, tc.says)
		}
		if len(seen) != 0 {
			t.Errorf("%s: the files were read with %q before the refusal", tc.name, seen)
		}
	}
}
