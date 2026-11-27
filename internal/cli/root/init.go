package root

// gragas init (S67): in a folder that is neither a project nor a rulebook it
// asks which, and the rulebook form writes identity and structure and the
// first history entry through the one form every plan command ends in. The
// project form is M6's.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The values of --kind, the answer to the first question.
const (
	kindProject  = "project"
	kindRulebook = "rulebook"
)

// An initFlags is what init is answered with. Every question init asks names,
// in code, the flag that answers it (cli.environment.14): askFor takes the
// flag and refuses one the command lacks.
type initFlags struct {
	kind, name, rbKind, version, language, licence, presets, docs string
	parent, parentVersion, parentHash, layer, part                string
	owners, imports                                               []string
}

func (s *session) initCmd(ctx context.Context) *cobra.Command {
	var f initFlags
	c := &cobra.Command{
		Use:   "init",
		Short: "Set up a project or a rulebook",
		Long: `Sets up the folder you are in. In a folder that is neither a
project nor a rulebook it asks which. A rulebook gets its
identity, its structure and a first history entry, and the
two lines that wire the guard are printed for you to add.
The project form is not built yet.

Without --apply it prints the plan and writes nothing.

  gragas init --kind rulebook --name my-rules --owner me
  gragas init --kind rulebook --name my-rules --owner me --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{writes: "with --apply"},
	}
	fl := c.Flags()
	fl.StringVar(&f.kind, "kind", "", "What the folder is: project or rulebook")
	fl.StringVar(&f.name, "name", "", "The rulebook's short name, a slug")
	fl.StringVar(&f.rbKind, "rulebook-kind", string(model.KindBase), "base, overlay, companion or fork")
	fl.StringVar(&f.version, "version", "1.0.0", "The starting version the first release publishes")
	fl.StringVar(&f.language, "language", "en", "The authoritative language")
	fl.StringVar(&f.licence, "licence", "CC-BY-4.0", "The licence of the rules")
	fl.StringVar(&f.presets, "presets-licence", "MIT", "The licence of the presets")
	fl.StringVar(&f.docs, "docs-url", "", "The base URL of the published pages")
	fl.StringArrayVar(&f.owners, "owner", nil, "Who owns the rulebook, shown in its views; it grants nothing (repeat for more)")
	fl.StringVar(&f.parent, "parent", "", "An overlay's parent rulebook")
	fl.StringVar(&f.parentVersion, "parent-version", "", "The parent's version the overlay pins")
	fl.StringVar(&f.parentHash, "parent-hash", "", "The content hash of the parent's pinned release, sha256:<64 hex digits>")
	fl.StringArrayVar(&f.imports, "import", nil, "A part taken from another rulebook, RULEBOOK@VERSION=PART,PART=sha256:<hash> (repeat for more)")
	fl.StringVar(&f.layer, "layer", "core", "The slug of the first layer")
	fl.StringVar(&f.part, "part", "core", "The slug of the first part")
	fl.Bool("apply", false, "Write the files")
	c.RunE = func(cmd *cobra.Command, _ []string) error { return exitErr(s.init(ctx, cmd, f)) }
	return c
}

const (
	initRulebook = "gragas init --kind rulebook"
	kindQuestion = "What is this folder? A project that follows a rulebook, or a rulebook"
)

func (s *session) init(ctx context.Context, cmd *cobra.Command, f initFlags) int {
	switch placeOf(s.p.World.Tree) {
	case PlaceRulebook, PlaceBoth:
		return s.end(ending{word: model.NotReady, what: "already a rulebook: this folder holds its identity file", next: "gragas validate"})
	case PlaceProject:
		return s.end(ending{word: model.NotReady, what: "already pinned: this folder follows a rulebook", next: "gragas status"})
	case PlaceNeither:
	}
	if f.kind == "" {
		got, ok := s.askFor(ctx, cmd, kindQuestion+" (rulebook or project)", "--kind")
		if !ok {
			return s.usage("init needs to know what this folder is", initRulebook, "gragas init --kind project")
		}
		f.kind = strings.ToLower(got)
	}
	switch f.kind {
	case kindRulebook:
	case kindProject:
		return s.end(ending{word: model.Refused, what: "the project form of init is not built yet, and it changed nothing", next: initRulebook})
	default:
		return s.usage("--kind is project or rulebook, not "+f.kind, initRulebook, "gragas init --kind project")
	}
	if f.name == "" {
		got, ok := s.askFor(ctx, cmd, "Name of the rulebook", "--name")
		if !ok {
			return s.usage("a rulebook needs a name", initRulebook+" --name my-rules")
		}
		f.name = got
	}
	root, ok := s.folder()
	if !ok {
		return 1
	}
	world, ok := s.withGit()
	if !ok {
		return exit.NotReady
	}
	// the owners are never read from git: a person's name goes into a shared
	// and published rulebook only when someone passes it
	if len(f.owners) == 0 {
		return s.initUsage(cmd, "a rulebook has at least one owner: name it with --owner", f.name, "--owner", "me")
	}
	form, err := f.form()
	if err != nil {
		return s.initUsage(cmd, err.Error(), f.name)
	}
	record, line, err := s.planLines(commandArgv(cmd, initRulebook, []string{"kind"}, map[string][]string{"name": {f.name}, "owner": f.owners}))
	if err != nil {
		return s.initUsage(cmd, err.Error(), f.name)
	}
	run := s.rec.id()
	if run == "" {
		run = "init-preview"
	}
	change, err := coreapi.InitRulebook(world, root, form, coreapi.Lines{Command: record, Applies: line, Who: cmd.CommandPath()}, run, s.build.Version)
	if err != nil {
		return s.initUsage(cmd, err.Error(), f.name)
	}
	s.sayInitDoc(cmd, coreapi.InitDoc(form, config.StateFile))
	return s.plan(ctx, planRun{cmd: cmd, change: change, next: line, ask: s.canAsk()})
}

// sayInitDoc holds doc as the run's document and says what to add to the
// rulebook: the guard's three hooks, the gate and the ignore line, after
// applying on a dry run.
func (s *session) sayInitDoc(cmd *cobra.Command, doc coreapi.Init) {
	s.data = doc
	head := "After applying, add these to the rulebook:"
	if on, err := cmd.Flags().GetBool("apply"); err == nil && on {
		head = "Add these to the rulebook:"
	}
	s.say(head)
	s.say("  pre-commit hook   " + doc.Guard[0])
	s.say("  agent settings    " + doc.Guard[1])
	s.say("  pre-merge-commit  " + doc.Guard[2])
	s.say("  the gate runs     " + doc.Gate)
	s.say("  git ignore        " + doc.Ignore)
	if doc.HashNote != "" {
		s.say(doc.HashNote)
	}
}

func (f initFlags) form() (model.InitForm, error) {
	g := model.Governance{Contribution: model.ContributionProposals, DecisionRequired: true, TrialReleases: 3, ReviewDays: 90}
	out := model.InitForm{
		Name: f.name, Kind: model.IdentityKind(f.rbKind), Version: f.version, Language: f.language,
		Licences: model.Licences{Rules: f.licence, Presets: f.presets}, DocsURL: f.docs, Owners: f.owners, Governance: g,
		Layer: model.Named{Slug: f.layer, Name: titled(f.layer)}, Part: model.Named{Slug: f.part, Name: titled(f.part)},
	}
	if f.parent != "" || f.parentVersion != "" {
		out.Extends = &model.Extends{Rulebook: f.parent, Version: f.parentVersion, ContentHash: model.Digest(f.parentHash)}
	}
	for _, raw := range f.imports {
		i, err := parseImport(raw)
		if err != nil {
			return out, err
		}
		out.Imports = append(out.Imports, i)
	}
	return out, nil
}

// parseImport reads one --import, RULEBOOK@VERSION=PART,PART=HASH: the pin, the
// parts taken and the content hash of the release.
func parseImport(raw string) (model.Import, error) {
	bad := fmt.Errorf("the import %q is not RULEBOOK@VERSION=PART,PART=sha256:<hash> (--import)", raw)
	pin, rest, ok := strings.Cut(raw, "=")
	if !ok {
		return model.Import{}, bad
	}
	parts, hash, ok := strings.Cut(rest, "=")
	if !ok {
		return model.Import{}, bad
	}
	at := strings.LastIndex(pin, "@")
	if at <= 0 || at == len(pin)-1 || parts == "" {
		return model.Import{}, bad
	}
	return model.Import{Rulebook: pin[:at], Version: pin[at+1:], Parts: strings.Split(parts, ","), ContentHash: model.Digest(hash)}, nil
}

// titled is a slug as a name: the first letter up.
func titled(slug string) string {
	if slug == "" {
		return slug
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

// askFor asks one question at a terminal, naming the flag that answers it. It
// answers false when nobody can answer, so the caller ends with usage.
func (s *session) askFor(ctx context.Context, cmd *cobra.Command, question, flag string) (string, bool) {
	form, err := newForm(cmd, question, flag)
	if err != nil {
		s.die("%v", err)
		return "", false
	}
	if !s.canAsk() {
		return "", false
	}
	got, err := s.p.World.Terminal.Ask(ctx, form.Prompt())
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false
	}
	got = strings.TrimSpace(got)
	return got, got != ""
}

// usage ends a run that was not told enough, with the lines that tell it.
func (s *session) usage(what string, lines ...string) int {
	return s.end(ending{word: model.Usage, what: what, next: lines[0]})
}

// initUsage ends usage with what, and its line is init's rulebook form with
// --name name and extra, as one pasted line (pasteLine). A name no line can
// carry gives the help of init in its place, and what names the value
// (A6): the name is never put in the line some other way.
func (s *session) initUsage(cmd *cobra.Command, what, name string, extra ...string) int {
	line, err := s.line(append(append(strings.Fields(initRulebook), "--name", name), extra...))
	if err != nil {
		return s.usage(what+"; "+err.Error(), helpLine(cmd))
	}
	return s.usage(what, line)
}

// dryRun is the label a dry run gives git for its run: no run id is invented,
// and the adapter still refuses an empty one.
const dryRun = "dry-run"

// withGit is the world of this run with the History port made from the git the
// settings name. When they name none, the run has ended not ready and ok is
// false.
func (s *session) withGit() (world port.Ports, ok bool) {
	world = s.p.World
	run := s.rec.id()
	if run == "" {
		run = dryRun
	}
	h, err := s.p.History(run)
	switch {
	case errors.Is(err, ErrNoGit):
		s.end(ending{word: model.NotReady, what: "init needs git to say who made the rulebook: set programs.git in the settings file to the absolute path of git", next: "edit the settings file"})
		return world, false
	case err != nil:
		s.end(ending{word: model.Failed, what: "git could not be set up: " + err.Error()})
		return world, false
	}
	world.History = h
	return world, true
}
