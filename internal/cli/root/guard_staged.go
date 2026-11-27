package root

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/pflag"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// GuardStaged judges the staged change of the working folder's repository,
// replaying each history line it appends through the command tree. It fails
// closed as guard edit does: from the moment its root is open, a crash is a
// refusal the root journals (finding 3).
func (k kitSession) GuardStaged(ctx context.Context, mergeHook bool) (model.Guard, *model.Refusal, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return model.Guard{}, nil, fmt.Errorf("the working folder could not be opened: %w", err)
	}
	ask := coreapi.StagedAsk{Root: root, Replay: k.s.replayer(), Run: k.s.rec.id(), Engine: k.s.build.String(), MergeHook: mergeHook}
	k.s.crashJournal = func() error {
		return coreapi.JournalGuardCrash(context.WithoutCancel(ctx), k.s.p.World, coreapi.GuardAsk{Root: root, Run: ask.Run, Engine: ask.Engine}, coreapi.GuardStagedCommand)
	}
	h, err := k.s.p.History("guard-staged")
	if err != nil {
		return model.Guard{}, nil, err
	}
	w := k.s.p.World
	w.History = h
	return coreapi.GuardStaged(ctx, w, ask)
}

// captured is a replay's run: the change its plan step was handed, which it
// hands back instead of planning (guard staged).
type captured struct{ change *coreapi.Change }

// replayer runs a history line's command as a script would, off a terminal
// and with nothing printed, through a command tree of its own over p, and
// answers the change it would plan. Only a plan command is replayed, with
// only its own flags, and never with --apply: a replay plans over the
// commit's tree, and reads nothing on this host and writes nothing.
func (s *session) replayer() coreapi.Replayer {
	return func(ctx context.Context, p port.Ports, line string) (*coreapi.Change, *model.Refusal, error) {
		words, err := lineWords(line)
		if err != nil {
			return nil, nil, err
		}
		args := words[1:]
		if err := replayable(ctx, args); err != nil {
			return nil, nil, err
		}
		sub := s.p
		sub.Args, sub.Stdout, sub.Draw = args, io.Discard, nil
		// The old ports read the host's disk, not the commit's tree; no plan
		// command reads them, and a replay that reached one is refused.
		sub.Ports = noOldPorts()
		p.Terminal = mute{}
		sub.World, sub.Diagnostics = p, mute{}
		sub.History = func(string) (port.History, error) { return p.History, nil }
		r := &session{p: sub, build: s.build, json: wantsJSON(args), capture: &captured{}}
		r.rec = &recorder{cfg: sub.Config, argv: words}
		tree := r.newRoot(ctx)
		tree.SetArgs(args)
		tree.SetOut(io.Discard)
		tree.SetErr(io.Discard)
		runErr := tree.ExecuteContext(ctx)
		switch {
		case r.capture.change != nil:
			return r.capture.change, nil, nil
		case r.ended != nil && r.ended.word != model.Done:
			ref, err := model.NewRefusal(r.ended.word, r.ended.what, nil, r.ended.next)
			if err != nil {
				return nil, nil, fmt.Errorf("it ended %s: %s", r.ended.word, r.ended.what)
			}
			return nil, &ref, nil
		case runErr != nil:
			return nil, nil, runErr
		}
		return nil, nil, errors.New("it planned no change")
	}
}

// replays are the commands a history line may name: each plans a change to
// the rulebook through the core's form (s.plan), over the ports the replay
// hands it, which read the commit's tree. Of the other commands that carry
// writes, gen reads the folder -C names through the old ports, contract
// fixture writes the folder --out names, and cache clear and journal recover
// change the host's own state, and preset writes the presets from the bindings and records no line: none of them is a plan a line records, and
// none is replayed. A command added to the tree is refused here
// until it is listed.
func replays() []string {
	return []string{
		"gragas bind", "gragas rebind", "gragas unbind", "gragas undo",
		"gragas import",
		"gragas decision new", "gragas decision supersede",
		"gragas init", "gragas migrate",
		"gragas rule confirm", "gragas rule edit", "gragas rule exclude", "gragas rule include", "gragas rule new",
		"gragas rule promote", "gragas rule renumber", "gragas rule replace", "gragas rule restore", "gragas rule retire",
		"gragas source edit", "gragas source new",
		"gragas term edit", "gragas term new", "gragas term retire",
	}
}

