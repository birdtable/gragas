package journeys

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// goVersion is the Go release in the version output, which moves with the
// toolchain and is put back as a word in the goldens.
var goVersion = regexp.MustCompile(`go1\.[0-9]+(\.[0-9]+)?`)

// TestTheGoldensHoldWhatAPersonReads: help, version and check, as a pipe at
// 80, 60 and 40 columns (cli.look.8), each compared with its golden and never
// rewritten by the gate (cli.output.3, tests.generated.5). A golden is
// written only by go test -run TestTheGoldens -update, and a person reads
// the diff. Risk: a help or a report that changes shape unseen.
func TestTheGoldensHoldWhatAPersonReads(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{"help", []string{"--help"}},
			{"gen-help", []string{"gen", "--help"}},
			{"version", []string{"version"}},
			{"check", []string{"check"}},
			{"gen-dry-run", []string{"gen"}},
			{"init", []string{"init"}},
			{"mistyped", []string{"chek"}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				r := run(t, repo, map[string]string{"COLUMNS": cols}, tc.args...)
				got := "$ gragas " + strings.Join(tc.args, " ") + "\n--- stdout\n" + r.stdout + "--- stderr\n" + r.stderr
				uitest.Golden(t, tc.name+"-"+cols, scrub(got, ""))
			})
		}
	}
}

// TestJSONIsOneDocumentWithASchemaNumber: --json prints one JSON document
// on stdout and the rest on stderr, for a pass, a failure and a usage error
// alike, its fields held by a golden (cli.output.1, cli.errors.1). A result
// carries schema 2 since the rulebook moved to schema v2; version keeps its
// own number, contract. Risk: a
// script that parses stdout and finds two documents, or a banner.
func TestJSONIsOneDocumentWithASchemaNumber(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	for _, tc := range []struct {
		name     string
		args     []string
		code     int
		schema   float64
		contract string
	}{
		{"check", []string{"check", "--json"}, 0, 2, ""},
		{"version", []string{"version", "--json"}, 0, 0, "1.3"},
		{"gen-dry-run", []string{"gen", "--json"}, 0, 2, ""},
		{"init", []string{"init", "--json"}, 4, 2, ""},
		{"mistyped", []string{"chek", "--json"}, 2, 2, ""},
		{"not-a-repository", []string{"check", "--json", "-C", "rulebook"}, 4, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, repo, nil, tc.args...)
			if r.code != tc.code {
				t.Fatalf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
			}
			dec := json.NewDecoder(strings.NewReader(r.stdout))
			var doc map[string]any
			if err := dec.Decode(&doc); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, r.stdout)
			}
			if dec.More() {
				t.Fatalf("stdout holds more than one document:\n%s", r.stdout)
			}
			if tc.contract != "" {
				if doc["contract"] != tc.contract || doc["schema"] != nil {
					t.Errorf("contract = %v and schema = %v, want contract %q and no schema", doc["contract"], doc["schema"], tc.contract)
				}
			} else if n, ok := doc["schema"].(float64); !ok || n != tc.schema {
				t.Errorf("schema = %v, want the integer %v", doc["schema"], tc.schema)
			}
			uitest.Golden(t, "json-"+tc.name, scrub(r.stdout, ""))
		})
	}
}

// escape is an escape byte, which plain output never holds.
const escape = "\x1b"

// TestTheTerminalRows: what forces colour and what forbids it, against the
// real binary (cli.environment.1, .4, .6, .11, .13). CLICOLOR_FORCE paints a
// pipe; NO_COLOR, TERM=dumb, CI and --no-color each beat it, and plain output
// carries the same words. Risk: a pipe or a CI log full of escape codes.
func TestTheTerminalRows(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	forced := run(t, repo, map[string]string{"CLICOLOR_FORCE": "1"}, "check")
	if !strings.Contains(forced.stdout, escape) {
		t.Fatalf("CLICOLOR_FORCE drew no colour, so the rows below prove nothing:\n%q", forced.stdout)
	}
	plain := run(t, repo, nil, "check")
	if strings.Contains(plain.stdout+plain.stderr, escape) {
		t.Fatalf("a pipe got escape codes:\n%q", plain.stdout)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
	}{
		{"NO_COLOR beats CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1"}, nil},
		{"TERM=dumb beats CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1", "TERM": "dumb"}, nil},
		{"CI beats CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1", "CI": "true"}, nil},
		{"--no-color beats CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1"}, []string{"--no-color"}},
		{"help under NO_COLOR", map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1"}, []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"check"}, tc.args...)
			if len(tc.args) > 0 && tc.args[0] == "--help" {
				args = []string{"--help"}
			}
			r := run(t, repo, tc.env, args...)
			if strings.Contains(r.stdout+r.stderr, escape) {
				t.Errorf("escape codes in plain output:\n%q", r.stdout)
			}
			if args[0] == "check" && r.stdout != plain.stdout {
				t.Errorf("plain output differs from a pipe's:\n got %q\nwant %q", r.stdout, plain.stdout)
			}
		})
	}
}

// TestPlainAndAccessibleSayTheSame: --plain and --accessible change how a
// thing is asked or drawn, never what is said (cli.environment.7, .11;
// cli.config.6's test of each switch at its non-default).
func TestPlainAndAccessibleSayTheSame(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	want := run(t, repo, nil, "check").stdout
	symbol := regexp.MustCompile(`(?m)^  [*!#/&%<>] `)
	for _, flag := range []string{"--plain", "--accessible"} {
		r := run(t, repo, nil, "check", flag)
		got := r.stdout
		if flag == "--accessible" {
			got = symbol.ReplaceAllString(got, "  ") // the symbol is the decoration the accessible form adds
		}
		if r.code != 0 || got != want {
			t.Errorf("check %s: exit %d, said\n%s\nwant\n%s", flag, r.code, r.stdout, want)
		}
	}
}
