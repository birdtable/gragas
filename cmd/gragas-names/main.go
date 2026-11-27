// Command gragas-names runs the rulebook's names check as a gate member: it
// loads the format 3 rulebook in a folder through LoadBookNamed with the
// private names list on, prints the place of each names finding, and fails
// when there is one. It is built and run from this repository's Taskfile and
// is not shipped. The names check reads every line of every file the load
// reads, and each path (gragas#1741).
//
// The list is read from the settings file's names-file key and never from the
// environment: a variable can be set by anything that starts the run, and the
// key is in a file the owner holds. A run with the variable set says on stderr
// that it is not read and goes on with the key (gragas#1625). A variable that
// chooses another settings file, or a home but the account's own, is not ready
// (config.OwnersSettingsFile, gragas#1629, #1958). A run with no list, a list it cannot read
// or a line it cannot parse, or a rulebook it read no rule of, is not ready
// (exit 4) and says nothing was scanned, so a gate that cannot look never
// reads as a gate that looked and found nothing (supply.secrets.4). A finding
// prints the file and line and a fixed phrase, never the name, and the load
// masks every path while a list is held.
//
// main is the only place that exits (go.errors.4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/assets"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/host/term"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
)

func wiring() Env {
	return Env{Getenv: os.Getenv, AccountHome: config.SystemHome, Read: hosttrust.ReadOwned, World: world}
} // what main hands the run
func main() {
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system (gragas#1614).
	os.Exit(term.Main(context.Background(), "gragas-names", os.Stderr, func(ctx context.Context, args []string) int {
		return run(ctx, args, wiring(), os.Stdout, os.Stderr)
	}))
}

// world is the machine's Tree and the embedded format schemas: all the load
// reads, with a Plugins port that serves nothing. The close lets go of every folder the Tree opened, since Windows
// refuses to remove a folder that is held open (gragas#927).
func world(cfg config.Config) (port.Ports, func() error) {
	tree := &hostfs.Tree{Mounts: cfg.Mounts}
	return port.Ports{Tree: tree, Owned: tree, Schema: jsonschema.Embedded(assets.New(), "format/schema"), Plugins: noPlugins{}}, tree.Close
}

// Env is what the run reads from the machine, so a test hands it doubles.
type Env struct {
	Getenv      func(string) string
	AccountHome func() (string, error) // the system's record of the home, never a variable (gragas#1958)
	Read        func(path string, mounts []string) ([]byte, error)
	// World is the ports the load reads through, and how to let go of them.
	World func(config.Config) (port.Ports, func() error)
}

// namesKey is the settings file's key that holds the list's path.
const namesKey = "names-file"

// nothingScanned starts every line of a run that read no rulebook to hold to
// the list, or no list to hold it to.
const nothingScanned = "gragas-names: nothing was scanned: "

// run is the program with its streams, so a test drives it without a process.
func run(ctx context.Context, args []string, env Env, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("gragas-names", flag.ContinueOnError)
	set.SetOutput(stderr)
	dir := set.String("C", ".", "the repository, whose rulebook is checked")
	notAsked := set.Bool("not-asked", false, "say the check was not asked on this machine, which holds no list; refused where the list is named")
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return exit.Usage
	}
	out, diag := &say.Speaker{W: stdout}, &say.Speaker{W: stderr}
	code := 0
	if p := config.OwnersSettingsFile(env.Getenv, env.AccountHome); p != nil {
		diag.Text("gragas-names: the names check cannot run: " + p.What + ". " + p.Next + "\n")
		code = exit.NotReady
	} else if *notAsked {
		code = notAskedHere(env, out, diag)
	} else {
		code = check(ctx, *dir, env, out, diag)
	}
	if out.Err != nil || diag.Err != nil {
		return exit.Failed
	}
	return code
}

