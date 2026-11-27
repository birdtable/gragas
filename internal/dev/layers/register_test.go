package layers_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// library is a register of one chosen row that bans an import and a call, one row that needs a spike, a tool and
// a dropped name.
const library = `part: go
libraries:
  - key: yaml
    job: read and write YAML
    use:
      module: go.yaml.in/yaml/v3
      version: v3.0.6
      released: "2026-01-01"
      package: anywhere a program reads YAML
    scope: anywhere
    replaces:
      - {name: gopkg.in/yaml.v3, why: archived upstream}
    by_hand:
      - a strict YAML reader
    ban:
      deny: [gopkg.in/yaml.v3, github.com/go-yaml/yaml]
      forbid:
        - {pattern: '^yaml\.Unmarshal$', pkg: gopkg.in/yaml.v3}
      linter: [depguard, forbidigo]
    gap: no unique-keys option
    source: https://github.com/yaml/go-yaml
    state: chosen
  - key: path-lock
    job: one holder per path
    use:
      module: github.com/gofrs/flock
      version: v0.13.2
      released: "2026-11-08"
      age: "2026-11-15"
      package: the one lock package
    scope: one package per program
    replaces:
      - {name: github.com/juju/fslock, why: archived}
    ban:
      deny: [github.com/juju/fslock]
      linter: depguard
    gap: the holder note stays ours
    source: https://github.com/gofrs/flock
    state: needs-spike
tools:
  - key: govulncheck
    version: v1.9.0
    tools_yaml: "rulebook/lang/go/tools.yaml, key govulncheck"
    replaces: a hand-written vulnerability scan
    source: https://github.com/golang/vuln
dropped:
  - {name: osv-scanner, by: govulncheck}
`

func noRegisterProblems(any) []string { return nil }

func register(t *testing.T, text string) (*layers.Register, error) {
	t.Helper()
	return layers.ParseRegister([]byte(text), noRegisterProblems)
}

// TestTheRegisterReadsItsRowsAndFeedsThePresetFromTheChosenOnes: rows, a tool and a dropped name read with their
// fields, the feed is the chosen row that bans something and not the row that needs a spike, and a row with only
// a planned pattern or a conditional state feeds none.
//
// What it costs when this fails: a row read wrongly, or a spike row fed to the preset, puts a ban in every repo's
// lint for a library nobody has proven.
func TestTheRegisterReadsItsRowsAndFeedsThePresetFromTheChosenOnes(t *testing.T) {
	t.Parallel()
	r, err := register(t, library)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 2 || len(r.Tools) != 1 || len(r.Dropped) != 1 {
		t.Fatalf("read %d rows, %d tools and %d dropped, want 2, 1 and 1", len(r.Rows), len(r.Tools), len(r.Dropped))
	}
	if got := r.Rows[0]; got.Module != "go.yaml.in/yaml/v3" || got.State != layers.StateChosen || len(got.Deny) != 2 || len(got.Forbid) != 1 || got.Line == 0 {
		t.Errorf("the yaml row read as %+v", got)
	}
	feed := r.Feed()
	if len(feed) != 1 || feed[0].Key != "yaml" || !slices.Equal(feed[0].Deny, []string{"gopkg.in/yaml.v3", "github.com/go-yaml/yaml"}) || feed[0].Forbid[0].Pkg != "gopkg.in/yaml.v3" {
		t.Errorf("the feed is %+v, want the yaml row alone with its two denied imports and its call", feed)
	}
	planned := strings.Replace(library, "      forbid:\n        - {pattern: '^yaml\\.Unmarshal$', pkg: gopkg.in/yaml.v3}\n", "      planned: ['^unix\\.Openat2$']\n", 1)
	planned = strings.Replace(planned, "      deny: [gopkg.in/yaml.v3, github.com/go-yaml/yaml]\n", "", 1)
	planned = strings.Replace(planned, "      linter: [depguard, forbidigo]\n", "", 1)
	planned = strings.Replace(planned, "    state: chosen\n", "    state: chosen\n    open: [a planted fixture proves the pattern]\n    \n", 1)
	planned = strings.Replace(planned, "    ban:\n      planned", "    ban:\n      note: unproven\n      planned", 1)
	p, err := register(t, planned)
	if err != nil {
		t.Fatalf("a row with only a planned pattern: %v", err)
	}
	if len(p.Feed()) != 0 {
		t.Errorf("a chosen row with only a planned pattern fed the preset: %+v", p.Feed())
	}
	conditional := strings.Replace(library, "    state: needs-spike\n", "    state: conditional\n    proof: the spike passes on Windows\n", 1)
	c, err := register(t, conditional)
	if err != nil || c.Rows[1].Proof == "" || len(c.Feed()) != 1 {
		t.Errorf("a conditional row: %v, proof %q, feed %d", err, c.Rows[1].Proof, len(c.Feed()))
	}
}

