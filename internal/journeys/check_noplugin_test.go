package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheOldCheckOfARulebookRepositoryWithNoPluginEndsNotReadyNamingTheTool
// : the built binary's check, in a rulebook repository whose go
// part binds golangci-lint with no plugin installed for it, ends not ready
// (exit 4), says which tool was not checked and how to install its plugin,
// and says gragas tools as the next step, as the new check does. It is no
// warning above a done.
// Risk: a pass that judged no tool, which the gate's rulebook member reads as
// green. A change back to a warning leaves the exit at 0 and this red.
func TestTheOldCheckOfARulebookRepositoryWithNoPluginEndsNotReadyNamingTheTool(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(t.TempDir(), "repo")
	copyTree(t, oldBook(), filepath.Join(repo, "rulebook"))
	for path, body := range map[string]string{
		".gragas/parts": "go\n",
		".golangci.yml": "version: \"2\"\nlinters:\n  default: none\n",
	} {
		p := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := ruleEnv(t)
	env["COLUMNS"] = "200"
	r := run(t, filepath.Dir(repo), env, "check", "-C", repo)
	said := strings.Join(strings.Fields(r.stdout+r.stderr), " ")
	if r.code != 4 || !strings.Contains(said, "no plugin is installed for golangci-lint") || !strings.Contains(said, "gragas plugin add") || !strings.Contains(said, "gragas tools") {
		t.Errorf("exit %d, want 4 (not ready) naming golangci-lint, plugin add and gragas tools\n%s%s", r.code, r.stdout, r.stderr)
	}
}
