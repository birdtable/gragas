package plugintest_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugintest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// serving is a plugin that serves the tool versions in holds, and answers
// unsupported to the rest and to a target it does not know.
func serving(holds func(string) bool) *porttest.StandinPlugin {
	return &porttest.StandinPlugin{
		OnPlan: func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
			if in.Target != plugin.TargetSource || !in.For.Valid() || !holds(in.ToolVersion) {
				return plugin.PlanResponse{}, unsupported()
			}
			return plugin.PlanResponse{Final: true}, nil
		},
		OnEntries: func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			if in.Target != plugin.TargetSource || !holds(in.ToolVersion) {
				return plugin.EntriesResponse{}, unsupported()
			}
			return plugin.EntriesResponse{Entries: []plugin.Entry{}}, nil
		},
	}
}

func runWith(t *testing.T, e plugintest.Engine, b plugintest.Bundle) map[int]model.PluginCheck {
	t.Helper()
	out := map[int]model.PluginCheck{}
	for _, c := range plugintest.Run(t.Context(), e, b) {
		out[c.N] = c
	}
	return out
}

// TestARangeThatServesOnlyPastItsExclusiveBound: a range such as >1.0.0 serves
// 1.0.1 and not 1.0.0. A plugin that answers unsupported to every call is red
// on C16, which names the version it should serve; one that serves the range
// and answers unsupported only outside it is green on C14 to C16. Risk: a
// probe sent at a fixed version, passing for the wrong reason.
func TestARangeThatServesOnlyPastItsExclusiveBound(t *testing.T) {
	t.Parallel()
	e := withEngine()
	e.Ends = func(string) ([]string, []string, error) { return []string{"1.0.1"}, []string{"1.0.0"}, nil }
	b := full()

	b.Plugin = opened(t, serving(func(string) bool { return false }))
	got := runWith(t, e, b)
	if got[16].Status != model.CheckFailed {
		t.Errorf("C16 for a plugin that serves nothing: %s %v, want failed", got[16].Status, got[16].What)
	}

	b.Plugin = opened(t, serving(func(v string) bool { return v == "1.0.1" }))
	got = runWith(t, e, b)
	for _, n := range []int{14, 15, 16} {
		if got[n].Status == model.CheckFailed {
			t.Errorf("C%d for a plugin that serves the range: failed %v", n, got[n].What)
		}
	}
	if got[16].Status != model.CheckPassed {
		t.Errorf("C16 for a plugin that serves the range: %s %v, want passed", got[16].Status, got[16].What)
	}
}

// TestARangeThatNamesNoServedVersionIsNotRun: with no version to send, C14, C15
// and C16 are not run with why, never passed.
func TestARangeThatNamesNoServedVersionIsNotRun(t *testing.T) {
	t.Parallel()
	e := withEngine()
	e.Ends = func(string) ([]string, []string, error) { return nil, []string{"1.0.0"}, nil }
	b := full()
	b.Manifest = fullManifest("provides: [entries, findings, plan, rank, preset]", "provides: [entries]")
	b.Plugin = opened(t, serving(func(string) bool { return false }))
	got := runWith(t, e, b)
	for _, n := range []int{14, 15, 16} {
		if got[n].Status != model.CheckNotRun || len(got[n].What) == 0 {
			t.Errorf("C%d: %s %v, want not run with why", n, got[n].Status, got[n].What)
		}
	}
}

// TestACheckForAnOperationNotProvidedIsNotAsked: a bundle that provides only
// entries asks no evidence of findings, plan, preset or rank, and says so; none
// reads as passed, and none counts against done.
func TestACheckForAnOperationNotProvidedIsNotAsked(t *testing.T) {
	t.Parallel()
	b := good()
	b.Manifest = manifest()
	b.Cases = cases()[:1]
	b.Recordings = []plugintest.Recording{{Version: "10.13.0", Files: []string{"out.txt"}}}
	got := run(t, b)
	if got[18].Status != model.CheckPassed {
		t.Errorf("C18: %s %v, want passed", got[18].Status, got[18].What)
	}
	for n := 19; n <= 23; n++ {
		if got[n].Status != model.CheckNotAsked || len(got[n].What) == 0 {
			t.Errorf("C%d: %s %v, want not asked naming the operation", n, got[n].Status, got[n].What)
		}
	}
}

