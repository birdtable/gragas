package internal

// The canary of the secret scan (gragas#1314, from #1241's point 2). The
// plants of secrets_test.go are private keys, and the second pass,
// .gitleaks-key.toml, finds a private key alone: a first pass that
// found nothing would leave every one of them red. The canary is a secret only
// the first pass's rules find, gitleaks' default github-pat rule, so the
// members are red on it only while .gitleaks.toml still reaches it.
//
// TestTheSecretScanFindsTheCanaryInEveryMemberAndPass runs the members over a
// clone that holds it, through the repository's own Taskfile and configs: a
// change to .gitleaks.toml that puts the canary out of reach turns it red.
// TestASilencedSecretScanFailsTheCanary proves the check on a copy of the
// Taskfile beside a planted .gitleaks.toml, one plant per way of silencing
// the scan, each of which leaves the members green: an allow-list path, a
// stopword, an empty rule set, and the default rules dropped.
//
// The canary is made up, built here from parts and random letters, and lives
// only in the test's temp folder and its memory, so the repository's own scan
// never finds one; no report may hold it.

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canaryRule is the rule of gitleaks' default config the canary is in the
// shape of, and canaryPath the file of the clone that holds it.
const (
	canaryRule = "github-pat"
	canaryPath = "fixtures/canary.txt"
)

// canaryToken is a made-up token in the github-pat shape: its prefix and 36
// random letters and digits, new for each call.
func canaryToken() string {
	const alnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 36)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24)
	for i := range b {
		b[i] = alnum[int(b[i])%len(alnum)]
	}
	return "gh" + "p_" + string(b)
}

// canaryClone is a clone of a fresh base with the canary committed past its
// origin/main, and the token it holds.
func canaryClone(t *testing.T) (dir, token string) {
	t.Helper()
	base := secretsBase(t)
	dir = filepath.Join(t.TempDir(), "clone")
	wideGit(t, filepath.Dir(dir), "clone", "-q", base, dir)
	token = canaryToken()
	put(t, dir, canaryPath, []byte("a made-up value: "+token+"\n"))
	wideGit(t, dir, "add", "-A")
	wideGit(t, dir, "commit", "-q", "-m", "canary")
	return dir, token
}

// missedCanary is what is wrong with a member's run over a clone that holds
// the canary, or "": it must be red, name the rule and the path, and hold no
// part of the token past its prefix.
func missedCanary(out string, ok bool, token string) string {
	switch {
	case strings.Contains(out, token[4:]):
		return "the report holds the canary's value"
	case ok:
		return "green on the canary: the scan is silenced"
	case !strings.Contains(out, canaryRule) || !strings.Contains(out, canaryPath):
		return "red without naming " + canaryRule + " in " + canaryPath
	}
	return ""
}

// canaryMember is a member of the Taskfile, and the SCAN it is given, if any.
type canaryMember struct{ name, scan string }

// canaryMembers are the runs that must each find the canary: secrets and
// secrets-full as the Taskfile runs them, and the tree pass alone, since
// secrets-full stops at its first red pass, the history.
func canaryMembers() []canaryMember {
	return []canaryMember{
		{name: "secrets"},
		{name: "secrets-full"},
		{name: "secrets-scan", scan: "tree"},
	}
}

func runCanary(t *testing.T, taskfile, dir, name, scan string) (string, bool) {
	t.Helper()
	if scan != "" {
		return runSecretsFrom(t, taskfile, dir, name, "SCAN="+scan)
	}
	return runSecretsFrom(t, taskfile, dir, name)
}

func TestTheSecretScanFindsTheCanaryInEveryMemberAndPass(t *testing.T) {
	t.Parallel()
	dir, token := canaryClone(t)
	for _, m := range canaryMembers() {
		out, ok := runCanary(t, "", dir, m.name, m.scan)
		if why := missedCanary(out, ok, token); why != "" {
			t.Errorf("%s %s: %s:\n%s", m.name, m.scan, why, strings.ReplaceAll(out, token[4:], "<the canary>"))
		}
	}
}

// silencedRoot is a folder that holds a copy of the repository's Taskfile and
// private-key config, and gitleaks as its .gitleaks.toml: the root a planted
// first pass runs from. It answers the copy's Taskfile.
func silencedRoot(t *testing.T, gitleaks string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"Taskfile.yml", ".gitleaks-key.toml", "internal/journeys/goldens.txt"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		put(t, dir, name, data)
	}
	put(t, dir, ".gitleaks.toml", []byte(gitleaks))
	return filepath.Join(dir, "Taskfile.yml")
}

func TestASilencedSecretScanFailsTheCanary(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(root, ".gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	own := string(data)
	const extend = "[extend]\nuseDefault = true\n"
	if !strings.Contains(own, extend) {
		t.Fatalf(".gitleaks.toml no longer holds %q, which the plant that drops the default rules removes", extend)
	}
	// With the default rules out, the allow-list of a default rule goes too
	// (gragas#2340): gitleaks refuses a config whose allow-list names a rule it
	// does not hold, and that red is not the silence the plant is.
	noDefaults := strings.Replace(own, extend, "", 1)
	if i := strings.Index(noDefaults, "\n[[allowlists]]"); i >= 0 {
		noDefaults = noDefaults[:i+1]
	}
	dir, token := canaryClone(t)
	// The control: the copy beside the repository's own config finds the
	// canary, so the copy is not what turns the plants green.
	t.Run("control", func(t *testing.T) {
		t.Parallel()
		taskfile := silencedRoot(t, own)
		for _, m := range canaryMembers() {
			out, ok := runCanary(t, taskfile, dir, m.name, m.scan)
			if why := missedCanary(out, ok, token); why != "" {
				t.Errorf("%s %s from the copy: %s:\n%s", m.name, m.scan, why, strings.ReplaceAll(out, token[4:], "<the canary>"))
			}
		}
	})
	for name, gitleaks := range map[string]string{
		"an allow-list path":    own + "\n[[allowlists]]\npaths = ['''^fixtures/''']\n",
		"a stopword":            own + "\n[[allowlists]]\nstopwords = ['''" + "gh" + "p_" + "''']\n",
		"an empty rule set":     "title = \"no rules\"\n",
		"the default rules out": noDefaults,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			taskfile := silencedRoot(t, gitleaks)
			for _, m := range canaryMembers() {
				out, ok := runCanary(t, taskfile, dir, m.name, m.scan)
				switch {
				case !ok:
					t.Errorf("%s %s: red under the plant, so it is not a silenced scan, which is green:\n%s", m.name, m.scan, strings.ReplaceAll(out, token[4:], "<the canary>"))
				case missedCanary(out, ok, token) == "":
					t.Errorf("%s %s: the check finds the canary under a silenced .gitleaks.toml:\n%s", m.name, m.scan, strings.ReplaceAll(out, token[4:], "<the canary>"))
				}
			}
		})
	}
}
