package pluginbundle_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/pluginbundle"
)

// lockWith is a lock of one entry, lint-a at version.
func lockWith(version string) []byte {
	return []byte("version: 1\nplugins:\n  - name: lint-a\n    version: " + version +
		"\n    source: file:lint-a\n    digest: sha256:" + strings.Repeat("a", 64) + "\n")
}

// TestALockReadsAVersionWithNoLeadingZero: each part 0 or a whole number
// that does not start with 0 is read.
func TestALockReadsAVersionWithNoLeadingZero(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"0.0.0", "0.10.0", "10.20.300", "1.0.9"} {
		got, err := pluginbundle.DecodePluginLock(lockWith(v))
		if err != nil || len(got) != 1 || got[0].Ref.Version != v {
			t.Errorf("DecodePluginLock of %s = %+v, %v, want the one entry", v, got, err)
		}
	}
}

// TestALockEntryWithALeadingZeroIsRefusedByNameWithItsFix: a lock written in
// the form before this one, a part with a leading zero, is refused whole,
// naming the plugin, its version and its folder, and saying how to put it
// right; semver could not order it.
func TestALockEntryWithALeadingZeroIsRefusedByNameWithItsFix(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"02.0.0", "2.00.0", "2.0.01", "00.0.0"} {
		got, err := pluginbundle.DecodePluginLock(lockWith(v))
		if !errors.Is(err, fs.ErrInvalid) || got != nil {
			t.Errorf("DecodePluginLock of %s = %+v, %v, want refused as invalid", v, got, err)
			continue
		}
		for _, want := range []string{
			`plugin lint-a's version "` + v + `" has a leading zero`,
			"plugins/lint-a/" + v,
			"gragas plugin add <folder>",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("DecodePluginLock of %s = %v, want it to say %q", v, err, want)
			}
		}
	}
}

// TestALockEntryThatIsNoVersionIsRefusedAsBefore: a version that is not three
// whole numbers keeps its own refusal; the leading-zero fix is not offered.
func TestALockEntryThatIsNoVersionIsRefusedAsBefore(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"2.0", "v2.0.0", "2.0.0-rc.1", "-1.0.0"} {
		_, err := pluginbundle.DecodePluginLock(lockWith(v))
		if !errors.Is(err, fs.ErrInvalid) || err == nil || !strings.Contains(err.Error(), "is not a version of three whole numbers") || strings.Contains(err.Error(), "leading zero") {
			t.Errorf("DecodePluginLock of %s = %v, want it refused as no version", v, err)
		}
	}
}