// TestANotAskedCheckCountsTowardDoneAndANotRunOneDoesNot: the closing word of a
// list whose checks passed or were not asked is done; one check that applies
// and did not run makes it not ready.
func TestANotAskedCheckCountsTowardDoneAndANotRunOneDoesNot(t *testing.T) {
	t.Parallel()
	var checks []model.PluginCheck
	for n := 1; n <= model.PluginChecks; n++ {
		s := model.CheckPassed
		if n >= 19 {
			s = model.CheckNotAsked
		}
		checks = append(checks, model.PluginCheck{N: n, Status: s})
	}
	if w := (model.PluginTest{Checks: checks}).Word(); w != model.Done {
		t.Errorf("passed or not asked: %s, want done", w)
	}
	checks[4].Status = model.CheckNotRun
	if w := (model.PluginTest{Checks: checks}).Word(); w != model.NotReady {
		t.Errorf("one applying check not run: %s, want not ready", w)
	}
}

// TestAProbeIsSentInsideTheRangeAtRoundZero: with the range >1.0.0 a plugin
// that serves 1.0.1 and ignores the for it is sent is red on C14, not passed
// for answering unsupported to a version outside the range; and a one-round
// plugin that answers unsupported past round 0, with plan first in provides, is
// green on C14 to C16.
func TestAProbeIsSentInsideTheRangeAtRoundZero(t *testing.T) {
	t.Parallel()
	e := withEngine()
	e.Ends = func(string) ([]string, []string, error) { return []string{"1.0.1"}, []string{"1.0.0"}, nil }
	b := full()
	b.Manifest = fullManifest("provides: [entries, findings, plan, rank, preset]", "provides: [plan, entries]")

	lax := serving(func(v string) bool { return v == "1.0.1" })
	lax.OnPlan = func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		if in.Target != plugin.TargetSource || in.ToolVersion != "1.0.1" {
			return plugin.PlanResponse{}, unsupported()
		}
		return plugin.PlanResponse{Final: true}, nil
	}
	b.Plugin = opened(t, lax)
	if got := runWith(t, e, b); got[14].Status != model.CheckFailed {
		t.Errorf("C14 for a plugin that ignores for: %s %v, want failed", got[14].Status, got[14].What)
	}

	oneRound := serving(func(v string) bool { return v == "1.0.1" })
	oneRound.OnPlan = func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		if in.Round != 0 || in.Target != plugin.TargetSource || !in.For.Valid() || in.ToolVersion != "1.0.1" {
			return plugin.PlanResponse{}, unsupported()
		}
		return plugin.PlanResponse{Final: true}, nil
	}
	b.Plugin = opened(t, oneRound)
	got := runWith(t, e, b)
	for _, n := range []int{14, 15, 16} {
		if got[n].Status == model.CheckFailed {
			t.Errorf("C%d for a one-round plugin: failed %v", n, got[n].What)
		}
	}
}

// TestNotAskedIsOnlyForTheEvidenceChecks: not asked on C5 is no pass, so the
// word is not ready; on C23 it is, and the word is done.
func TestNotAskedIsOnlyForTheEvidenceChecks(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		n    int
		want model.Word
	}{{5, model.NotReady}, {18, model.Done}, {23, model.Done}} {
		var checks []model.PluginCheck
		for n := 1; n <= model.PluginChecks; n++ {
			s := model.CheckPassed
			if n == c.n {
				s = model.CheckNotAsked
			}
			checks = append(checks, model.PluginCheck{N: n, Status: s})
		}
		if w := (model.PluginTest{Checks: checks}).Word(); w != c.want {
			t.Errorf("not asked on C%d: %s, want %s", c.n, w, c.want)
		}
	}
}

