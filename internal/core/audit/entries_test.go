package audit_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// standinFiles is the readers row of the stand-in's config: two names, the
// first that is there being the one read.
func standinFiles() []string { return []string{"standin.conf", "standin.cfg"} }

// standinBound is three bindings to the stand-in's keys.
func standinBound() []audit.Binding {
	return []audit.Binding{
		{"s.lint.1", "standin", "errcheck"},
		{"s.lint.2", "standin", "gosec"},
		{"s.lint.3", "standin", "forbidigo"},
	}
}

// seam is the check's Entries over the stand-in plugin: the
// stand-in reads its entries from the config the check sends, a line "key"
// on and "-key" off, and none when it is sent none. Its answer is encoded and
// read back through the contract's types (ToCanon, canon, FromCanon), so the
// audit judges what the answer becomes as data, not the Go value the
// stand-in built; the wire's own checks of an
// answer are held in core/api, which may import the codec. The seam records
// each request, and answers err in place of the plugin when err is set.
type seam struct {
	mu    sync.Mutex
	asked []plugin.PlanRequest
	err   error
}

func (s *seam) entries(tool string) (audit.EntriesCall, string, string) {
	if tool != "standin" {
		return nil, "", "no plugin of " + tool + " is installed"
	}
	p := porttest.NewStandinPlugin()
	return func(_ context.Context, in plugin.PlanRequest) (plugin.EntriesResponse, error) {
		s.mu.Lock()
		s.asked = append(s.asked, in)
		s.mu.Unlock()
		if s.err != nil {
			return plugin.EntriesResponse{}, s.err
		}
		got, err := p.OnEntries(plugin.EntriesRequest{ToolVersion: in.ToolVersion, Config: in.Config, Have: []plugin.Result{}, Target: in.Target})
		if err != nil {
			return got, err
		}
		return roundTrip(got)
	}, "1.0.0", ""
}

// roundTrip is an answer encoded as data and read back into a fresh value.
func roundTrip(in plugin.EntriesResponse) (plugin.EntriesResponse, error) {
	var out plugin.EntriesResponse
	b, err := canon.Encode(in.ToCanon(), canon.Document)
	if err != nil {
		return out, err
	}
	v, err := canon.Decode(b)
	if err != nil {
		return out, err
	}
	return out, out.FromCanon(v)
}

func standinOnly() []audit.Source {
	return []audit.Source{{Tool: "standin", Files: standinFiles(), Kind: audit.KindEntries}}
}

func conf(text string) fstest.MapFS { return fstest.MapFS{"standin.conf": {Data: []byte(text)}} }

// TestTwoBoundEntriesOffAreTwoFindings: an entry the plugin says is off is a
// finding naming the file, and one that is on holds its binding.
func TestTwoBoundEntriesOffAreTwoFindings(t *testing.T) {
	t.Parallel()
	s := &seam{}
	got := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("errcheck\n-gosec\n-forbidigo\n"), Bindings: standinBound(), Entries: s.entries})
	want := []string{
		"standin.conf: s.lint.2 (standin gosec): gosec is off",
		"standin.conf: s.lint.3 (standin forbidigo): forbidigo is off",
	}
	if len(got.Findings) != 2 || got.Findings[0].String() != want[0] || got.Findings[1].String() != want[1] {
		t.Fatalf("findings = %v, want %v", got.Findings, want)
	}
	clean := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("errcheck\ngosec\nforbidigo\n"), Bindings: standinBound(), Entries: s.entries})
	if len(clean.Findings)+len(clean.Exempt)+len(clean.NotChecked) != 0 {
		t.Fatalf("the clean fixture reports %+v, want nothing", clean)
	}
}

// TestAKeyWithNoEntryIsAFindingNamingTheFileOrItsAbsence: a binding whose key
// the plugin gives no entry of is a finding; with no config there, the
// plugin is asked with none, and the finding names the first file the tool
// would take.
// Risk: a rule whose key the config does not hold passes as on.
func TestAKeyWithNoEntryIsAFindingNamingTheFileOrItsAbsence(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"s.lint.9", "standin", "depguard"}}
	s := &seam{}
	there := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("other\n"), Bindings: b, Entries: s.entries})
	if len(there.Findings) != 1 || there.Findings[0].Missing != "standin.conf has no entry depguard" {
		t.Errorf("a config with no entry: %+v", there.Findings)
	}
	none := &seam{}
	gone := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: fstest.MapFS{}, Bindings: b, Entries: none.entries})
	if len(gone.Findings) != 1 || gone.Findings[0].Missing != "there is no standin.conf, so depguard is not set" || gone.Findings[0].File != "standin.conf" {
		t.Errorf("no config: %+v", gone.Findings)
	}
	if len(none.asked) != 1 || len(none.asked[0].Config) != 0 {
		t.Errorf("no config: the plugin was asked %+v, want once with no config", none.asked)
	}
}

