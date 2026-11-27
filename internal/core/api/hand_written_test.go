package coreapi_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// handBook is valueBook with a fifth rule binding lint-h, and the identity
// listing listed as the tools whose presets are hand-written.
func handBook(listed string) map[string]string {
	files := valueBook()
	files["language/go/rules/errors.yaml"] += `  - id: go.errors.5
    statement: Keep the changelog.
    strength: should
    status: active
    origin: authored
    bindings:
      - {tool: lint-h, target: config, key: entry, value: kept, severity: warning, rung: 1, run_point: commit}
`
	files["rulebook.yaml"] += "hand_written_presets: [" + listed + "]\n"
	return files
}

// validated is the findings Validate gives of the book at root, as messages.
func validated(t *testing.T, w *porttest.World, root model.Root) []string {
	t.Helper()
	v, err := coreapi.Validate(context.Background(), w.Ports(), root, "")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var out []string
	for _, f := range v.Findings {
		out = append(out, f.Msg)
	}
	return out
}

// TestAListedToolIsRankedByNoPluginAndEndsNothingNotReady: a tool the identity lists
// as hand-written is asked of no plugin, and its two values that differ as
// text are a conflict, not a read ended not ready; the book is clean.
// Planted failure: a rank that asks the plugins of a listed tool too, which
// ends the read on lint-a unserved.
func TestAListedToolIsRankedByNoPluginAndEndsNothingNotReady(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, handBook("lint-a, lint-h"))
	other, asked := ranking(w, root, plugin.WordSame)
	other.Facts.Tool = "lint-z"
	if got := validated(t, w, root); len(got) != 0 {
		t.Fatalf("a listed tool no plugin serves: findings %q; want none", got)
	}
	l, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
	if err != nil || len(l.Conflicts) != 1 || !slices.Equal(l.Conflicts[0].Differs, []string{model.DiffValue}) {
		t.Fatalf("ListConflicts = %+v, %v; want wrap's value conflict and no error", l, err)
	}
	if got := asked(); len(got) != 0 {
		t.Errorf("a plugin was asked %+v of a listed tool", got)
	}
}

// TestAListedToolAPluginServesIsAFinding: the list never silences a plugin
// that can rank the tool. Validate finds it, naming the plugin, and the
// conflicts listing, which reads no book with findings, ends on them, asking
// the plugin nothing. Planted failure: a load that weighs the list against
// no plugin, which finds nothing.
func TestAListedToolAPluginServesIsAFinding(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, handBook("lint-a, lint-h"))
	_, asked := ranking(w, root, plugin.WordSame)
	want := "hand_written_presets lists lint-a, which the installed plugin a-lint 1.0.0 serves"
	if got := validated(t, w, root); !slices.Equal(got, []string{want}) {
		t.Fatalf("findings %q; want %q", got, want)
	}
	if _, err := coreapi.ListConflicts(context.Background(), w.Ports(), root); !errors.As(err, new(*coreapi.FindingsError)) {
		t.Errorf("ListConflicts: %v; want FindingsError", err)
	}
	if got := asked(); len(got) != 0 {
		t.Errorf("the plugin was asked %+v of a listed tool", got)
	}
}

// TestAnUnlistedToolNoPluginServesStillEndsNotReady: the list is no default.
// With lint-h listed, lint-a, bound and unlisted, that no plugin serves ends
// the conflicts listing not ready, naming lint-a alone. Planted failure: a
// rank that skips every tool once the book lists one, which lists the
// conflict and ends done.
func TestAnUnlistedToolNoPluginServesStillEndsNotReady(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, handBook("lint-h"))
	if got := validated(t, w, root); len(got) != 0 {
		t.Fatalf("findings %q; want none", got)
	}
	_, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
	u, ok := errors.AsType[*coreapi.UnservedTools](err)
	if !ok || !slices.Equal(u.Tools, []string{"lint-a"}) || !errors.Is(err, coreapi.ErrNotReady) {
		t.Fatalf("ListConflicts: %v; want lint-a unserved, not ready", err)
	}
}

// TestAListedToolNoActiveOrTrialRuleBindsIsAFinding: no entry stays on the
// list once its rules are gone. lint-q, which no rule binds, is a finding of
// the identity with the fix to take it off; lint-h and lint-a, which rules
// bind, are not. Planted failure: a load that does not hold the list to the
// bindings, which finds nothing.
func TestAListedToolNoActiveOrTrialRuleBindsIsAFinding(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, handBook("lint-a, lint-h, lint-q"))
	want := "hand_written_presets lists lint-q, which no active or trial rule binds"
	got := validated(t, w, root)
	if !slices.Equal(got, []string{want}) {
		t.Fatalf("findings %q; want %q", got, want)
	}
	v, err := coreapi.Validate(context.Background(), w.Ports(), root, "")
	if err != nil || len(v.Findings) != 1 || v.Findings[0].File != "rulebook.yaml" ||
		!strings.Contains(v.Findings[0].Msg+v.Findings[0].Fix, "take lint-q off hand_written_presets") {
		t.Errorf("the finding = %+v, %v; want it on rulebook.yaml with the fix", v.Findings, err)
	}
}