// TestTheRoundTripReadsWithTheRecordedRun: a plugin whose entries come from
// the recorded run, and are none without it, reads the preset back; the
// round trip is green only because have is sent.
func TestTheRoundTripReadsWithTheRecordedRun(t *testing.T) {
	t.Parallel()
	p := standin()
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		if in.Target != plugin.TargetSource || !inRange(in.ToolVersion) {
			return plugin.EntriesResponse{}, unsupported()
		}
		if len(in.Have) == 0 {
			return plugin.EntriesResponse{Entries: []plugin.Entry{}}, nil
		}
		sev := plugin.Severity("error")
		return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "k", On: true, Severity: &sev, Value: canonInt(1)}}}, nil
	}
	b := full()
	b.Plugin = opened(t, p)
	if got := run(t, b); got[23].Status != model.CheckPassed {
		t.Errorf("C23 for a plugin that reads the run: %s %v, want passed", got[23].Status, got[23].What)
	}
}

// withTrialEntry is a bundle whose preset golden asks for a trial entry besides
// its own, and a plugin that reads back what the tool has on: that entry too,
// with the severity the preset gave it, as golangci-lint's trial gocritic is.
func withTrialEntry(t *testing.T, extra *plugin.Entry) plugintest.Bundle {
	t.Helper()
	const trialIn = `{"key":"t","options":null,"rules":["r"],"severity":"warning","target":"source","trial":true,"value":1}`
	p := standin()
	on := p.OnEntries
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		r, err := on(in)
		if len(in.Config) > 0 {
			r.Entries = append(r.Entries, plugin.Entry{Key: "t", On: true, Value: canonInt(1)})
			if extra != nil {
				r.Entries = append(r.Entries, *extra)
			}
		}
		return r, err
	}
	b := full()
	b.Plugin = opened(t, p)
	for i, c := range b.Cases {
		if c.Name == "7-preset" {
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), `"entries":[`+presetIn+`]`, `"entries":[`+presetIn+`,`+trialIn+`]`, 1))
		}
	}
	return b
}

// TestTheRoundTripLeavesATrialEntryOutOfWhatIsNotInThePreset: an entry the
// preset carries as a trial is left out of its files on purpose, so the tool
// having it on is not "read back and not in the preset"; the same read-back
// with one entry the preset never carried is still red.
func TestTheRoundTripLeavesATrialEntryOutOfWhatIsNotInThePreset(t *testing.T) {
	t.Parallel()
	if got := run(t, resave(withTrialEntry(t, nil))); got[23].Status != model.CheckPassed {
		t.Errorf("C23 with a trial entry read back: %s %v, want passed", got[23].Status, got[23].What)
	}
	got := run(t, resave(withTrialEntry(t, &plugin.Entry{Key: "extra", On: true, Value: canonInt(1)})))
	said := strings.Join(got[23].What, "\n")
	if got[23].Status != model.CheckFailed || !strings.Contains(said, "extra of") || strings.Contains(said, "t of") {
		t.Errorf("C23 with an extra entry beside the trial: %s %v, want failed on extra alone", got[23].Status, got[23].What)
	}
}

// TestTheRoundTripIsNotRunWhereNoCaseHoldsARecordedRun: with no entries case
// for the preset's tool version holding a have, C23 is not run with why, never
// passed and never failed.
func TestTheRoundTripIsNotRunWhereNoCaseHoldsARecordedRun(t *testing.T) {
	t.Parallel()
	b := withPlugin(t)
	for i, c := range b.Cases {
		if c.Name == "1-entries" {
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), ran, "", 1))
		}
	}
	got := run(t, resave(b))
	if got[23].Status != model.CheckNotRun || len(got[23].What) == 0 {
		t.Errorf("C23 with no recorded run: %s %v, want not run with why", got[23].Status, got[23].What)
	}
	for n, c := range got {
		if c.Status == model.CheckFailed {
			t.Errorf("C%d failed: %v", n, c.What)
		}
	}
}

// withReadback is a bundle whose manifest carries the preset block with the
// given readback lines, and a plugin that reads back what
// entries makes of the standin's one entry, k, which the preset golden holds
// with severity error.
func withReadback(t *testing.T, readback string, entries func(k plugin.Entry) []plugin.Entry) plugintest.Bundle {
	t.Helper()
	p := standin()
	on := p.OnEntries
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		r, err := on(in)
		if len(in.Config) > 0 && len(r.Entries) == 1 {
			r.Entries = entries(r.Entries[0])
		}
		return r, err
	}
	b := full()
	b.Manifest = fullManifest("warning: false\n", "warning: false\npreset:\n  path: c.yml\n  inputs: []\n  mark: MARK\n  readback:\n"+readback)
	b.Plugin = opened(t, p)
	return b
}

