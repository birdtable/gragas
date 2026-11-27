package trust

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
	"github.com/birdtable/go-files/trust/trusttest"
)

// The Bind plants of host/trust. Each needs a disk mounted
// nosuid, which only a user namespace that keeps its capabilities gives
// without root (trusttest.Bind), so each runs where Bind can mount one;
// elsewhere Bind skips it, saying why.

// TestANameOnANosuidDiskIsTrustedOnlyWithItsMountNamed: a folder on a disk
// mounted nosuid is refused by the walk and by Held as the disk's, with
// MountFix, and passes both once Mounts names the folder the disk is mounted
// on; the walk answers the path it walked. In a user namespace root's / is the
// overflow account's, which the walk and Held of a test binary take
// (Who.Overflow), so the pass is the mount's. Planted: Mounts not handed to
// go-files by the walk or by Held (the second half refused); go-files's entry
// refusal of a folder bound over a root-made one, as gragas's own MountsHeld
// refused one this account's bind made (the second half refused).
//
//nolint:paralleltest // serial: two Bind plants in one process, and a host may hold only one root-made entry (docs/checks.md asks for /mnt or /srv), on which the second would skip
func TestANameOnANosuidDiskIsTrustedOnlyWithItsMountNamed(t *testing.T) {
	entry, dir := trusttest.Bind(t, trusttest.Nosuid)
	git := Subject{Name: "git", File: "the settings file", Last: Folder}
	_, err := Walk(git, dir, Who{})
	if u, ok := errors.AsType[*Unsafe](err); !ok || u.Fix != MountFix || !u.Unjudged {
		t.Errorf("a walk to %s on a nosuid disk, no mounts named = %#v; want the disk's refusal, with MountFix", dir, err)
	}
	if got, err := Walk(git, dir, Who{Mounts: []string{entry}}); err != nil || got != dir {
		t.Errorf("the same walk with %s named = %q, %v; want %s passed", entry, got, err, dir)
	}
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := Held(f, Folder, nil); !errors.Is(err, gotrust.ErrMount) || errors.Is(err, gotrust.ErrMountEntry) {
		t.Errorf("Held of %s on a nosuid disk, no mounts named = %v; want ErrMount, the disk's", dir, err)
	}
	if err := Held(f, Folder, []string{entry}); err != nil {
		t.Errorf("Held of %s with %s named = %v; want it held", dir, entry, err)
	}
}

// TestASettingsFileOnANosuidDiskIsRefusedNamingTheVariable: a settings file on
// a disk mounted nosuid, read with no mounts, is refused as the disk's, in
// go-files's words, with readMountFix, which names GRAGAS_MOUNTS, and why the
// mounts key in that very file cannot let it be read, and never chown; once the
// mounts name the folder the disk is mounted on, it is read whole. In the user
// namespace root's / is the overflow account's, which ReadOwned in a test
// binary takes (Who.Overflow, go-files v0.9.0), so the read is the mount's.
// Planted: ErrMount answered as another account's file (the chown fix), or with
// the disk's fix that names the mounts key alone; the mounts not handed to
// go-files's ReadOwned (the second half refused).
//
//nolint:paralleltest // serial: two Bind plants in one process, and a host may hold only one root-made entry (docs/checks.md asks for /mnt or /srv), on which the second would skip
func TestASettingsFileOnANosuidDiskIsRefusedNamingTheVariable(t *testing.T) {
	entry, dir := trusttest.Bind(t, trusttest.Nosuid)
	file := filepath.Join(dir, "settings.yaml")
	want := "version: 1\n"
	if err := os.WriteFile(file, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadOwned(file, nil)
	o, ok := errors.AsType[*Owned](err)
	if !ok || o.Fix != readMountFix || !errors.Is(err, gotrust.ErrMount) || errors.Is(err, gotrust.ErrMountEntry) {
		t.Fatalf("a settings file on a nosuid disk, no mounts named = %#v; want the disk's refusal, with readMountFix", err)
	}
	if !strings.Contains(o.Fix, MountsVar) || strings.Contains(o.Fix, "chown") {
		t.Errorf("the refusal's fix %q; want it to name %s, with no chown", o.Fix, MountsVar)
	}
	if got, err := ReadOwned(file, []string{entry}); err != nil || string(got) != want {
		t.Errorf("the same settings file with %s named = %q, %v; want %q read", entry, got, err, want)
	}
}
