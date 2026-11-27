package journeys

import "path/filepath"

// oldBook is the format 2 rulebook the old check's journeys read: gragas's
// own rulebook/ as it stood at 5b414e0f1, before the switch that
// converted the live one to format 3, which the old check does not read. It
// is frozen, never regenerated, and goes with the old check.
func oldBook() string {
	return filepath.Join(moduleRoot(), "internal", "journeys", "testdata", "rulebook-v2")
}