func sayC23(t *testing.T, b plugintest.Bundle) (model.PluginCheck, string) {
	t.Helper()
	got := run(t, resave(b))
	return got[23], strings.Join(got[23].What, "\n")
}

// TestTheReadbackDefaultsExcuseWhatIsNotInThePresetAndNothingElse: a key the
// manifest names under defaults is the tool's own and not "read back and not in
// the preset"; a key it does not name is still red; and an entry the preset
// holds that the plugin drops or changes is red with the declaration there.
func TestTheReadbackDefaultsExcuseWhatIsNotInThePresetAndNothingElse(t *testing.T) {
	t.Parallel()
	same := func(k plugin.Entry) []plugin.Entry { return []plugin.Entry{k} }
	extra := func(key string) func(k plugin.Entry) []plugin.Entry {
		return func(k plugin.Entry) []plugin.Entry {
			return []plugin.Entry{k, {Key: key, On: true, Value: canonInt(1)}}
		}
	}
	const decl = "    defaults: [dflt]\n"
	if c, said := sayC23(t, withReadback(t, decl, same)); c.Status != model.CheckPassed {
		t.Errorf("C23 with nothing extra: %s %s, want passed", c.Status, said)
	}
	if c, said := sayC23(t, withReadback(t, decl, extra("dflt"))); c.Status != model.CheckPassed {
		t.Errorf("C23 with the default read back: %s %s, want passed", c.Status, said)
	}
	if c, said := sayC23(t, withReadback(t, decl, extra("other"))); c.Status != model.CheckFailed || !strings.Contains(said, "other of") {
		t.Errorf("C23 with a key outside defaults: %s %s, want failed on other", c.Status, said)
	}
	if c, said := sayC23(t, withReadback(t, decl, extra("dflt-not"))); c.Status != model.CheckFailed || !strings.Contains(said, "dflt-not of") {
		t.Errorf("C23 with a key that only starts like a default: %s %s, want failed", c.Status, said)
	}
	dropped := func(k plugin.Entry) []plugin.Entry {
		return []plugin.Entry{{Key: "dflt", On: true, Value: canonInt(1)}}
	}
	if c, said := sayC23(t, withReadback(t, decl, dropped)); c.Status != model.CheckFailed || !strings.Contains(said, "k of") || !strings.Contains(said, "not read back") {
		t.Errorf("C23 with the preset's entry dropped: %s %s, want failed", c.Status, said)
	}
	changed := func(k plugin.Entry) []plugin.Entry {
		k.Value = canonInt(2)
		return []plugin.Entry{k, {Key: "dflt", On: true, Value: canonInt(1)}}
	}
	if c, said := sayC23(t, withReadback(t, decl, changed)); c.Status != model.CheckFailed || !strings.Contains(said, "another value") {
		t.Errorf("C23 with the preset's entry changed: %s %s, want failed", c.Status, said)
	}
}

// TestTheReadbackSeverityFalseComparesNoSeverityAndRefusesOneRead: a plugin
// that says it places no severity is not held to the preset's, and is red when
// an entry it reads back carries one; with the key out, severity is compared.
func TestTheReadbackSeverityFalseComparesNoSeverityAndRefusesOneRead(t *testing.T) {
	t.Parallel()
	bare := func(k plugin.Entry) []plugin.Entry { k.Severity = nil; return []plugin.Entry{k} }
	placed := func(k plugin.Entry) []plugin.Entry { return []plugin.Entry{k} }
	const no = "    severity: false\n"
	if c, said := sayC23(t, withReadback(t, no, bare)); c.Status != model.CheckPassed {
		t.Errorf("C23 with severity false and none read back: %s %s, want passed", c.Status, said)
	}
	if c, said := sayC23(t, withReadback(t, no, placed)); c.Status != model.CheckFailed || !strings.Contains(said, "places none") {
		t.Errorf("C23 with severity false and one read back: %s %s, want failed", c.Status, said)
	}
	if c, said := sayC23(t, withReadback(t, "    severity: true\n", bare)); c.Status != model.CheckFailed || !strings.Contains(said, "another value, options or severity") {
		t.Errorf("C23 with severity true and none read back: %s %s, want failed", c.Status, said)
	}
	if c, said := sayC23(t, withReadback(t, "    defaults: [dflt]\n", bare)); c.Status != model.CheckFailed {
		t.Errorf("C23 with defaults alone and no severity read back: %s %s, want failed", c.Status, said)
	}
}

