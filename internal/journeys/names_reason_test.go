package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
)

// TestValidateOnABadNamesLineEndsNotReadyAndItsJSONHoldsNeitherThePatternNorThePath:
// validate --json with a names list whose second line does not compile exits
// 4, and its ending says the line's number and nothing else of the list:
// neither the pattern regexp's error quotes nor the list's path is on stdout
// or stderr. The control is the list with the bad line taken out,
// which ends done.
func TestValidateOnABadNamesLineEndsNotReadyAndItsJSONHoldsNeitherThePatternNorThePath(t *testing.T) {
	t.Parallel()
	dir, env := bindRepo(t), ruleEnv(t)
	servedBy(t, dir, env)
	list := filepath.Join(filestest.Private(t), "zebra-secret-list.txt")
	env["GRAGAS_NAMES_FILE"] = list
	plant := func(body string) {
		t.Helper()
		if err := os.WriteFile(list, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plant("\\bacme\\b\n")
	if r := run(t, dir, env, "validate", "--json"); r.code != 0 {
		t.Fatalf("the control: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	plant("\\bacme\\b\n(ZEBRAMARKER\n")
	r := run(t, dir, env, "validate", "--json")
	if r.code != 4 {
		t.Fatalf("exit %d, want 4\n%s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "line 2 of the names file is not a pattern that compiles") {
		t.Errorf("stdout does not say the line:\n%s", r.stdout)
	}
	for _, secret := range []string{"ZEBRAMARKER", "zebra-secret", list, filepath.Dir(list), "regexp"} {
		if strings.Contains(r.stdout+r.stderr, secret) {
			t.Errorf("the run says %q:\nstdout:\n%s\nstderr:\n%s", secret, r.stdout, r.stderr)
		}
	}
}

// TestValidateOnANamesFilePathThatIsNotCleanHoldsNoPartOfIt: validate --json
// with a GRAGAS_NAMES_FILE that is relative, or absolute and climbing, is
// refused before anything is read, its ending names the variable and says why
// from the closed set, and no part of the path is on stdout or stderr: the
// path can hold a login that is on the list. The made-up folder is
// zebra-secret. The control is the absolute, clean path, which ends done.
func TestValidateOnANamesFilePathThatIsNotCleanHoldsNoPartOfIt(t *testing.T) {
	t.Parallel()
	dir, env := bindRepo(t), ruleEnv(t)
	servedBy(t, dir, env)
	private := filepath.Join(filestest.Private(t), "zebra-secret")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(private, "list.txt")
	if err := os.WriteFile(list, []byte("\\bacme\\b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["GRAGAS_NAMES_FILE"] = list
	if r := run(t, dir, env, "validate", "--json"); r.code != 0 {
		t.Fatalf("the control: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	for name, v := range map[string]string{
		"a relative path":    filepath.Join("zebra-secret", "list.txt"),
		"a path that climbs": private + "/../zebra-secret/list.txt",
	} {
		env["GRAGAS_NAMES_FILE"] = v
		r := run(t, dir, env, "validate", "--json")
		said := r.stdout + r.stderr
		if r.code == 0 || !strings.Contains(said, "GRAGAS_NAMES_FILE must be") {
			t.Errorf("%s: exit %d, want a refusal naming GRAGAS_NAMES_FILE\nstdout:\n%s\nstderr:\n%s", name, r.code, r.stdout, r.stderr)
		}
		for _, secret := range []string{"zebra-secret", "list.txt", v} {
			if strings.Contains(said, secret) {
				t.Errorf("%s: the run says %q:\nstdout:\n%s\nstderr:\n%s", name, secret, r.stdout, r.stderr)
			}
		}
	}
}
