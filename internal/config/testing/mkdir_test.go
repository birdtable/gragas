package configtest

import (
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"testing"
)

// TestMkdirPrivateMakesEachMissingLevelPrivateAndKeepsWhatIsThere holds
// MkdirPrivate: a name of two levels makes both, each as private
// as the folder Private makes (go-files' own), a name already there is kept
// with what it holds, and the test names no mode.
// Planted failure: the helper makes no folder, makes only the last level, or
// makes the folder with os.MkdirAll.
func TestMkdirPrivateMakesEachMissingLevelPrivateAndKeepsWhatIsThere(t *testing.T) {
	t.Parallel()
	own, err := os.Stat(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := filestest.Private(t)
	got := MkdirPrivate(t, dir, filepath.Join(".tools", "bin"))
	if want := filepath.Join(dir, ".tools", "bin"); got != want {
		t.Errorf("MkdirPrivate answers %s, want %s", got, want)
	}
	for _, p := range []string{filepath.Join(dir, ".tools"), got} {
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			t.Fatalf("%s is not a folder: %v", p, err)
		}
		if st.Mode() != own.Mode() {
			t.Errorf("%s has mode %v, want the %v of a private folder", p, st.Mode(), own.Mode())
		}
	}
	filestest.WriteFile(t, filepath.Join(got, "kept"), []byte("x"))
	if again := MkdirPrivate(t, dir, ".tools"); again != filepath.Join(dir, ".tools") {
		t.Errorf("a second call answers %s", again)
	}
	if _, err := os.Stat(filepath.Join(got, "kept")); err != nil {
		t.Errorf("a name already there lost what it held: %v", err)
	}
}
