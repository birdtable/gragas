package tool_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// ends is what f answers, failing t when it has not answered within a bound
// far past what a read of a few files takes: a read takes no ctx, so a read
// that waits is a run that never ends.
func ends(t *testing.T, what string, f func() ([]plugin.File, error)) ([]plugin.File, error) {
	t.Helper()
	type answer struct {
		files []plugin.File
		err   error
	}
	done := make(chan answer, 1)
	go func() { files, err := f(); done <- answer{files, err} }()
	select {
	case a := <-done:
		return a.files, a.err
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not end", what)
		return nil, nil
	}
}

// TestAGlobOfManyStarsEndsAtOnce: a glob of many stars against a long name, and of many ** against a
// deep path, that matches nothing ends at once, as the match goes back only
// to its last star; the same globs that end on the name do match it.
// Planted: partMatch trying every split at each * (the name rows never end,
// so red at the bound), and globMatch trying every split at each ** (the
// path rows red the same way).
func TestAGlobOfManyStarsEndsAtOnce(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 200)
	deep := strings.Repeat("a/", 40) + "c"
	_, root := scratchWith(t, map[string]string{long: "x", deep: "x"})
	stars := strings.Repeat("*a", 16)
	doubles := strings.Repeat("**/a/", 12)
	for _, c := range []struct {
		name, glob string
		want       []string
	}{
		{"many stars, then a letter the name lacks", stars + "b", nil},
		{"many stars that end on the name", stars, []string{long}},
		{"many ** then a name the path lacks", doubles + "b", nil},
		{"many ** that end on the path", doubles + "c", []string{deep}},
	} {
		got, err := ends(t, c.name, func() ([]plugin.File, error) { return tool.ReadGlob(root, c.glob) })
		if err != nil || len(got) != len(c.want) || (len(got) == 1 && got[0].Path != c.want[0]) {
			t.Errorf("%s: ReadGlob = %q, %v, want %q", c.name, paths(got), err, c.want)
		}
	}
}

// TestAPipeIsRefusedAndHoldsNoRead: a pipe in the scratch folder whose writer is held open, which a
// step's program may leave behind, is refused by a read that matches it and
// by a collect that names it, matching port.ErrRefused, and neither waits for
// its writer. Planted: the walk keeping what is no regular file (the read
// row red at the bound), the walk skipping it in place of refusing it (the
// read row red: no refusal), and readCapped reading what it opened with no
// look at its type (the collect row red at the bound).
func TestAPipeIsRefusedAndHoldsNoRead(t *testing.T) {
	t.Parallel()
	for name, read := range map[string]func(*os.Root, string) ([]plugin.File, error){
		"a read":    func(root *os.Root, base string) ([]plugin.File, error) { return tool.ReadGlob(root, "*") },
		"a collect": func(root *os.Root, base string) ([]plugin.File, error) { return tool.Collect(root, []string{base}) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, release := testenv.HoldRead(t)
			defer release()
			root, err := os.OpenRoot(filepath.Dir(p))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = root.Close() })
			got, err := ends(t, name, func() ([]plugin.File, error) { return read(root, filepath.Base(p)) })
			if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "no regular file") {
				t.Errorf("%s of a pipe = %q, %v, want refused as no regular file, matching port.ErrRefused", name, paths(got), err)
			}
		})
	}
}