// filesReadPlugin is a plugin shaped like ast-grep's: it reads files, never a
// run. Given a read result in have it reads those files (rules/a.yml, as the
// recording's) and answers their entries, as the read-back's old fault had it;
// given none it reads the config it is sent, and answers the entries of those
// files, less the key drop, when drop names one.
func filesReadPlugin(drop string) *porttest.StandinPlugin {
	p := standin()
	p.OnPlan = planReading("**")
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		if in.Target != plugin.TargetSource || !inRange(in.ToolVersion) {
			return plugin.EntriesResponse{}, unsupported()
		}
		// As the declarative SDK's entriesFromFiles: the files come from the read
		// results of have and from no other place, the config is not read.
		found := false
		for _, r := range in.Have {
			if r.Kind != plugin.KindRead {
				continue
			}
			for _, f := range r.Files {
				if f.Path == "rules/a.yml" {
					return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "from-rules-a", On: true, Value: canonInt(1)}}}, nil
				}
				if f.Path == "p.yml" && f.Data.Text != nil && strings.Contains(*f.Data.Text, "k: 1") {
					found = true
				}
			}
		}
		if !found || drop == "k" {
			return plugin.EntriesResponse{Entries: []plugin.Entry{}}, nil
		}
		sev := plugin.Severity("error")
		return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "k", On: true, Severity: &sev, Value: canonInt(1)}}}, nil
	}
	return p
}

// readOf is the recorded result of reading another rule file, as a files-read
// plugin's recording holds it beside or in place of a run.
const readOf = `{"cut":false,"files":[{"data":{"text":"id: from-rules-a\n"},"path":"rules/a.yml"}],"kind":"read","round":0,"step":0}`

// withReadRecording is a bundle whose recorded have holds the result of
// reading another rule file and no run, as a files-read plugin's does.
func withReadRecording(t *testing.T, p *porttest.StandinPlugin) plugintest.Bundle {
	t.Helper()
	b := full()
	b.Plugin = opened(t, p)
	for i, c := range b.Cases {
		if c.Name == "1-entries" {
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), ran, readOf, 1))
		}
	}
	return resave(b)
}

// TestTheRoundTripGivesAFilesReadPluginNoRecordedRead: a plugin that reads
// files reads the preset's files in the read-back, never the recording's reads
// of other files; and it is still red on a preset entry it does not read back.
func TestTheRoundTripGivesAFilesReadPluginNoRecordedRead(t *testing.T) {
	t.Parallel()
	if got := run(t, withReadRecording(t, filesReadPlugin(""))); got[23].Status != model.CheckPassed {
		t.Errorf("C23 for a files-read plugin that reads the preset back: %s %v, want passed", got[23].Status, got[23].What)
	}
	got := run(t, withReadRecording(t, filesReadPlugin("k")))
	if got[23].Status != model.CheckFailed || !strings.Contains(strings.Join(got[23].What, "\n"), "not read back") {
		t.Errorf("C23 with a preset entry missing: %s %v, want failed, not read back", got[23].Status, got[23].What)
	}
}

// TestTheRoundTripKeepsTheReadOfTheFilesInsideTheContract: a recording whose
// last run is in round 2 gets the preset's files in round 2 at the next step,
// never in a round the contract does not have, and the read has
// no cut member, as the engine's read has none.
func TestTheRoundTripKeepsTheReadOfTheFilesInsideTheContract(t *testing.T) {
	t.Parallel()
	p := filesReadPlugin("")
	inner := p.OnEntries
	var seen []plugin.Result
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		for _, r := range in.Have {
			if r.Kind == plugin.KindRead && len(r.Files) > 0 && r.Files[0].Path == "p.yml" {
				seen = append(seen, r)
			}
		}
		return inner(in)
	}
	b := full()
	b.Plugin = opened(t, p)
	for i, c := range b.Cases {
		if c.Name == "1-entries" {
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), ran, strings.Replace(ran, `"round":0`, `"round":2`, 1)+","+readOf, 1))
		}
	}
	if got := run(t, resave(b)); got[23].Status != model.CheckPassed {
		t.Fatalf("C23 for a recording with a run in round 2: %s %v, want passed", got[23].Status, got[23].What)
	}
	if len(seen) == 0 {
		t.Fatal("the plugin was handed no read of the preset's files")
	}
	for _, r := range seen {
		if r.Round != 2 || r.Step != 1 || r.Cut != nil {
			t.Errorf("the read of the files is at round %d step %d with cut %v, want round 2 step 1 and no cut", r.Round, r.Step, r.Cut)
		}
	}
}

