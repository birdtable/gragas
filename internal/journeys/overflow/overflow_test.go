package overflow

import (
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/appdirs"
	gotrust "github.com/birdtable/go-files/trust"
	testenv "github.com/birdtable/go-testkit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The built gragas never takes the overflow account for root. In a test
// binary on a system with user namespaces the trust adapter asks go-files'
// walk with Who.Overflow, so a test can walk a / that shows as the overflow
// account's; the program is no test binary, so it asks
// without, and a / of the overflow account's is refused as any other
// account's is.

// TestTheProgramRefusesARootTheOverflowAccountOwns: in a user namespace that
// maps this account and not root, / shows as the overflow account's. This
// package's TestMain runs the tests in go-testkit's contain, which is that
// namespace. The built gragas, run there with git set, is refused at /, and
// says the overflow account owns it, by the uid go-files' own walk of /
// names in its refusal (Unsafe.Observed): the test binary's path is never the
// program's (point 2). Whether the run is in
// such a namespace is go-files' to judge, as it judges Who.Overflow, never a
// read of /proc here. Where go-files refuses Who.Overflow, as
// where root is mapped, it is red rather than a pass that proved nothing: a
// host that refuses contain its namespace fails here, and is never skipped. A
// system go-files takes no Who.Overflow on has no namespace to put the
// program in, and the test says so and ends. Risk: a program that trusts the
// overflow account, which in a namespace any account can make is anyone.
func TestTheProgramRefusesARootTheOverflowAccountOwns(t *testing.T) {
	t.Parallel()
	_, err := gotrust.Path("/", gotrust.Folder, gotrust.Who{Overflow: true})
	switch {
	case errors.Is(err, errors.ErrUnsupported):
		t.Logf("this system has no user namespace go-files takes Who.Overflow in, so no overflow root to refuse: %v", err)
		return
	case err != nil:
		t.Fatalf("this needs a user namespace that maps this account and not root, which this package's TestMain asks contain for; go-files refuses Who.Overflow here: %v", err)
	}
	_, err = gotrust.Path("/", gotrust.Folder, gotrust.Who{})
	u, ok := errors.AsType[*gotrust.Unsafe](err)
	if !ok || !errors.Is(err, gotrust.ErrNotOwners) || u.Path != "/" {
		t.Fatalf("/ walked without Who.Overflow = %v; want it refused, owned by the overflow account", err)
	}
	owner, _, _ := strings.Cut(u.Observed, ",")
	said, code := runInit(t)
	if code == 0 {
		t.Fatalf("the program walked a / the overflow account owns and ran\n%s", said)
	}
	if !strings.HasPrefix(owner, "/ is owned by uid:") || !strings.Contains(said, owner) {
		t.Fatalf("exit %d, and the refusal does not say %q, what go-files saw of /\n%s", code, owner, said)
	}
}

// runInit builds gragas and runs its init in a folder of the test's own, with
// the real git set as the program's git, through go-testkit's Binary, in an
// environment of the test's own: its home, which appdirs places every folder of
// the program under, and a settings file naming git. It answers what the run
// printed and its exit code.
func runInit(t *testing.T) (said string, code int) {
	t.Helper()
	home := filestest.Private(t)
	settings := filepath.Join(home, "settings.yaml")
	if err := os.WriteFile(settings, []byte("version: 1\nprograms:\n  git: "+testenv.GitPath(t)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	kit := testenv.Build("cmd/gragas").Trimpath()
	if kit.Path() == "" {
		t.Fatalf("could not build gragas:\n%s", configtest.GoSaid(kit.Run().Stderr))
	}
	r := kit.Dir(home).Env(appdirs.HomeVar+"="+home, "GRAGAS_CONFIG="+settings, "GRAGAS_NAMES_FILE=", "COLUMNS=80", "GRAGAS_SHELL=posix").
		Run("init", "--kind", "rulebook", "--name", "demo", "--owner", "team-a")
	return r.Out(), r.Code
}