// TestTheRegisterRefusesARowThatDoesNotHold: a row with no source, with a source that is ours, with a state it
// does not know, a conditional row with no proof, a ban pattern that does not compile, a row that replaces nothing and
// bans nothing with no note, a duplicate key, a key it does not know, and a dropped name that has a row each
// fail with ErrRegister and say what.
//
// What it costs when this fails: a row with no source, an unripe pin or a ban that never compiles reaches every
// repo's lint, and the register stops being the one list a library is chosen from.
func TestTheRegisterRefusesARowThatDoesNotHold(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, text, says string }{
		{"no source", strings.Replace(library, "    source: https://github.com/yaml/go-yaml\n", "", 1), "source is missing"},
		{"a source that is not https", strings.Replace(library, "https://github.com/yaml/go-yaml", "http://example.com/x", 1), "is not an https address"},
		{"a source that is an audit of ours", strings.Replace(library, "https://github.com/yaml/go-yaml", "https://example.com/docs/audits/2026/libraries.md", 1), "names audit"},
		{"a source in the owner's repository", strings.Replace(library, "https://github.com/yaml/go-yaml", "https://github.com/birdtable/x/issues/1", 1), "is a repository of the owner"},
		{"a state it does not know", strings.Replace(library, "    state: chosen\n", "    state: maybe\n", 1), "is not one of chosen, needs-spike, conditional"},
		{"an age with no release date", strings.Replace(library, "      released: \"2026-01-01\"\n", "      age: \"2026-11-20\"\n", 1), "needs the release's date"},
		{"a version that is none of the forms", strings.Replace(library, "version: v3.0.6", "version: as listed", 1), "is a release (v1.2.3), floor, toolchain or none"},
		{"a conditional row with no proof", strings.Replace(library, "    state: needs-spike\n", "    state: conditional\n", 1), "names the proof it waits for"},
		{"a proof on a row that is not conditional", strings.Replace(library, "    state: chosen\n", "    state: chosen\n    proof: x\n", 1), "only a conditional row has a proof"},
		{"a pattern that does not compile", strings.Replace(library, "'^yaml\\.Unmarshal$'", "'^yaml\\.(Unmarshal$'", 1), "does not compile as a Go regexp"},
		{"a row that bans its own library", strings.Replace(library, "deny: [gopkg.in/yaml.v3, github.com/go-yaml/yaml]", "deny: [go.yaml.in/yaml/v3]", 1), "denies its own library"},
		{"a row that replaces nothing", strings.Replace(library, "    replaces:\n      - {name: gopkg.in/yaml.v3, why: archived upstream}\n    by_hand:\n      - a strict YAML reader\n", "", 1), "says what it replaces"},
		{"a row that bans nothing and says nothing", strings.Replace(library, "    ban:\n      deny: [github.com/juju/fslock]\n      linter: depguard\n", "    ban: {}\n", 1), "says in ban.note why it is review only"},
		{"a key written twice", strings.Replace(library, "  - key: path-lock\n", "  - key: yaml\n", 1), "is there twice"},
		{"a key it does not know", strings.Replace(library, "    scope: anywhere\n", "    scope: anywhere\n    colour: blue\n", 1), "colour is not a key"},
		{"a dropped name that has a row", strings.Replace(library, "{name: osv-scanner,", "{name: yaml,", 1), "is dropped and has a row"},
		{"a dropped name a row calls", strings.Replace(library, "{name: osv-scanner,", "{name: go.yaml.in/yaml/v3,", 1), "is dropped and row yaml calls it"},
		{"a ban that names no linter", strings.Replace(library, "      deny: [github.com/juju/fslock]\n      linter: depguard\n", "      deny: [github.com/juju/fslock]\n", 1), "names no ban.linter that holds it"},
		{"a linter it does not know", strings.Replace(library, "      linter: depguard\n", "      linter: grep\n", 1), "none of depguard, forbidigo, gomodguard_v2 and gomoddirectives"},
		{"a denied import held by forbidigo alone", strings.Replace(library, "      linter: depguard\n", "      linter: forbidigo\n", 1), "names neither depguard nor gomodguard_v2"},
		{"a linter named twice", strings.Replace(library, "linter: [depguard, forbidigo]", "linter: [depguard, depguard, forbidigo]", 1), "names depguard twice"},
		{"an alias", strings.Replace(library, "  - key: path-lock\n", "  - &k {key: x}\n  - key: path-lock\n", 1), "the anchor &k lets another place stand for this value"},
	}
	ran := 0
	for _, tc := range cases {
		ran++
		if tc.text == library {
			t.Errorf("%s: the plant changed nothing", tc.name)
			continue
		}
		_, err := register(t, tc.text)
		if !errors.Is(err, layers.ErrRegister) || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: ParseRegister = %v, want ErrRegister saying %q", tc.name, err, tc.says)
		}
	}
	if ran != len(cases) || ran < 20 {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
	if _, err := layers.ParseRegister([]byte(library), nil); !errors.Is(err, layers.ErrRegister) {
		t.Errorf("with no schema check ParseRegister = %v, want ErrRegister", err)
	}
	if _, err := register(t, library); err != nil {
		t.Errorf("the unplanted register: %v", err)
	}
	if _, err := layers.ParseRegister([]byte(library), func(any) []string { return []string{"line 1: planted"} }); err == nil || !strings.Contains(err.Error(), "planted") {
		t.Errorf("a schema problem was not reported: %v", err)
	}
}

