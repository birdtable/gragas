// Package git is the one adapter that runs git (arch.ports.6, cli.process.1).
// It serves the History port: the identity (Who), the merge base, the files
// and blobs of a tree or the index, and the tags (history.go). It serves
// the Remote port too: a tag fetched into the releases cache from a path on
// this machine or from a host over https or ssh, a commit of named paths,
// and a signed annotated tag (remote.go, shell.go).
//
// git is run by the absolute path the settings file names (programs.git),
// never from PATH and never from a literal (code.config.1), and so are the
// ssh and the credential helper git runs for a host (programs.ssh,
// programs.credential). Before each run the path is walked from / with
// Lstat, and every folder, every link and the file itself must be root's or
// this account's and not writable by group or others (program.go).
// Each run is a verb of the closed table in run.go, with a fixed argv, a
// bare environment, a cap on what it prints and a deadline (cli.process.2).
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/birdtable/gragas/internal/model"
)

// Setting is the settings key that names git's path.
const Setting = "programs.git"

// FailureKind is the kind of a failure of git itself: a run that ends at the
// deadline, prints past the cap or exits in a way the verb does not expect.
const FailureKind model.Kind = "git"

// Config is what the adapter is made from. Every field comes from config,
// never from a literal and never from the environment directly.
type Config struct {
	// Git is the absolute path the settings file gives programs.git, "" when
	// it names none.
	Git string
	// Releases is the releases cache, an absolute folder, which Fetch fetches
	// into; "" when there is none.
	Releases string
	// SSH is the absolute path the settings file gives programs.ssh, the
	// ssh git runs to fetch from an ssh:// host; "" when it names none.
	SSH string
	// Credential is the absolute path the settings file gives
	// programs.credential, the one credential helper a fetch from an
	// https:// host may ask; "" when it names none, and then no credential
	// is sent.
	Credential string
	// Agent is the socket of the account's ssh agent, as the caller read
	// SSH_AUTH_SOCK, which an ssh fetch is handed; "" hands it none. Any
	// other value is an absolute, clean path (HomeOK), else New refuses it.
	Agent string
	// Home is the account's home folder, handed to git as HOME so it reads
	// the account's own git config. "" hands it none; any other value is an
	// absolute, clean path (HomeOK), else New refuses it.
	Home string
	// Run is the id of this run, which a failure names.
	Run string
	// Mounts are config.Config's Mounts: folders on whose disks a name
	// mounted nosuid, or noexec without nosuid, is judged by its owners in
	// place of refused, in the walk of each program.
	Mounts []string
}

// Git runs git for one run. Make it with New.
type Git struct {
	cfg Config
	// wait is how long a run may take: deadline, unless a test of this
	// package set it shorter.
	wait time.Duration
	ran  []Ran
	// band is what the git's version allows, read once, by the first
	// History method that reads objects (history-version-bands); found is the version.
	band  band
	found version
	// trees is the listing of each commit this value has read, by the commit's
	// full object name alone: a commit never changes, so a ref or the index,
	// which do, is never a key. tree answers a copy of it.
	trees map[string][]entry
	// opening, when a test of this package sets it, runs after the last
	// Lstat of a named path and before the path is opened, with the name
	// of the path in the root, so a test can put another file there between
	// the check and the open. It runs only in a test binary.
	opening func(name string)
	// renaming, when a test of this package sets it, is how a fetch moves its
	// files into the cache, in place of confine.RenameNoReplace, so a test can
	// answer what a system's rename answers. It runs only in a test binary.
	renaming func(root *os.Root, from, to string) error
	// ca, when a test of this package sets it, is the CA file an https
	// fetch checks its server against (testCA). It is read only in a test
	// binary.
	ca string
}

// New makes the adapter. It runs nothing and reads no file: a git that is
// missing or unsafe is found when it is first run, and every run checks it
// again. A run id is required, since every failure names it.
func New(cfg Config) (*Git, error) {
	if cfg.Run == "" {
		return nil, model.ErrNoRun
	}
	if cfg.Home != "" && !HomeOK(cfg.Home) {
		return nil, fmt.Errorf("the home folder %q is not an absolute, clean path: git would read a config from the folder it runs in", cfg.Home)
	}
	if cfg.Agent != "" && !HomeOK(cfg.Agent) {
		return nil, fmt.Errorf("the agent's socket %q is not an absolute, clean path", cfg.Agent)
	}
	// the home and the agent's socket are made this system's own form once,
	// here at the edge, so env hands git the paths HomeOK judged
	cfg.Home, cfg.Agent = filepath.FromSlash(cfg.Home), filepath.FromSlash(cfg.Agent)
	return &Git{cfg: cfg, wait: deadline}, nil
}

// Ran is the record of one run of git: the verb, the argv as it was run, its
// whole environment, the folder, and the exit code (-1 when it did not
// exit).
type Ran struct {
	Verb string
	Argv []string
	Env  []string
	Dir  string
	Code int
}

// Ran answers every run this adapter made, in order, a copy.
func (g *Git) Ran() []Ran {
	out := make([]Ran, len(g.ran))
	for i, r := range g.ran {
		r.Argv = append([]string(nil), r.Argv...)
		r.Env = append([]string(nil), r.Env...)
		out[i] = r
	}
	return out
}

// Who answers the name and address the repository at root commits as, from
// one `git config` call: the repository's own config, then the account's.
// The machine-wide config is not read: it names no person. Each is one line
// with its control sequences stripped. None configured, or either half
// missing, is a *NotReady naming git config; git that is missing is a
// *NotReady naming programs.git; git that is unsafe to run is a *Refused;
// git that hangs, prints past the cap or fails is a *Failed with the run id.
func (g *Git) Who(ctx context.Context, root model.Root) (model.Identity, error) {
	if !filepath.IsAbs(string(root)) {
		return model.Identity{}, fmt.Errorf("the root %q is not absolute", string(root))
	}
	out, err := g.run(ctx, verbWho, string(root))
	if err != nil {
		if ex, ok := errors.AsType[*exited](err); ok && ex.code == 1 && len(ex.stderr) == 0 {
			// git config exits 1, saying nothing, when no key matches
			return model.Identity{}, noIdentity("user.name and user.email")
		}
		// no git, or one unsafe to run: the answer is the check's own, as it is
		if nr, ok := errors.AsType[*NotReady](err); ok {
			return model.Identity{}, nr
		}
		if rf, ok := errors.AsType[*Refused](err); ok {
			return model.Identity{}, rf
		}
		return model.Identity{}, g.failed("git config could not be read", err)
	}
	id, missing, err := parseIdentity(out)
	switch {
	case err != nil:
		return model.Identity{}, g.failed("git config printed what it does not print", err)
	case missing != "":
		return model.Identity{}, noIdentity(missing)
	}
	return id, nil
}

// HomeOK is whether h can be handed to git as HOME: absolute, clean and free
// of control characters. A relative HOME makes git read the account's config
// from the folder it runs in, which is the repository, so a file the head
// commits would be read as the account's own.
func HomeOK(h string) bool {
	// The model's slash form is judged as this system's own: env hands git the same native form.
	return nativeClean(filepath.FromSlash(h)) && !strings.ContainsFunc(h, unicode.IsControl)
}
