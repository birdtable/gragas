package internal

import (
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// splitGit is what is wrong with the adapters of History and Remote, "" when
// one package fills both: two subjects, two ports, and one external program
// serves both, so one adapter package does (arch.ports.6, architecture 4).
func splitGit(tb *layers.Table) string {
	var history, remote string
	for _, p := range tb.Ports.List {
		switch p.Name {
		case "History":
			history = p.Adapter
		case "Remote":
			remote = p.Adapter
		}
	}
	if history == "" || history != remote {
		return "History is served by " + history + " and Remote by " + remote + ", want one package"
	}
	return ""
}

// TestHistoryAndRemoteAreOneAdapterPackage: the table has one adapter for
// both. Planted: History's adapter moved to a package of its own.
func TestHistoryAndRemoteAreOneAdapterPackage(t *testing.T) {
	t.Parallel()
	tb := table(t)
	if got := splitGit(tb); got != "" {
		t.Error(got)
	}
	for i, p := range tb.Ports.List {
		if p.Name == "History" {
			tb.Ports.List[i].Adapter = "internal/host/gitlog"
		}
	}
	if splitGit(tb) == "" {
		t.Error("a History split from Remote was not seen")
	}
}
