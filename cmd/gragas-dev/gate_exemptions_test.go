package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/project"
)

// fakeToday is the day fakeMachine judges an expiry on: every row of the
// committed gate/exemptions.yaml is ahead of it.
const fakeToday = "2026-11-14"

// tiers are the tiers that run gragas-dev gate check, by their name in a
// subtest. The landing tier's --land went with the landing check
// (gragas#912): the lander's merged-tree check runs pre-push and then full
// whole, and the gate check is a member of full (gragas#1619).
func tiers() []string { return []string{"full"} }

// exemptionsGate is the gate over a copy of the committed registry, with
// plant run on the copy and the day today gives (none when today is nil).
func exemptionsGate(t *testing.T, today func() string, plant func(dir string)) (int, string) {
	t.Helper()
	dir := copyGate(t)
	m := fakeMachine(t, dir)
	m.today = today
	if plant != nil {
		plant(dir)
	}
	var o, e bytes.Buffer
	code := runWith(t.Context(), m, []string{"gate", "check", "-C", dir}, &o, &e)
	return code, o.String() + e.String()
}

// day is a clock that says d.
func day(d string) func() string { return func() string { return d } }

// TestTheGateHoldsTheExemptionsItself: gragas-dev reads gate/exemptions.yaml
// and judges each expiry with its own clock, prints the exemptions' line for
// the tier's report, and fails on each finding (gates.exemptions.2,
// gates.exemptions.4, gates.ratchets.1). Risk: a tier that never judges the
// file.
func TestTheGateHoldsTheExemptionsItself(t *testing.T) {
	t.Parallel()
	for _, tier := range tiers() {
		t.Run(tier+", the committed file is a line and a pass", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day(fakeToday), nil)
			if code != exitDone || !strings.Contains(out, reportMark+"exemptions: ") || strings.Contains(out, "next expiry none") {
				t.Errorf("exit %d, want %d and the exemptions' line\n%s", code, exitDone, out)
			}
		})
		t.Run(tier+", a day past every expiry fails on each row", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day("2099-01-01"), nil)
			if code != exitFailed || !strings.Contains(out, reportMark+"exemptions: ") || !strings.Contains(out, "next expiry none\n") || !strings.Contains(out, "\n"+project.ExemptionsFile+":") || !strings.Contains(out, " expired on ") {
				t.Errorf("exit %d, want %d naming an expired row\n%s", code, exitFailed, out)
			}
		})
		t.Run(tier+", a row that expired yesterday fails", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day(fakeToday), func(dir string) {
				rewrite(t, dir, project.ExemptionsFile, func(s string) string {
					return strings.Replace(s, `expires: "2027-03-31"`, `expires: "2026-11-13"`, 1)
				})
			})
			if code != exitFailed || !strings.Contains(out, " expired on 2026-11-13: retire it or renew it") {
				t.Errorf("exit %d, want %d naming the row\n%s", code, exitFailed, out)
			}
		})
		t.Run(tier+", no clock fails, never passes", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, nil, nil)
			if code != exitFailed || !strings.Contains(out, project.ExemptionsFile+": no date was given, so no expiry can be judged") {
				t.Errorf("exit %d, want %d saying no day was given\n%s", code, exitFailed, out)
			}
		})
		t.Run(tier+", a clock that names no day fails", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day("2026-02-30"), nil)
			if code != exitFailed || !strings.Contains(out, "no date was given") {
				t.Errorf("exit %d, want %d\n%s", code, exitFailed, out)
			}
		})
		t.Run(tier+", a missing file is a finding of its own", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day(fakeToday), func(dir string) {
				if err := os.Remove(filepath.Join(dir, filepath.FromSlash(project.ExemptionsFile))); err != nil {
					t.Fatal(err)
				}
			})
			if code != exitFailed || !strings.Contains(out, project.ExemptionsFile+": cannot be read, so no expiry is judged") {
				t.Errorf("exit %d, want %d saying the file cannot be read\n%s", code, exitFailed, out)
			}
		})
		t.Run(tier+", an exemptions that is not a list is a finding", func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day(fakeToday), func(dir string) {
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(project.ExemptionsFile)), []byte("exemptions: oops\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			})
			if code != exitFailed || !strings.Contains(out, "exemptions is not a list of rows, so no row could be judged") {
				t.Errorf("exit %d, want %d\n%s", code, exitFailed, out)
			}
		})
	}
}

// TestAnExemptionsFindingIsOneLineEscaped: a row's id is the head's text, so
// a finding that names it is printed as one line, escaped as every line the
// registry prints is, and can forge no line of the report.
func TestAnExemptionsFindingIsOneLineEscaped(t *testing.T) {
	t.Parallel()
	for _, tier := range tiers() {
		t.Run(tier, func(t *testing.T) {
			t.Parallel()
			code, out := exemptionsGate(t, day("2099-01-01"), func(dir string) {
				rewrite(t, dir, project.ExemptionsFile, func(s string) string {
					return strings.Replace(s, "id: cli-look-5", `id: "a\nthe gate registry holds\e[2K\u202e"`, 1)
				})
			})
			if code != exitFailed || strings.Contains(out, "\nthe gate registry holds") || strings.ContainsAny(out, "\x1b\u202e") {
				t.Fatalf("exit %d, a forged line is its own\n%s", code, out)
			}
			if want := `a\nthe gate registry holds\x1b[2K\u202e expired on 2027-03-31`; !strings.Contains(out, want) {
				t.Errorf("no line holds the id escaped (%q)\n%s", want, out)
			}
		})
	}
}

// TestTheExemptionsLineIsPrintable: the line holds nothing past printable
// ASCII, and a row no reader can see is never counted as judged.
func TestTheExemptionsLineIsPrintable(t *testing.T) {
	t.Parallel()
	got := holdExemptions(os.DirFS(t.TempDir()), fakeToday)
	if len(got.findings) != 1 || strings.ContainsFunc(got.line+strings.Join(got.findings, ""), func(r rune) bool { return r < 0x20 || r >= 0x7f }) {
		t.Errorf("an empty folder holds %+v", got)
	}
}

// TestAGateOverNoRuleFilesSaysTheRuleFilesAreNotRead: a rulebook folder with no
// rule file in it is one finding of its own, "the rule files are not read", and
// the gate fails; with the finding gone every row would only look unused, which
// an emptied exemptions file does not show (gragas#1739). Risk: a gate that
// reads no rule and says nothing about it.
func TestAGateOverNoRuleFilesSaysTheRuleFilesAreNotRead(t *testing.T) {
	t.Parallel()
	code, out := exemptionsGate(t, day(fakeToday), func(dir string) {
		if err := os.RemoveAll(filepath.Join(dir, "rulebook")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "rulebook", "base"), 0o750); err != nil {
			t.Fatal(err)
		}
	})
	if code != exitFailed || !strings.Contains(out, "rulebook: the rule files are not read, so every exemption is unused: ") {
		t.Errorf("exit %d, want %d and the line \"rulebook: the rule files are not read\"\n%s", code, exitFailed, out)
	}
}