// TestTheRoundTripSaysWhenNoStepIsLeftForTheFiles: a recording whose last run
// is the last step of the last round leaves no place for the read of the
// preset's files, and C23 is skipped with that reason, not failed.
func TestTheRoundTripSaysWhenNoStepIsLeftForTheFiles(t *testing.T) {
	t.Parallel()
	b := full()
	b.Plugin = opened(t, filesReadPlugin(""))
	for i, c := range b.Cases {
		if c.Name == "1-entries" {
			last := strings.Replace(strings.Replace(ran, `"round":0`, `"round":2`, 1), `"step":0`, `"step":15`, 1)
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), ran, last+","+readOf, 1))
		}
	}
	got := run(t, resave(b))
	if got[23].Status != model.CheckNotRun || !strings.Contains(strings.Join(got[23].What, "\n"), "no step is left") {
		t.Errorf("C23 with a run at round 2 step 15: %s %v, want skipped, no step is left", got[23].Status, got[23].What)
	}
}

// TestARunPluginIsGivenThePresetOnceInItsConfig: a plugin whose recording holds
// runs and no read reads no files, so the read-back gives the preset in its
// config alone and adds no read result, and a preset over half the entries cap
// is not counted twice. A files-read plugin, whose recording
// holds a read, is given the files as a read. Planted: the read added whatever
// the recording held (the run row red: it is handed a read, and the preset
// twice).
func TestARunPluginIsGivenThePresetOnceInItsConfig(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name      string
		recording string
		wantRead  bool
	}{
		{"a recorded run", ran, false},
		{"a recorded run and a read", ran + "," + readOf, true},
	} {
		p := standin()
		inner := p.OnEntries
		var configBytes, readBytes int
		var reads int
		p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			if in.Target == plugin.TargetSource && len(in.Config) > 0 {
				configBytes, readBytes, reads = 0, 0, 0
				for _, f := range in.Config {
					if f.Data != nil && f.Data.Text != nil {
						configBytes += len(*f.Data.Text)
					}
				}
				for _, r := range in.Have {
					if r.Kind == plugin.KindRead {
						reads++
						for _, f := range r.Files {
							if f.Data.Text != nil {
								readBytes += len(*f.Data.Text)
							}
						}
					}
				}
			}
			return inner(in)
		}
		p.OnPlan = planReading("rules/*.yml")
		b := full()
		b.Plugin = opened(t, p)
		big := strings.Repeat("#", 40<<10)
		for i, k := range b.Cases {
			switch k.Name {
			case "1-entries":
				b.Cases[i].Request = []byte(strings.Replace(string(k.Request), ran, c.recording, 1))
			case "7-preset":
				b.Cases[i].Response = []byte(strings.Replace(string(k.Response), "# MARK\\nk: 1\\n", "# MARK\\n"+big+"\\nk: 1\\n", 1))
			}
		}
		run(t, resave(b))
		if configBytes < 40<<10 {
			t.Errorf("%s: the plugin was handed %d bytes of config, want the preset of over 40 KiB in it", c.name, configBytes)
		}
		if got := reads > 0; got != c.wantRead {
			t.Errorf("%s: the plugin was handed %d read results, want a read: %v", c.name, reads, c.wantRead)
		}
		if !c.wantRead && readBytes != 0 {
			t.Errorf("%s: the preset was handed over twice, %d bytes in a read beside %d in the config", c.name, readBytes, configBytes)
		}
	}
}