// TestThePluginIsAskedWithTheFileTheToolTakesAsText: the check sends the one
// file the tool takes, the first of the row's names that is there, as text,
// once, with the tool version the seam names, for the source target; a file
// that is not UTF-8 is never sent. Planted failure: the row's order reversed
// sends the other file.
func TestThePluginIsAskedWithTheFileTheToolTakesAsText(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{"standin.conf": {Data: []byte("errcheck\n")}, "standin.cfg": {Data: []byte("gosec\n")}}
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{"the row's order", standinFiles(), "standin.conf"},
		{"the order reversed, planted", []string{"standin.cfg", "standin.conf"}, "standin.cfg"},
	} {
		s := &seam{}
		audit.Check(t.Context(), audit.Input{Sources: []audit.Source{{Tool: "standin", Files: tc.files, Kind: audit.KindEntries}}, Repo: repo, Bindings: standinBound(), Entries: s.entries})
		if len(s.asked) != 1 || len(s.asked[0].Config) != 1 {
			t.Fatalf("%s: asked %+v, want one request with one config", tc.name, s.asked)
		}
		in := s.asked[0]
		if c := in.Config[0]; c.Path != tc.want || c.Data == nil || c.Data.Text == nil || *c.Data.Text != string(repo[tc.want].Data) {
			t.Errorf("%s: sent %+v, want %s as its text", tc.name, c, tc.want)
		}
		if in.ToolVersion != "1.0.0" || in.Target != plugin.TargetSource || in.For != plugin.ForConfig {
			t.Errorf("%s: request %+v, want version 1.0.0, target source, for config", tc.name, in)
		}
	}
	s := &seam{}
	got := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("\xff\xfe\n"), Bindings: standinBound(), Entries: s.entries})
	if len(s.asked) != 0 || len(got.NotChecked) != 1 || !strings.Contains(got.NotChecked[0].Why, "standin.conf is not UTF-8 text") {
		t.Errorf("a config that is not text: asked %d, %+v; want no call and one not-checked naming it", len(s.asked), got)
	}
}

// TestAToolWhosePluginCannotAnswerIsNotCheckedOrHalts: no seam, a seam with
// no plugin of the tool and a call that errs are each the tool's "not
// checked", in their words, with no finding; a run the host cannot hold
// halts the check red.
// Risk: a rule passes because its plugin could not be asked.
func TestAToolWhosePluginCannotAnswerIsNotCheckedOrHalts(t *testing.T) {
	t.Parallel()
	refused := &seam{err: &plugin.CallError{Code: plugin.CodeRefused, Message: "standin.conf loads code the tool would run"}}
	other := func(string) (audit.EntriesCall, string, string) { return nil, "", "no plugin of standin is installed" }
	for name, tc := range map[string]struct {
		entries audit.Entries
		says    string
	}{
		"no seam":            {nil, "no plugin of standin is there to read its config"},
		"no plugin":          {other, "no plugin of standin is installed"},
		"the plugin refuses": {refused.entries, "loads code the tool would run"},
	} {
		got := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("errcheck\n"), Bindings: standinBound(), Entries: tc.entries})
		if len(got.Findings) != 0 || len(got.NotChecked) != 1 || got.NotChecked[0].Tool != "standin" || !strings.Contains(got.NotChecked[0].Why, tc.says) || got.Halt != nil {
			t.Errorf("%s: %+v, want one not-checked saying %q and no finding", name, got, tc.says)
		}
	}
	held := &seam{err: fmt.Errorf("the run: %w", port.ErrNotHeld)}
	got := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("errcheck\n"), Bindings: standinBound(), Entries: held.entries})
	if !errors.Is(got.Halt, port.ErrNotHeld) || len(got.NotChecked) != 0 || len(got.Findings) != 0 {
		t.Errorf("a run the host cannot hold: %+v, want the check halted", got)
	}
}

// TestAnExemptedRuleIsExemptNotAFinding: a live exemption row waives the
// finding of its rule until its date; an expired row is not offered.
func TestAnExemptedRuleIsExemptNotAFinding(t *testing.T) {
	t.Parallel()
	s := &seam{}
	live := func(rule string) (string, bool) { return "2027-03-31", rule == "s.lint.2" }
	got := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("errcheck\n"), Bindings: standinBound(), Entries: s.entries, Exempt: live})
	if len(got.Findings) != 1 || got.Findings[0].Rule != "s.lint.3" {
		t.Errorf("findings = %v, want only s.lint.3", got.Findings)
	}
	if len(got.Exempt) != 1 || got.Exempt[0].Until != "2027-03-31" || !strings.HasSuffix(got.Exempt[0].String(), "(exempt until 2027-03-31)") {
		t.Errorf("exempt = %v", got.Exempt)
	}
	expired := func(string) (string, bool) { return "", false }
	if r := audit.Check(t.Context(), audit.Input{Sources: standinOnly(), Repo: conf("errcheck\n"), Bindings: standinBound(), Entries: s.entries, Exempt: expired}); len(r.Findings) != 2 {
		t.Errorf("expired row: %v, want two findings", r.Findings)
	}
}

// TestOnlyTheEntriesKindAsksThePlugin: the TOML and the task kinds never
// reach the Entries seam, whatever tool a row names (condition 5).
func TestOnlyTheEntriesKindAsksThePlugin(t *testing.T) {
	t.Parallel()
	asked := 0
	count := func(string) (audit.EntriesCall, string, string) {
		asked++
		return nil, "", "counted"
	}
	b := []audit.Binding{{"supply.secrets.1", "gitleaks", "extend.useDefault"}, {"gates.shared.1", "task", "vet"}}
	audit.Check(t.Context(), audit.Input{Repo: fstest.MapFS{}, Bindings: b, Sources: sources(), Entries: count, TOML: toml(nil)})
	if asked != 0 {
		t.Errorf("the seam was asked %d times for a TOML and a task rule", asked)
	}
}
