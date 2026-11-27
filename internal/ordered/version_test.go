package ordered_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// version is the key version: parsed from doc, as each versioned file's reader
// parses it.
func version(t *testing.T, doc string) any {
	t.Helper()
	v, err := ordered.ParseStrict([]byte(doc))
	if err != nil {
		t.Fatalf("parse %q: %v", doc, err)
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		t.Fatalf("parse %q: %T, want a mapping", doc, v)
	}
	return m.Val("version")
}

func TestVersionRefusesZeroANegativeAndAWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		doc   string
		wantN int64
		want  ordered.VersionIs
	}{
		{"version: 0", 0, ordered.VersionBelowOne},
		{"version: -1", -1, ordered.VersionBelowOne},
		{"version: one", 0, ordered.VersionNotWhole},
		{"version: \"1\"", 0, ordered.VersionNotWhole},
		{"version: 1.5", 0, ordered.VersionNotWhole},
		{"version: [1]", 0, ordered.VersionNotWhole},
	} {
		n, is := ordered.Version(version(t, tc.doc), 3)
		if n != tc.wantN || is != tc.want {
			t.Errorf("Version of %q = %d, %d; want %d, %d", tc.doc, n, is, tc.wantN, tc.want)
		}
	}
}

func TestVersionSaysNewerNamingBoth(t *testing.T) {
	t.Parallel()
	n, is := ordered.Version(version(t, "version: 4"), 3)
	if n != 4 || is != ordered.VersionNewer {
		t.Errorf("Version of 4 to a reader of 3 = %d, %d; want 4, VersionNewer (%d), so the caller names 4 beside its own 3", n, is, ordered.VersionNewer)
	}
	for _, doc := range []string{"version: 1", "version: 3"} {
		if n, is := ordered.Version(version(t, doc), 3); is != ordered.VersionRead || n < 1 || n > 3 {
			t.Errorf("Version of %q to a reader of 3 = %d, %d; want it read", doc, n, is)
		}
	}
}

func TestVersionRefusesNoValue(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{"version:", "other: 1"} {
		if n, is := ordered.Version(version(t, doc), 3); n != 0 || is != ordered.VersionNotWhole {
			t.Errorf("Version of %q = %d, %d; want 0, VersionNotWhole: no value is not a version, and never the reader's own", doc, n, is)
		}
	}
}