// planReading is a plan that asks for one read of glob and is final.
func planReading(glob string) func(plugin.PlanRequest) (plugin.PlanResponse, error) {
	return func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		return plugin.PlanResponse{Steps: []plugin.Step{{Read: &plugin.Read{Glob: glob}}}, Final: true}, nil
	}
}

// TestAPresetTheReadGlobMatchesIsHandedOverOnce: a files-read plugin whose
// glob matches a preset of over 40 KiB is handed it once, in the read and not
// in the config, as the engine hands it (gragas#2375). Planted: the matched
// file left in the config as well (the config row red: 40 KiB twice).
func TestAPresetTheReadGlobMatchesIsHandedOverOnce(t *testing.T) {
	t.Parallel()
	p := standin()
	inner := p.OnEntries
	var configBytes, readBytes int
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		if in.Target == plugin.TargetSource && len(in.Have) > 1 {
			configBytes, readBytes = 0, 0
			for _, f := range in.Config {
				if f.Data != nil && f.Data.Text != nil {
					configBytes += len(*f.Data.Text)
				}
			}
			for _, r := range in.Have {
				if r.Kind == plugin.KindRead {
					for _, f := range r.Files {
						if f.Data.Text != nil {
							readBytes += len(*f.Data.Text)
						}
					}
				}
			}
		}
		return inner(in)
	}
	p.OnPlan = planReading("p.yml")
	b := full()
	b.Plugin = opened(t, p)
	big := strings.Repeat("#", 40<<10)
	for i, k := range b.Cases {
		switch k.Name {
		case "1-entries":
			b.Cases[i].Request = []byte(strings.Replace(string(k.Request), ran, ran+","+readOf, 1))
		case "7-preset":
			b.Cases[i].Response = []byte(strings.Replace(string(k.Response), "# MARK\\nk: 1\\n", "# MARK\\n"+big+"\\nk: 1\\n", 1))
		}
	}
	run(t, resave(b))
	if readBytes < 40<<10 {
		t.Errorf("the read held %d bytes, want the preset of over 40 KiB once", readBytes)
	}
	if configBytes != 0 {
		t.Errorf("the config held %d bytes beside the read: the preset was handed over twice", configBytes)
	}
}

// TestAFilesReadPluginIsHandedOnlyTheFilesItsPlanReads: C23 asks the plugin's
// plan for its reads, matches the globs with the contract's matcher, and hands
// the files they match over as the read, with the whole preset in the config
// as the engine sends it; a plan that asks for no read gets no read result.
// Planted: every file the read, whatever the plan
// asked (the "rules/**" row red), or the matched files taken out of the config
// (the rows that match red).
func TestAFilesReadPluginIsHandedOnlyTheFilesItsPlanReads(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name        string
		glob        string
		wantConfig  int
		wantResults int
		wantFiles   int
	}{
		{"a glob of the root's files hands over the file", "*.yml", 1, 1, 1},
		{"a path hands over the file", "p.yml", 1, 1, 1},
		{"a folder the preset has no file in hands over none", "rules/**", 1, 1, 0},
		{"a glob of every folder hands over the file", "**/*.yml", 1, 1, 1},
		{"a plan that asks for no read gets no read result", "", 1, 0, 0},
	} {
		p := filesReadPlugin("")
		inner := p.OnEntries
		var inConfig, inResults, inFiles int
		p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			if in.Target == plugin.TargetSource && inRange(in.ToolVersion) {
				inConfig, inResults, inFiles = len(in.Config), 0, 0
				for _, r := range in.Have {
					if r.Kind == plugin.KindRead {
						inResults++
						inFiles += len(r.Files)
					}
				}
			}
			return inner(in)
		}
		p.OnPlan = planReading(c.glob)
		if c.glob == "" {
			p.OnPlan = func(plugin.PlanRequest) (plugin.PlanResponse, error) { return plugin.PlanResponse{Final: true}, nil }
		}
		run(t, withReadRecording(t, p))
		if inConfig != c.wantConfig || inResults != c.wantResults || inFiles != c.wantFiles {
			t.Errorf("%s: a plan reading %q: %d files in the config, %d read results holding %d files, want %d, %d and %d",
				c.name, c.glob, inConfig, inResults, inFiles, c.wantConfig, c.wantResults, c.wantFiles)
		}
	}
}

