package internal

// The one allow-list of .gitleaks.toml (gragas#2340): a pinned plugin's
// recorded response.json holds linter names as {"key":"<linter>"}, which
// gitleaks' generic-api-key reads as a key when the name is long. The
// allow-list names the one value, never a shape, and
// holds only where the rule, the file and the value are all so: that name there
// is not found, and a key of another shape in that file, a lowercase hex key
// that a linter's shape would let by, the same name in another file, and
// another rule's secret in that file are each found, named by their rule. The
// file holds that allow-list and no other, as its last block, so a second one,
// or a wider one, is a change this test names.

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// theAllowList is .gitleaks.toml's one allow-list, which ends the file. The
// name is built from parts, so this file holds no match of the rule.
const theAllowList = "[[allowlists]]\n" +
	`description = "A pinned plugin's recorded linter names, read as generic API keys (gragas#2340)"` + "\n" +
	`targetRules = ["generic-api-key"]` + "\n" +
	`condition = "AND"` + "\n" +
	`regexTarget = "match"` + "\n" +
	`paths = ['''^plugins/[a-z0-9]+(?:-[a-z0-9]+)*/[0-9]+\.[0-9]+\.[0-9]+/testdata/cases/[^/]+/response\.json$''']` + "\n" +
	`regexes = ['''^key":"` + "exhaust" + "ruct_v5" + `"$''']` + "\n"

func TestTheGitleaksConfigHoldsItsOneAllowListLast(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(root, ".gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if n := strings.Count(s, "[[allowlists]]"); n != 1 {
		t.Errorf(".gitleaks.toml holds %d allow-lists, want the one of gragas#2340: another is a change a reviewer reads, and this test names it", n)
	}
	if !strings.HasSuffix(s, "\n\n"+theAllowList) {
		t.Errorf(".gitleaks.toml does not end with its one allow-list as written here:\n%s", theAllowList)
	}
}

// recordedResponse is where a pinned plugin's recorded entries list lives.
const recordedResponse = "plugins/golangci-lint/1.0.1/testdata/cases/1-entries/response.json"

// mixedKey is a made-up key of 32 letters and digits that begins with a capital,
// new for each call, so no linter's name is ever its shape.
func mixedKey() string {
	const alnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 31)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24)
	for i := range b {
		b[i] = alnum[int(b[i])%len(alnum)]
	}
	return "Q" + string(b)
}

// allowClone is a clone of a fresh base with path holding text, committed.
func allowClone(t *testing.T, path, text string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	wideGit(t, filepath.Dir(dir), "clone", "-q", secretsBase(t), dir)
	put(t, dir, path, []byte(text))
	wideGit(t, dir, "add", "-A")
	wideGit(t, dir, "commit", "-q", "-m", "plant")
	return dir
}

func TestTheRecordedLinterNamesAreAllowedAndNothingElse(t *testing.T) {
	t.Parallel()
	// built from parts, so this file holds no match of the rule
	const linter = `{"key":"` + "exhaust" + "ruct_v5" + `"}` + "\n"
	t.Run("a linter's name in the recorded response", func(t *testing.T) {
		t.Parallel()
		if out, ok := runSecretsFrom(t, "", allowClone(t, recordedResponse, linter), "secrets-scan", "SCAN=tree"); !ok {
			t.Errorf("a recorded linter's name is found:\n%s", out)
		}
	})
	key := mixedKey()
	// every hex digit twice, from a letter: a linter's shape, and a key's entropy
	hex := strings.Repeat("a3f9c0e1"+"7b5d2846", 2)
	for _, c := range []struct{ name, path, text, rule, secret string }{
		{"a key of another shape in the recorded response", recordedResponse, `{"key":"` + key + `"}` + "\n", "generic-api-key", key},
		{"a lowercase hex key in the recorded response", recordedResponse, `{"key":"` + hex + `"}` + "\n", "generic-api-key", hex},
		{"a linter's name in another file", "fixtures/response.json", linter, "generic-api-key", ""},
		{"another rule's secret in the recorded response", recordedResponse, `{"note":"` + canaryToken() + `"}` + "\n", canaryRule, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, ok := runSecretsFrom(t, "", allowClone(t, c.path, c.text), "secrets-scan", "SCAN=tree")
			shown := out
			if c.secret != "" {
				shown = strings.ReplaceAll(out, c.secret, "<the key>")
			}
			if ok || !strings.Contains(out, c.rule) || !strings.Contains(out, c.path) {
				t.Errorf("%s: ok %v, want red naming %s in %s:\n%s", c.name, ok, c.rule, c.path, shown)
			}
		})
	}
}
