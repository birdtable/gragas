package coreapi_test

import (
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestARecordThatWeakensTheParentsBindingIsAFinding (S34): with the
// parent handed in, as a fetch will hand it, an overlay's bindings/
// record that reports a parent's rule less loudly than the parent binds it
// is a finding at the record's severity line, whose fix says the removal in
// words with the path as %q; the control, a record that tightens the same
// binding, has none. A record whose name holds a space, ; and $( gets the
// same words, never a command joined from its path (go-console#218).
// Risk: an overlay that loosens a parent's rule by a record written by hand;
// a fix that pastes a second command a record's name smuggles in.
func TestARecordThatWeakensTheParentsBindingIsAFinding(t *testing.T) {
	t.Parallel()
	overlay := strings.Replace(bookIdentity, "kind: base", "kind: overlay", 1) + "extends:\n  rulebook: std-base\n  version: 2.1.0\n"
	parent := format.Composed{Parent: &format.Foreign{Rulebook: "std-base", Version: "2.1.0", Parts: []string{"std"},
		Rules: []model.Rule{{ID: "std.errors.4", Status: model.Active, Bindings: []model.Binding{{
			Tool: "golangci-lint", Target: "source", Key: "errorlint", Value: true, Severity: "error", Rung: 1, RunPoint: "commit"}}}}}}
	for _, tc := range []struct {
		name, file, severity, fix string
		finding                   bool
	}{
		{"a weakening record", "std-errors-4.yaml", "warning", `remove the record "book/bindings/std-errors-4.yaml" from the repository with git rm`, true},
		{"a weakening record whose name holds a space, ; and $(", "std errors;$(touch x).yaml", "warning", `remove the record "book/bindings/std errors;$(touch x).yaml" from the repository with git rm`, true},
		{"the control, a tightening record", "std-errors-4.yaml", "error", "", false},
	} {
		record := "subject: std.errors.4\ntool: golangci-lint\ntarget: source\nkey: errorlint\nvalue: true\nseverity: " + tc.severity + "\nrung: 1\nrun_point: commit\n"
		w, root := checkersWorld(t, overlay, map[string]string{"bindings/" + tc.file: record})
		r, err := coreapi.LoadBookComposed(t.Context(), w.Ports(), root, parent)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var mine []model.Finding
		for _, f := range r.Findings {
			if f.File == "book/bindings/"+tc.file {
				mine = append(mine, f)
			}
		}
		if !tc.finding {
			if len(mine) != 0 {
				t.Errorf("%s: %+v", tc.name, mine)
			}
			continue
		}
		if len(mine) != 1 || mine[0].Line != 6 || mine[0].Fix != tc.fix ||
			!strings.Contains(mine[0].Msg, "the record of std.errors.4 weakens the binding the parent std-base 2.1.0 lends at golangci-lint source errorlint, at its severity") {
			t.Errorf("%s: %+v", tc.name, r.Findings)
		}
	}
}

// TestARecordsValueIsRankedByThePluginThatServesTheTool: a
// record whose value differs from the one the parent lends is a finding where
// the plugin installed for the tool ranks it weaker, and none where it ranks
// it tighter. With no plugin of the tool, the value is not compared: a notice
// names the tool and no value finding is made up, while a second record that
// weakens the severity at the same value is still a finding. The plant is
// the plugin's weaker word, which the tighter row reads as no finding.
func TestARecordsValueIsRankedByThePluginThatServesTheTool(t *testing.T) {
	t.Parallel()
	overlay := strings.Replace(bookIdentity, "kind: base", "kind: overlay", 1) + "extends:\n  rulebook: std-base\n  version: 2.1.0\n"
	lends := func(id, key string) model.Rule {
		return model.Rule{ID: model.RuleID(id), Status: model.Active, Bindings: []model.Binding{{
			Tool: "golangci-lint", Target: "source", Key: key, Value: "strict", Severity: "error", Rung: 1, RunPoint: "commit"}}}
	}
	parent := format.Composed{Parent: &format.Foreign{Rulebook: "std-base", Version: "2.1.0", Parts: []string{"std"},
		Rules: []model.Rule{lends("std.errors.4", "errorlint"), lends("std.errors.5", "wrapcheck")}}}
	record := func(id, key, value, severity string) string {
		return "subject: " + id + "\ntool: golangci-lint\ntarget: source\nkey: " + key + "\nvalue: " + value + "\nseverity: " + severity + "\nrung: 1\nrun_point: commit\n"
	}
	records := map[string]string{
		"bindings/std-errors-4.yaml": record("std.errors.4", "errorlint", "warn", "error"),
		"bindings/std-errors-5.yaml": record("std.errors.5", "wrapcheck", "strict", "warning"),
	}
	for _, tc := range []struct {
		name   string
		word   plugin.Word
		served bool
		value  bool
		notice string
	}{
		{"ranked weaker", plugin.WordWeaker, true, true, ""},
		{"ranked tighter", plugin.WordTighter, true, false, ""},
		{"no plugin of the tool", "", false, false, "no plugin is installed for golangci-lint"},
	} {
		w, root := checkersWorld(t, overlay, records)
		if tc.served {
			p := servedBy(w, root, "go-lint", "1.0.0", "golangci-lint")
			p.OnRank = func(plugin.RankRequest) (plugin.RankResponse, error) {
				return plugin.RankResponse{Words: []plugin.Word{tc.word}}, nil
			}
		}
		r, err := coreapi.LoadBookComposed(t.Context(), w.Ports(), root, parent)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var value, severity int
		for _, f := range r.Findings {
			switch {
			case f.File == "book/bindings/std-errors-4.yaml" && strings.Contains(f.Msg, "at its value"):
				value++
			case f.File == "book/bindings/std-errors-5.yaml" && strings.Contains(f.Msg, "at its severity"):
				severity++
			}
		}
		if (value == 1) != tc.value || severity != 1 {
			t.Errorf("%s: %d value and %d severity findings in %+v; want the value %v and the severity", tc.name, value, severity, r.Findings, tc.value)
		}
		said := false
		for _, n := range r.Notices {
			said = said || strings.Contains(n.Msg, "the binding records were not all compared")
			if tc.notice == "" || !strings.Contains(n.Msg, tc.notice) {
				continue
			}
			tc.notice = ""
		}
		if tc.notice != "" || said != !tc.served {
			t.Errorf("%s: notices %+v", tc.name, r.Notices)
		}
	}
}