// TestAPresetTheReadTakesWholeFailsAsInTheEngine: the engine sends the whole
// config beside a read, so a plugin whose read takes a preset of over half the
// entries cap is refused there, and C23 fails it here and does not pass it.
// Planted: the matched files taken out of the config (this row passes, as the old split did).
func TestAPresetTheReadTakesWholeFailsAsInTheEngine(t *testing.T) {
	t.Parallel()
	p := filesReadPlugin("")
	p.OnPlan = planReading("p.yml")
	inner := p.OnEntries
	// The stand-in applies no cap, so it refuses as the codec does: an entries
	// request of the config and the read files over wire.MaxEntriesInput.
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		size := 0
		for _, f := range in.Config {
			if f.Data != nil && f.Data.Text != nil {
				size += len(*f.Data.Text)
			}
		}
		for _, r := range in.Have {
			for _, f := range r.Files {
				if f.Data.Text != nil {
					size += len(*f.Data.Text)
				}
			}
		}
		if size > 64<<10 {
			return plugin.EntriesResponse{}, &plugin.LimitError{What: plugin.LimitEntries, Size: size, Cap: 64 << 10}
		}
		return inner(in)
	}
	b := withReadRecording(t, p)
	big := strings.Repeat("#", 40<<10)
	for i, k := range b.Cases {
		if k.Name == "7-preset" {
			b.Cases[i].Response = []byte(strings.Replace(string(k.Response), "# MARK\\nk: 1\\n", "# MARK\\n"+big+"\\nk: 1\\n", 1))
		}
	}
	got := run(t, resave(b))
	if got[23].Status != model.CheckFailed {
		t.Errorf("C23 for a plugin whose read takes a 40 KiB preset whole: %s %v, want failed, as the engine refuses it", got[23].Status, got[23].What)
	}
}

// TestARunOnlyRecordingAtTheLastStepIsNotSkipped: a recording of a run alone
// at round 2 step 15 adds no read, so no place is needed for one and C23 is
// not skipped for want of a step. Planted: the
// skip held whatever the recording held (this row red).
func TestARunOnlyRecordingAtTheLastStepIsNotSkipped(t *testing.T) {
	t.Parallel()
	b := full()
	b.Plugin = opened(t, standin())
	for i, c := range b.Cases {
		if c.Name == "1-entries" {
			last := strings.Replace(strings.Replace(ran, `"round":0`, `"round":2`, 1), `"step":0`, `"step":15`, 1)
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), ran, last, 1))
		}
	}
	got := run(t, resave(b))
	if got[23].Status == model.CheckNotRun {
		t.Errorf("C23 with only a run at round 2 step 15: %s %v, want it not skipped", got[23].Status, got[23].What)
	}
}

// TestThePlanIsAskedAsTheEngineAsksIt: the engine asks round r's plan with
// only the results of the rounds before r, so a recorded run in round 0 leaves
// the plan of round 0 an empty have. The plan
// here names the preset's file only for such a request. Planted: the plan
// asked with the whole recorded have (this row red: the glob misses, so the
// read result holds no file; p.yml is in the config either way).
func TestThePlanIsAskedAsTheEngineAsksIt(t *testing.T) {
	t.Parallel()
	p := filesReadPlugin("")
	inner := p.OnEntries
	var inConfig, inRead int
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		if in.Target == plugin.TargetSource && inRange(in.ToolVersion) {
			inConfig, inRead = len(in.Config), 0
			for _, r := range in.Have {
				if r.Kind == plugin.KindRead {
					inRead += len(r.Files)
				}
			}
		}
		return inner(in)
	}
	p.OnPlan = func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		glob := "none/**"
		if in.Round == 0 && len(in.Have) == 0 {
			glob = "p.yml"
		}
		return planReading(glob)(in)
	}
	b := full()
	b.Plugin = opened(t, p)
	for i, c := range b.Cases {
		if c.Name == "1-entries" {
			b.Cases[i].Request = []byte(strings.Replace(string(c.Request), ran, ran+","+readOf, 1))
		}
	}
	run(t, resave(b))
	if inConfig != 1 || inRead != 1 {
		t.Errorf("a plan that reads p.yml for round 0 with no have: %d files in the config and %d in the reads, want 1 and 1", inConfig, inRead)
	}
}
