package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/birdtable/go-files/appdirs"

	"github.com/birdtable/gragas/internal/config"
)

// TestOnlyTheAccountsOwnStateFolderIsTheDefault: the state
// folder the home alone gives, the home the system's, is the account's own;
// one the state variable moves, one a home variable naming another folder
// moves, and every doubt are not, so they are judged. Planted failure:
// DefaultState answering true for any state folder, or comparing against the
// home the variable names.
func TestOnlyTheAccountsOwnStateFolderIsTheDefault(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	own := filepath.Join(dir, "h")
	system := func() (string, error) { return own, nil }
	home := map[string]string{appdirs.HomeVar: own}
	tests := []struct {
		name string
		env  map[string]string
		home func() (string, error)
		want bool
	}{
		{"the home alone, the system's", home, system, true},
		{"a state variable moves it", moving(t, home, func(f string) string { return filepath.Join(dir, "moved", f) }), system, false},
		{"a home variable naming another folder moves it", map[string]string{appdirs.HomeVar: filepath.Join(dir, "other")}, system, false},
		{"the system cannot say the home", home, func() (string, error) { return "", errors.New("no home") }, false},
		{"a system home appdirs would not join", home, func() (string, error) { return "h", nil }, false},
		{"no home, so no state folder", map[string]string{}, system, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := config.FromEnv(env(tc.env)).DefaultState(tc.home); got != tc.want {
				t.Errorf("DefaultState = %v, want %v", got, tc.want)
			}
		})
	}
}
