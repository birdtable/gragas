package coreapi_test

import (
	"testing"
	"testing/fstest"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// exemptRows is an exemptions file with a row of arch.ports.24 for a file, one
// for a folder, an expired one, one with no path, one with a ./ spelling, and a
// "." row of another rule.
const exemptRows = `exemptions:
  - {id: a, rule: arch.ports.24, path: internal/journeys/pty_test.go, expires: "2027-03-31"}
  - {id: b, rule: arch.ports.24, path: internal/tools/, expires: "2027-03-31"}
  - {id: c, rule: arch.ports.24, path: internal/old_linux.go, expires: "2026-01-01"}
  - {id: d, rule: arch.ports.24, expires: "2027-03-31"}
  - {id: e, rule: arch.ports.24, path: ./internal/dot_linux.go, expires: "2027-03-31"}
  - {id: f, rule: other.rule.1, path: ., expires: "2027-03-31"}
`

// TestALiveRowWaivesItsFileOrFolderAndNoOther holds liveRowsAt, the lookup the
// os-files reader waives by: a live row of the rule waives the file it names
// and the files of the folder it names; an expired row, a row with no path, a
// ./ spelling and a row of another rule waive nothing; with no today, nothing
// is live.
// Planted failure: expiry is not read, a row with no path or of another rule
// waives every file, or a folder row waives a file beside the folder.
func TestALiveRowWaivesItsFileOrFolderAndNoOther(t *testing.T) {
	t.Parallel()
	src := fstest.MapFS{"gate/exemptions.yaml": {Data: []byte(exemptRows)}}
	at := coreapi.LiveRowsAt(src, "2026-11-17")
	for file, want := range map[string]bool{
		"internal/journeys/pty_test.go":   true,
		"internal/tools/x_linux.go":       true,
		"internal/tools/deep/x_darwin.go": true,
		"internal/toolsx/x_linux.go":      false,
		"internal/old_linux.go":           false,
		"internal/dot_linux.go":           false,
		"internal/other_windows.go":       false,
	} {
		if _, got := at("arch.ports.24", file); got != want {
			t.Errorf("arch.ports.24 at %s: waived %v, want %v", file, got, want)
		}
	}
	if until, ok := at("other.rule.1", "any/file.go"); !ok || until != "2027-03-31" {
		t.Errorf(`a "." row waives every file of its rule, got %q %v`, until, ok)
	}
	if _, ok := coreapi.LiveRowsAt(src, "")("arch.ports.24", "internal/journeys/pty_test.go"); ok {
		t.Error("with no today a row is live")
	}
	if _, ok := coreapi.LiveRowsAt(fstest.MapFS{}, "2026-11-17")("arch.ports.24", "internal/journeys/pty_test.go"); ok {
		t.Error("with no exemptions file a row is live")
	}
}
