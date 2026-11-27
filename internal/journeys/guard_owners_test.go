package journeys

import (
	"strings"
	"testing"
)

// addOwner is a hand edit of the book's owners: the list in rulebook.yaml
// gains team-b, as a person adds an owner with no command (none changes
// owners).
func addOwner(t *testing.T) func(string, map[string]string) {
	t.Helper()
	return func(dir string, _ map[string]string) {
		// the fixture book writes the list in flow form, init in block form
		if strings.Contains(read(t, dir, "rulebook.yaml"), "owners: [team-a]") {
			edit(t, dir, "rulebook.yaml", "owners: [team-a]", "owners: [team-a, team-b]")
			return
		}
		edit(t, dir, "rulebook.yaml", "owners:\n  - \"team-a\"\n", "owners:\n  - \"team-a\"\n  - \"team-b\"\n")
	}
}

// TestGuardStagedAllowsAHandEditOfTheOwners: the owners of rulebook.yaml are not a lifecycle value. They
// are display only (model.RulebookIdentity), no command writes them after
// init and migrate, and the guard leaves the identity to a file edit
// (lifecycle.UnguardedParts). So a hand edit of them is allowed in a plain
// commit (K1), in a merge over a base with one book (K2), and in a merge
// whose base holds two books, made in the merge itself (M4).
// Each row is red against a guard that holds the owners as a lifecycle value.
// Risk: owners become frozen, refused in the one way they can change, or a
// later change to the guard holds them unseen.
func TestGuardStagedAllowsAHandEditOfTheOwners(t *testing.T) {
	t.Parallel()
	t.Run("K1, a plain commit", func(t *testing.T) {
		t.Parallel()
		dir, env := stagedRepo(t)
		addOwner(t)(dir, env)
		gitIn(t, dir, env, "add", "-A")
		allowed(t, "a hand edit of the owners", staged(t, dir, env))
	})
	t.Run("K2, a merge over a base with one book", func(t *testing.T) {
		t.Parallel()
		dir, env := branched(t, addOwner(t), proseEdit(t))
		gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
		allowed(t, "a merge that brings in a hand edit of the owners", staged(t, dir, env))
	})
	t.Run("M4, a merge over a base with two books, edited in the merge", func(t *testing.T) {
		t.Parallel()
		dir, env := beforeTheSwitch(t, makesTheBook(t))
		addOwner(t)(dir, env)
		gitIn(t, dir, env, "add", "-A")
		allowed(t, "a merge over a base with two books whose owners are edited in it", staged(t, dir, env))
	})
}
