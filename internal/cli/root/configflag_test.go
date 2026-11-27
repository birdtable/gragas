package root

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigFileIsReadFromTheLineBeforeTheRest: main reads --config before
// the tree runs, by the tree's own parser, so a value is never taken for the
// flag and nothing after -- counts. Risk: a run that reads a settings file
// nobody named, or not the one named.
func TestConfigFileIsReadFromTheLineBeforeTheRest(t *testing.T) {
	t.Parallel()
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	a := filepath.Join(root, "a", "s.yaml")
	b := filepath.Join(root, "b", "s.yaml")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"after the command", []string{"check", "--config", a}, a},
		{"before the command, with =", []string{"--config=" + a, "gen", "--apply"}, a},
		{"a relative path is from the working folder", []string{"check", "--config", "s.yaml"}, filepath.Join(here, "s.yaml")},
		{"the last of two, as the tree takes it", []string{"check", "--config", a, "--config", b}, b},
		{"none", []string{"check"}, ""},
		{"none at all", nil, ""},
		{"the value of another flag", []string{"gen", "-C", "--config"}, ""},
		{"after --", []string{"check", "--", "--config", a}, ""},
		{"a line the tree cannot parse", []string{"check", "--aply", "--config", a}, ""},
		{"with no value", []string{"check", "--config"}, ""},
	}
	named := 0
	for _, tc := range tests {
		if tc.want != "" {
			named++
		}
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ConfigFile(t.Context(), tc.args); got != tc.want {
				t.Errorf("ConfigFile(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
	if named == 0 {
		t.Fatal("no case names a file")
	}
}

// TestTheTreeTakesConfigAsAFlag: --config is a flag of every command, so
// the run that main read it for does not refuse it as unknown.
func TestTheTreeTakesConfigAsAFlag(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "check", "--config", filepath.Join(t.TempDir(), "s.yaml"))
	if code, said := executeSaid(t.Context(), t, p); code != 0 {
		t.Errorf("exit %d, said:\n%s", code, said)
	}
}
