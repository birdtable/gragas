// Package version is the version command: which gragas this is, said as lines
// or as one JSON document. It is the first command to live in a package of its
// own below internal/cli/cmd; the root joins it from the generated join_gen.go.
package version

import (
	"cmp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 520

// Row is the one declaration of version (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "version", Order: Order, Spec: true, Context: kit.Anywhere, Ends: kit.EndsListing, State: kit.Built,
		Summary: "Say which build this is",
		Examples: [2]kit.Example{
			{Line: "gragas version", Does: "say which build this is"},
			{Line: "gragas version --json", Does: "as one JSON document"},
		},
		Exits:    []model.Word{model.Done},
		JSONKind: "version",
		Menu:     kit.Menu{Offered: true},
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print this build",
		Long: `Which gragas this is: its release tag, the commit it was built
from, the Go release that built it, the target, and the
format and contract versions it reads. --json gives the same
as one JSON document.

  gragas version
  gragas --version
  gragas version --json`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return s.Exit(run(s)) },
	}
}

func run(s kit.Session) int {
	v := doc(s.Build())
	if s.JSON() {
		s.Document(v)
		return exit.Done
	}
	tag, commit := "none, no release tag reaches this build", "unknown, the build recorded none"
	if v.Tag != nil {
		tag = *v.Tag
	}
	if v.Commit != nil {
		commit = *v.Commit
	}
	s.Say("gragas " + s.Build().Line)
	for _, r := range [][2]string{{"tag", tag}, {"commit", commit}, {"go", v.Go}, {"target", v.Target},
		{"format", strconv.Itoa(v.Format)}, {"contract", v.Contract}} {
		s.Say("  " + kit.Column(r[0], 10) + r[1])
	}
	return s.End(kit.Ending{Word: model.Done, What: "this is the build above"})
}

// untaggedPrefix starts the version of a build no release tag reaches.
const untaggedPrefix = "v0.0.0-"

// doc is the facts of the build: a tag or a commit it cannot establish is
// null, with the reason under Notes (cli.output.9).
func doc(b kit.Build) coreapi.Version {
	v := coreapi.Version{Contract: model.ContractVersion, Version: b.Version, Go: cmp.Or(b.Go, "unknown"),
		Target: target(), Format: model.FormatVersion}
	if !strings.HasPrefix(b.Version, untaggedPrefix) {
		tag := b.Version
		v.Tag = &tag
	}
	if b.Commit != "" {
		c := b.Commit
		v.Commit = &c
	}
	if v.Tag == nil {
		v.Notes = map[string]string{"tag": "no release tag reaches this build"}
	}
	if v.Commit == nil {
		if v.Notes == nil {
			v.Notes = map[string]string{}
		}
		v.Notes["commit"] = "the build recorded none"
	}
	return v
}

// target is the system the binary was built for, as the build recorded it:
// os/arch. The build's own record is read, never the running system
// (arch.ports.1).
func target() string {
	goos, goarch := "", ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "GOOS":
				goos = s.Value
			case "GOARCH":
				goarch = s.Value
			}
		}
	}
	if goos == "" || goarch == "" {
		return "unknown"
	}
	return goos + "/" + goarch
}
