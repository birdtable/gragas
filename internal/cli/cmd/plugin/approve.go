package plugin

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// approveCmd is gragas plugin approve.
func approveCmd(s kit.Session) *cobra.Command {
	var digest string
	c := &cobra.Command{
		Use:   "approve <name>[@<version>]",
		Short: "Show what approving a plugin covers, and record the approval",
		Long: `Shows what an approval of one installed plugin covers, read from its
manifest alone: the name and version, the digest of the bundle as it lies on
disk, the tool and the versions of it served, every grant (its id, where its
program is found, the versions it is held to and how its version is asked),
the preset path, the folder a run links in, and the install words. A grant with
no script hands the plugin the whole command line, and the screen says so.
Every character outside printable ASCII is shown escaped. A bundle that
changed since it was locked, or whose manifest is refused, is never shown for
a yes.

Without --apply it records nothing and ends on the line to type. With --apply
it records the approval only when --digest is the digest shown, the installed
bundle's: there is no default answer, and a digest that is not the bundle's
changes nothing. The approval is of the bytes: a plugin whose module or grants
change by a byte has another digest and is not approved. A name the lock holds
in more than one version takes name@version.

  gragas plugin approve alpha
  gragas plugin approve alpha@1.0.0 --digest sha256:<digest> --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{kit.Writes: "with --apply"},
		RunE: func(c *cobra.Command, args []string) error {
			return s.Exit(runApprove(c, s, args[0], model.Digest(digest)))
		},
	}
	c.Flags().StringVar(&digest, "digest", "", "The digest of the bundle shown, which --apply records")
	c.Flags().BoolVar(s.ApplyFlag(), "apply", false, "Write the files")
	return c
}

func runApprove(c *cobra.Command, s kit.Session, ref string, digest model.Digest) int {
	shown, err := s.PluginApproval(c.Context(), ref)
	if err != nil {
		return approveEnding(s, ref, err)
	}
	line := fmt.Sprintf("gragas plugin approve %s --digest %s --apply", shown.Name+"@"+shown.Version, shown.Digest)
	if !s.Applying() {
		if s.JSON() {
			s.Document(shown)
			return s.End(kit.Ending{Word: model.Done, What: "nothing was recorded yet", Next: line})
		}
		what := "nothing was recorded yet: read what the approval covers, and approve the digest shown"
		if shown.Approved {
			what = "this digest is already approved; nothing to record"
		}
		return s.End(kit.Ending{Word: model.Done, What: what, Next: nextOf(shown, line),
			Lead: append(approvalScreen(shown), model.DryRunFooter{Flag: "--apply"})})
	}
	got, err := s.PluginApprove(c.Context(), ref, digest)
	if err != nil {
		return approveEnding(s, ref, err)
	}
	if s.JSON() {
		s.Document(got)
		return exit.Done
	}
	return s.End(kit.Ending{Word: model.Done, What: got.Name + "@" + got.Version + " is approved on this machine, as " + string(got.Digest)})
}

// nextOf is the line a dry run ends on: none when the digest is already
// approved.
func nextOf(shown model.PluginApproval, line string) string {
	if shown.Approved {
		return ""
	}
	return line
}

// approveEnding ends a run on an error: a plugin that is not meant, a digest
// that is not the bundle's, and a bundle the judge refuses are refused, each
// with nothing recorded and the line to type: the approve line that shows the
// digest when it is the digest that is not the bundle's, plugin list otherwise;
// anything else, a record that cannot be read among
// them, is failed.
func approveEnding(s kit.Session, ref string, err error) int {
	if coreapi.PluginRefused(err) {
		next := "gragas plugin list"
		if errors.Is(err, coreapi.ErrDigestGiven) {
			next = "gragas plugin approve " + ref
		}
		return s.End(kit.Ending{Word: model.Refused, What: err.Error(), Next: next})
	}
	return s.End(kit.Ending{Word: model.Failed, What: "the plugin could not be approved: " + err.Error()})
}

// approvalScreen is what the approval covers, as blocks above the closing word.
func approvalScreen(a model.PluginApproval) []model.Block {
	facts := []string{
		"plugin       " + a.Name + "@" + a.Version,
		"digest       " + string(a.Digest),
		"tool         " + a.Tool + " " + a.ToolVersions,
		"grants       " + string(a.GrantsDigest),
	}
	if a.Preset != "" {
		facts = append(facts, "preset       "+a.Preset)
	}
	if a.ResolveFrom != "" {
		facts = append(facts, "resolve from "+a.ResolveFrom)
	}
	if len(a.Install) > 0 {
		facts = append(facts, "install      "+strings.Join(a.Install, " ")+" (printed, never run)")
	}
	out := []model.Block{model.Heading{Text: "gragas plugin approve " + a.Name + "@" + a.Version}, model.List{Items: facts},
		model.Heading{Text: "grants since the last approval"}, model.List{Items: sinceLines(a)}}
	// A grant is a block of rows, not a table: a table drops its low-priority
	// columns on a narrow screen, and every grant's versions, version asked and
	// command line are shown before the yes at any width.
	for _, g := range a.Grants {
		var where string
		switch g.Where {
		case "path":
			where = "pinned places and " + g.Path
		case "script":
			where = "script " + g.ScriptAt + " in " + g.Manifest + ", run by " + g.Interpreter
		default:
			where = "pinned places alone"
		}
		line := "the script first"
		if g.WholeLine {
			line = "the plugin hands it the whole command line"
		}
		rows := []string{
			"found         " + where,
			"versions      " + g.Versions,
			"version asked " + strings.Join(g.VersionArgv, " "),
			"command line  " + line,
		}
		// A cache that steps share is a path from one step into the next: its names are shown before the yes, as resolve_from is.
		if len(g.CacheVars) > 0 {
			rows = append(rows, "cache vars    "+strings.Join(g.CacheVars, " ")+" (each a folder gragas keeps for this grant and project, which its later steps read)")
		}
		out = append(out, model.Heading{Text: "grant " + g.ID}, model.List{Items: rows})
	}
	return out
}

// sinceLines says how the grants compare with the plugin's prior approval
// : every grant is shown below either way, and a grant changed in
// place, one whose id stays and whose definition differs, is named as such.
func sinceLines(a model.PluginApproval) []string {
	s := a.Since
	switch {
	case s.Prior == "":
		return []string{"first approval of " + a.Name + ": every grant shown"}
	case !s.Recorded:
		return []string{"grants not recorded: every grant shown (last approved as " + s.Prior + ")"}
	case len(s.Changes) == 0:
		return []string{"no grant changed since " + s.Prior}
	}
	out := make([]string, 0, len(s.Changes))
	for _, c := range s.Changes {
		out = append(out, c.Change+" "+c.ID)
	}
	return out
}
