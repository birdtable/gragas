package proof

import "github.com/birdtable/gragas/internal/model"

// judge is the verdict on one binding from what the tool found over its good
// example and over its bad one. It passes only when the bad example draws a
// finding of the binding's key at the placed path and the good one draws none.
// Only a finding's key and path decide: its message, URL and column are never
// read, so a tool's text cannot say pass. A finding at another path is another
// file's and counts for nothing. A side that hit a limit is not run, limit hit.
func judge(b model.Binding, good, bad []found, limit model.Reason) verdict {
	if limit != "" {
		return verdict{truth: model.TruthNotRun, reason: limit}
	}
	if flags(bad, b) && !flags(good, b) {
		return verdict{truth: model.TruthPass}
	}
	return verdict{truth: model.TruthFail}
}

// flags is whether any finding is of the binding's key at its path.
func flags(fs []found, b model.Binding) bool {
	for _, f := range fs {
		if f.key == b.Key && f.path == b.PairPath {
			return true
		}
	}
	return false
}