// TestAYoungPinIsReadWithNoAgeLine: no release-age window is held, so a chosen row whose pin is 4
// days old is read with no age line.
func TestAYoungPinIsReadWithNoAgeLine(t *testing.T) {
	t.Parallel()
	young := strings.Replace(library, "released: \"2026-01-01\"", "released: \"2026-11-10\"", 1)
	if _, err := register(t, young); err != nil {
		t.Errorf("a pin of 4 days with no age line: %v", err)
	}
}

// TestADateOfARowIsInQuotes: an unquoted date is refused, since the reader hands it over as no string and the
// age window would then be counted from nothing.
//
// What it costs when this fails: a young pin is read as having no release date, and the window it must wait out
// is never counted.
func TestADateOfARowIsInQuotes(t *testing.T) {
	t.Parallel()
	young := strings.Replace(library, "released: \"2026-01-01\"", "released: 2026-11-10", 1)
	if _, err := register(t, young); err == nil || !strings.Contains(err.Error(), "is a date in quotes") {
		t.Errorf("an unquoted date: %v, want a refusal saying to quote it", err)
	}
}

// TestEachGuardOfTheRegisterLoaderRefusesItsPlant: one planted file for each remaining refusal of the loader, each
// answering ErrRegister with its own words: a text that is no mapping, a register with no row, a tool or a row written
// twice, a key that is no key, an empty job, scope or gap, a planned pattern with no open, a use that is no mapping, a
// module that is no path, a use with no package, a date on a pin that is no release, a date that is no date, a
// replaced module with no name or no why, a ban that is no mapping, an import denied twice or that is no path, a
// forbidden call that is no pattern, a planned pattern that does not compile, an item that is no one-line string, a tool
// with a key, a tools_yaml, a version or a source that is wrong, a dropped name that is empty or has a tool row, an
// unexpanded placeholder, and a chosen pin with no release date.
//
// What it costs when this fails: a loader guard is taken out and every test stays green, so a file with the fault
// it guarded reaches the preset.
func TestEachGuardOfTheRegisterLoaderRefusesItsPlant(t *testing.T) {
	t.Parallel()
	rep := func(old, repl string) string { return strings.Replace(library, old, repl, 1) }
	tool := "    version: v1.9.0\n    tools_yaml: \"rulebook/lang/go/tools.yaml, key govulncheck\"\n    replaces: r\n    source: https://github.com/golang/vuln\n"
	cases := []struct{ name, text, says string }{
		{"a text that is no mapping", "- a\n", "not a mapping"},
		{"a register with no row", "part: go\nlibraries: []\ndropped: []\n", "libraries holds no row"},
		{"a tool written twice", rep("dropped:", "  - key: govulncheck\n"+tool+"dropped:"), "tool govulncheck is there twice"},
		{"a key that is no key", rep("- key: yaml\n", "- key: Yaml\n"), "is not lower-case words joined by dashes"},
		{"an empty job", rep("job: read and write YAML", "job: ''"), "job is empty or holds a control character"},
		{"an empty scope", rep("    scope: anywhere\n", "    scope: ''\n"), "scope is empty or holds a control character"},
		{"an empty gap", rep("    gap: no unique-keys option", "    gap: ''"), "gap is empty or holds a control character"},
		{"a planned pattern with no open", rep("      deny: [github.com/juju/fslock]\n", "      deny: [github.com/juju/fslock]\n      planned: ['^x$']\n"), "a planned pattern says in open what proves it"},
		{"a use that is no mapping", rep("    use:\n      module: github.com/gofrs/flock\n", "    use: x\n    unused:\n      module: github.com/gofrs/flock\n"), "use is not a module, a version and a package"},
		{"a module that is no path", rep("module: go.yaml.in/yaml/v3", "module: 'not a path!'"), `module "not a path!" is not an import path`},
		{"a use with no package", rep("package: anywhere a program reads YAML", "package: ' '"), "use names no package"},
		{"a date on a pin that is no release", rep("version: v3.0.6\n", "version: floor\n"), "only a release (v1.2.3) has a date"},
		{"a release date that is no date", rep("released: \"2026-01-01\"", "released: \"yesterday\""), `released "yesterday" is not a date`},
		{"an age that is no date", rep("released: \"2026-01-01\"", "released: \"2026-01-01\"\n      age: \"soon\""), `age "soon" is not a date`},
		{"a replaced module with no name", rep("{name: gopkg.in/yaml.v3, why: archived upstream}", "{name: ' ', why: archived upstream}"), "a replaced module has no name"},
		{"a replaced module that is no name and why", rep("      - {name: gopkg.in/yaml.v3, why: archived upstream}\n", "      - just a string\n"), "a replaced module is not a name and a why"},
		{"a ban that is no mapping", rep("    ban:\n      deny: [github.com/juju/fslock]\n      linter: depguard\n", "    ban: x\n"), "ban is not a mapping"},
		{"an import denied twice", rep("deny: [github.com/juju/fslock]", "deny: [github.com/juju/fslock, github.com/juju/fslock]"), "github.com/juju/fslock is denied twice"},
		{"a denied import that is no path", rep("deny: [github.com/juju/fslock]", "deny: ['not a path!']"), `denied import "not a path!"`},
		{"a forbidden call that is no pattern and package", rep("      forbid:\n        - {pattern: '^yaml\\.Unmarshal$', pkg: gopkg.in/yaml.v3}\n", "      forbid: [oops]\n"), "a forbidden call is not a pattern and a package"},
		{"a planned pattern that does not compile", rep("      deny: [github.com/juju/fslock]\n      linter: depguard\n", "      deny: [github.com/juju/fslock]\n      linter: depguard\n      planned: ['^(x']\n    open: [proof]\n"), "planned pattern"},
		{"an item that is no one-line string", rep("      - a strict YAML reader\n", "      - ' '\n"), "by_hand holds an item that is not a one-line string"},
		{"a tool key that is no key", rep("  - key: govulncheck\n", "  - key: GoVuln\n"), `tool "GoVuln" is not lower-case words joined by dashes`},
		{"a tools_yaml that is neither pointer nor a file and key", rep("tools_yaml: \"rulebook/lang/go/tools.yaml, key govulncheck\"", "tools_yaml: somewhere"), `tools_yaml "somewhere" is pointer, or a tools.yaml file and key`},
		{"a tool version that is no release", rep("    version: v1.9.0\n", "    version: latest\n"), `tool govulncheck: version "latest" is a release (v1.2.3)`},
		{"a tool source that is not https", rep("source: https://github.com/golang/vuln", "source: http://example.com/x"), "tool govulncheck: source"},
		{"a tool source of ours", rep("source: https://github.com/golang/vuln", "source: https://github.com/birdtable/x"), "tool govulncheck: source https://github.com/birdtable/x is a repository of the owner"},
		{"a dropped name that is empty", rep("{name: osv-scanner,", "{name: ' ',"), "a dropped name is empty"},
		{"a dropped name that has a tool row", rep("{name: osv-scanner,", "{name: govulncheck,"), "govulncheck is dropped and has a tool row"},
		{"an unexpanded placeholder in a package", rep("package: anywhere a program reads YAML", "package: $p"), `package holds an unexpanded placeholder: "$p"`},
		{"an unexpanded placeholder in a replaced module", rep("{name: gopkg.in/yaml.v3, why: archived upstream}", "{name: $n, why: $w}"), `unexpanded placeholder: "$n"`},
		{"a chosen pin with no release date", rep("      released: \"2026-01-01\"\n", ""), "a chosen row's pinned release v3.0.6 carries the day it was published in released"},
	}
	ran := 0
	for _, tc := range cases {
		ran++
		if tc.text == library {
			t.Errorf("%s: the plant changed nothing", tc.name)
			continue
		}
		_, err := register(t, tc.text)
		if !errors.Is(err, layers.ErrRegister) || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: ParseRegister = %v, want ErrRegister saying %q", tc.name, err, tc.says)
		}
	}
	if ran != len(cases) || ran < 30 {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
	pattern := rep("      forbid:\n", "      forbid:\n        - {pattern: '^a\\.B$', pkg: x}\n")
	if _, err := register(t, pattern); err != nil {
		t.Errorf("a pattern that ends in $ was taken for a placeholder: %v", err)
	}
}
