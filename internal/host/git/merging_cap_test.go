package git

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAMergeHeadIsReadToItsCap: a MERGE_HEAD of maxMergeHead bytes is read
// whole, and one byte more is refused, naming the cap.
// boundary: maxMergeHead
// Risk: a MERGE_HEAD past the cap is read whole into memory, or one at it
// is refused.
func TestAMergeHeadIsReadToItsCap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, n := range []int{maxMergeHead, maxMergeHead + 1} {
		p := filepath.Join(dir, "MERGE_HEAD"+strconv.Itoa(n))
		if err := os.WriteFile(p, []byte(strings.Repeat("x", n)), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readMergeHead(p)
		switch {
		case n == maxMergeHead && (err != nil || len(got) != n):
			t.Errorf("a MERGE_HEAD at the cap = %d bytes, %v; want %d bytes", len(got), err, n)
		case n > maxMergeHead && (err == nil || got != nil || !strings.Contains(err.Error(), strconv.Itoa(maxMergeHead))):
			t.Errorf("a MERGE_HEAD one byte past the cap = %d bytes, %v; want refused, naming %d", len(got), err, maxMergeHead)
		}
	}
}