// check is the gate: its exit code, with the findings on out and the reason on diag.
func check(ctx context.Context, dir string, env Env, out, diag *say.Speaker) int {
	cfg, err := config.Load(withoutNamesVar(env.Getenv, diag), "", env.Read)
	if err != nil {
		diag.Text(nothingScanned + "the settings file cannot be read: " + err.Error() + " (the list is named by the settings file's " + namesKey + " key)\n")
		return exit.NotReady
	}
	if cfg.NamesFile == "" {
		diag.Text(nothingScanned + "the settings file has no " + namesKey + " key. Set it to the private names list's path\n")
		return exit.NotReady
	}
	p, done := env.World(cfg)
	r, err := load(ctx, p, dir, cfg.NamesFile)
	// The repository is read no more: let go of it before a word is said.
	if cerr := done(); err == nil && cerr != nil {
		diag.Text("gragas-names: the repository could not be let go of: " + cerr.Error() + "\n")
		return exit.Failed
	}
	switch {
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		// The error lists the paths it found, which the load has not masked:
		// the line says what is wrong in its own words.
		diag.Text(nothingScanned + "the folder " + dir + " holds no one rulebook.yaml of format 3 to read\n")
		return exit.NotReady
	case errors.Is(err, coreapi.ErrNotReady):
		// It says no path of the list (readBookNames): a refused or a bad
		// list, or a format newer than the engine.
		diag.Text(nothingScanned + err.Error() + " (the list is named by the settings file's " + namesKey + " key)\n")
		return exit.NotReady
	case err != nil:
		diag.Text("gragas-names: " + err.Error() + "\n")
		return exit.Failed
	case r.NamesOff != "":
		diag.Text(nothingScanned + r.NamesOff + " (the list is named by the settings file's " + namesKey + " key)\n")
		return exit.NotReady
	case len(r.Book.Rules) == 0:
		diag.Text(nothingScanned + "the rulebook in " + dir + " holds no rule the load read\n")
		return exit.NotReady
	}
	found := 0
	for _, f := range r.NamesFound() {
		out.Text(f.Entity + ": " + f.Msg + "\n")
		found++
	}
	if found > 0 {
		diag.Text(fmt.Sprintf("gragas-names: %d finding(s) hold a listed name\n", found))
		return exit.Failed
	}
	out.Text(fmt.Sprintf("names: %d rules read, no line and no path holds a listed name\n", len(r.Book.Rules)))
	return exit.Done
}

// load opens the folder and reads its rulebook with the list on.
func load(ctx context.Context, p port.Ports, dir, namesFile string) (coreapi.BookReport, error) {
	root, err := p.Tree.Open(dir)
	if err != nil {
		return coreapi.BookReport{}, err
	}
	return coreapi.LoadBookNamed(ctx, p, root, namesFile)
}

// withoutNamesVar is getenv with the names variable hidden, so config.Load,
// which lets the variable beat the key, finds the key's list only. A run that
// has the variable set (every fleet session sets it) is told on diag that it
// is not read, never what it holds, and goes on (gragas#1625).
func withoutNamesVar(getenv func(string) string, diag *say.Speaker) func(string) string {
	if getenv(config.NamesFileVar) != "" {
		diag.Text("gragas-names: " + config.NamesFileVar + " is set and not read: the list comes from the settings file's " + namesKey + " key\n")
	}
	return func(k string) string {
		if k == config.NamesFileVar {
			return ""
		}
		return getenv(k)
	}
}

// notAskedHere is the run on a machine that holds no list: a release on a lent session passes -not-asked
// (NAMES_NOT_ASKED=1, gragas#1566), and the member says it was not asked rather than fail for a list that is not there
// by design. The machine that owes the check cannot skip it: where the settings file's key names a list, the flag is
// refused, and the run is red. The variable is not read here either (gragas#1625: every fleet session sets it, a lent
// one too), so it neither refuses the flag nor stands in for the key. It reads the settings file alone, to see whether
// the key is there, and never opens the list.
func notAskedHere(env Env, out, diag *say.Speaker) int {
	cfg, err := config.Load(withoutNamesVar(env.Getenv, diag), "", env.Read)
	if err != nil {
		diag.Text("gragas-names: -not-asked (NAMES_NOT_ASKED) is refused: the settings file cannot be read to see whether names is asked here: " + err.Error() + "\n")
		return exit.Usage
	}
	if cfg.NamesFile != "" {
		diag.Text("gragas-names: names is asked here: the settings file's " + namesKey + " key names a list, so -not-asked (NAMES_NOT_ASKED) is refused; run the check without it\n")
		return exit.Usage
	}
	out.Text("names: not asked: this machine's settings file names no list, so the private list was not read here; the release run on the operator's Mac reads it (gragas#1566)\n")
	return exit.Done
}