// replayable is nil when args are a line guard staged may make again: a
// command of replays, given only flags of its own. The root's flags
// (--config, --json and the rest) and -C are refused: commandLine never
// writes one into a line, and --config and -C name a path on this host. The
// words are parsed by a tree of their own, so the run's tree parses them once.
func replayable(ctx context.Context, args []string) error {
	tree := (&session{}).newRoot(ctx)
	c, rest, err := tree.Find(args)
	if err != nil {
		return err
	}
	if !slices.Contains(replays(), c.CommandPath()) {
		return errors.New(c.CommandPath() + " is not a command that plans a change to the rulebook, and only such a command is made again")
	}
	if err := c.ParseFlags(rest); err != nil {
		return err
	}
	inherited := c.InheritedFlags()
	var bad []string
	c.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "dir" || inherited.Lookup(f.Name) != nil {
			bad = append(bad, "--"+f.Name)
		}
	})
	if len(bad) > 0 {
		return errors.New("it sets " + strings.Join(bad, ", ") + ", which no line a plan records holds: a replay reads the commit's tree, and nothing this host names")
	}
	return nil
}

// noOldPorts are the old ports of a replay: each refuses, since each reaches
// the host and not the commit's tree.
func noOldPorts() legacy.Ports {
	n := noOld{}
	return legacy.Ports{Files: n, Writer: n, Schemas: n, Locks: n, Runs: n, TOML: n}
}

// errReplayHost is a replay that reached for the host's disk.
var errReplayHost = errors.New("a replayed command reads the commit's tree and nothing on this host")

type noOld struct{}

func (noOld) Repo(string) (legacy.Source, error) { return nil, errReplayHost }
func (noOld) Open(string) (io.ReadCloser, error) { return nil, errReplayHost }
func (noOld) Write(string, string, []byte) error { return errReplayHost }
func (noOld) Compile(legacy.Source, string, ...string) (legacy.Checker, error) {
	return nil, errReplayHost
}
func (noOld) Take(string, string) (legacy.Lock, error)  { return nil, errReplayHost }
func (noOld) Read([]byte) (map[string]any, error)       { return nil, errReplayHost }
func (noOld) Begin(string, []string, string) legacy.Run { return noRun{} }

// noRun is a replay's run record: one that is never kept.
type noRun struct{}

func (noRun) ID() string                 { return "" }
func (noRun) Change(string)              {}
func (noRun) WayBack(legacy.Undo)        {}
func (noRun) Finish(int, string, string) {}
func (noRun) Err() error                 { return errReplayHost }

// lineWords is the words of a history line's command: a gragas command, and
// never one with --apply, which the line of a yes never holds.
func lineWords(line string) ([]string, error) {
	words, err := splitLine(line)
	if err != nil {
		return nil, err
	}
	if len(words) < 2 || words[0] != "gragas" {
		return nil, errors.New("it is not a gragas command")
	}
	for _, a := range words[1:] {
		if a == applyFlag || strings.HasPrefix(a, applyFlag+"=") {
			return nil, errors.New("it holds " + applyFlag + ", which the line of a yes never holds")
		}
	}
	return words, nil
}

// splitLine is the words of a command line as applies and commandLine quote
// them: a word in single quotes is taken as it stands, '\” within it is a
// quote, and words are split at spaces and tabs.
func splitLine(line string) ([]string, error) {
	var words []string
	var w strings.Builder
	in, quoted := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case in && c == '\'':
			in = false
		case in:
			w.WriteByte(c)
		case c == '\'':
			in, quoted = true, true
		case c == '\\' && i+1 < len(line) && line[i+1] == '\'':
			w.WriteByte('\'')
			i++
		case c == ' ' || c == '\t':
			if w.Len() > 0 || quoted {
				words = append(words, w.String())
			}
			w.Reset()
			quoted = false
		default:
			w.WriteByte(c)
		}
	}
	if in {
		return nil, errors.New("a quote is not closed")
	}
	if w.Len() > 0 || quoted {
		words = append(words, w.String())
	}
	return words, nil
}

// mute is the terminal of a replay: no one there, and nothing shown.
type mute struct{}

func (mute) Width() int                                                  { return 0 }
func (mute) Colors() int                                                 { return 0 }
func (mute) Ask(context.Context, string) (string, error)                 { return "", io.EOF }
func (mute) Say(string) error                                            { return nil }
func (mute) Show(model.Screen) error                                     { return nil }
func (mute) Banner(string) error                                         { return nil }
func (mute) Line(context.Context) (string, error)                        { return "", io.EOF }
func (mute) CanAsk() bool                                                { return false }
func (mute) Pick(string, []model.Choice, string) (value string, ok bool) { return "", false }
