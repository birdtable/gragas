package model_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// Every spelling a file system gragas runs on opens a reserved folder by is
// the same name, and a name that only looks near is not.
func TestSameNameFoldsASCIIAndDropsTrailingDots(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		step, name string
		want       bool
	}{
		{".git", ".git", true},
		{".GIT", ".git", true},
		{".Git", ".git", true},
		{".gIt", ".git", true},
		{".giT", ".git", true},
		{".git.", ".git", true},
		{".GIT.", ".git", true},
		{".git..", ".git", true},
		{".Git...", ".git", true},
		{".bin", ".bin", true},
		{".BIN", ".bin", true},
		{".Bin", ".bin", true},
		{".bin.", ".bin", true},
		{".Bin..", ".bin", true},
		// The reserved name may be the one spelled oddly.
		{".git", ".GIT.", true},

		// Near, and not reserved.
		{".gitignore", ".git", false},
		{".github", ".git", false},
		{".gitmodules", ".git", false},
		{".git-old", ".git", false},
		{".git_", ".git", false},
		{".gi", ".git", false},
		{"git", ".git", false},
		{"git.", ".git", false},
		{"..git", ".git", false},
		{"x.git", ".git", false},
		{".git.d", ".git", false},
		{".bin2", ".bin", false},
		{"bin", ".bin", false},
		{".bind", ".bin", false},
		{".git", ".bin", false},
		// A letter outside ASCII is not folded: U+0131, the dotless i, is
		// another name here.
		{".gıt", ".git", false},
		{".GİT", ".git", false},
		// Nor is a letter Unicode folds to an ASCII one: the Kelvin sign is not
		// k, and the long s is not s.
		{".\u212ait", ".kit", false},
		{".\u017fet", ".set", false},
		// Only dots are dropped.
		{".git ", ".git", false},
		{".git/", ".git", false},
		{".", ".git", false},
	} {
		if got := model.SameName(row.step, row.name); got != row.want {
			t.Errorf("SameName(%q, %q) is %v, want %v", row.step, row.name, got, row.want)
		}
		if got := model.SameName(row.name, row.step); got != row.want {
			t.Errorf("SameName(%q, %q) is %v, want %v: the compare has no side", row.name, row.step, got, row.want)
		}
	}
	// An empty step is no name, on either side.
	if model.SameName("", ".git") || model.SameName(".git", "") {
		t.Error("an empty step is the same name as .git")
	}
}
