package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/gitenv"
	"github.com/birdtable/go-proc/proc"
)

// The bounds of every run of git (cli.process.2): a run past the deadline is
// ended and failed, and a run that prints past a cap is failed, never cut
// and read as if it were whole. go-proc holds each stream to the verb's cap
// and ends the run there; stderr is held to maxStderr after the run.
const (
	deadline  = 10 * time.Second
	maxStdout = 64 << 10
	maxStderr = 4 << 10
	// maxListing is the cap of the verbs that print a tree, the index, the
	// tags or a file's content: a tree listing or a rules file is past 64 KiB.
	maxListing = 4 << 20
	// drain is how long a run reads an output pipe a process that left its
	// tree still holds once the tree is killed (go-proc's Cmd.Drain): the
	// 10 s go-proc gave a zero Drain before v0.4.0, which honours a zero as
	// none.
	drain = 10 * time.Second
)

// The verbs of the table.
const (
	// verbWho is the verb Who runs.
	verbWho = "config-identity"
	// verbVersion reads git's version, once per adapter, for its band (history-version-bands).
	verbVersion = "version"
	// verbPromisor reads whether the repository fetches missing objects
	// from a promisor remote, for a git too old for --no-lazy-fetch (history-version-bands).
	verbPromisor = "config-promisor"
	// verbFileMode reads the repository's core.fileMode, whether git takes a
	// file's exec bit for its mode (go-proc's gitenv.CommitMode).
	verbFileMode = "config-filemode"
	// verbShallow reads whether the repository is a shallow clone.
	verbShallow = "is-shallow"
	// verbResolve names the commit a rev is.
	verbResolve = "resolve-commit"
	// verbBase is the merge bases of two commits.
	verbBase = "merge-base"
	// verbTree lists the files of a commit's tree.
	verbTree = "tree-files"
	// verbIndex lists the files of the index.
	verbIndex = "index-files"
	// verbBlob prints one object's content.
	verbBlob = "blob"
	// verbBlobs prints each object named on stdin, headed by its name, type
	// and size: the files one commit reads, in one run.
	verbBlobs = "blob-objects"
	// verbTags lists the tags and the commit each names.
	verbTags = "tags"
	// verbMergeHead is where the worktree's merge state is kept: the
	// MERGE_HEAD file git writes when a merge stops before its commit. It
	// is asked with no --path-format: absolute resolves a link at the name
	// before the adapter can see it, and the default resolves nothing.
	verbMergeHead = "merge-head-path"
	// verbCommitsBetween lists the commits reachable from one commit and
	// not from another, each with its parents, every commit after its
	// parents.
	verbCommitsBetween = "commits-between"
	// verbRange lists the commits of a range, one full object name a line.
	verbRange = "range-commits"
	// verbCommits prints each commit named on stdin as its object stores it.
	verbCommits = "commit-objects"
	// verbWork prints the object name of each work-tree file named on stdin,
	// its bytes hashed as they are, and writes nothing.
	verbWork = "work-objects"

	// The verbs of Remote, the only ones that write. Each is marked writes,
	// so run puts core.hooksPath at an empty folder of the run's own before
	// it, and no hook of the repository or the account runs; none of them
	// stages a path that was not named.

	// verbInit makes the bare repository of the releases cache.
	verbInit = "init-cache"
	// verbFetch fetches one tag from a path, and nothing else.
	verbFetch = "fetch-tag"
	// verbFetchHTTPS fetches one tag from an https URL with no credential.
	verbFetchHTTPS = "fetch-tag-https"
	// verbFetchHelper fetches one tag from an https URL, with the credential
	// helper the settings name (programs.credential) as the only one.
	verbFetchHelper = "fetch-tag-https-helper"
	// verbFetchSSH fetches one tag from an ssh URL through the ssh the
	// settings name (programs.ssh), with fixed options.
	verbFetchSSH = "fetch-tag-ssh"
	// verbPrefix is where in its repository the folder git runs in is.
	verbPrefix = "show-prefix"
	// verbWorkTop is whether the folder is in a work tree, and where in it.
	verbWorkTop = "work-top"
	// verbHash stores the bytes on stdin as a blob, with no filter.
	verbHash = "hash-blob"
	// verbReadTree fills the run's own index from a commit's tree.
	verbReadTree = "read-tree"
	// verbIndexInfo sets the named paths in the run's own index.
	verbIndexInfo = "index-info"
	// verbIndexReal sets the named paths in the repository's index, after
	// the commit, so it holds what the commit holds for them and nothing else
	// of it changes.
	verbIndexReal = "index-info-real"
	// verbWriteTree writes the run's own index as a tree.
	verbWriteTree = "write-tree"
	// verbCommitTree makes the commit of a tree on its parent, unsigned.
	verbCommitTree = "commit-tree"
	// verbUpdateRef moves HEAD to the new commit only if it still names the
	// parent.
	verbUpdateRef = "update-ref"
	// verbTag makes one annotated tag, never signed: no config of the
	// account's or the repository's can sign it (release-pin-by-hash).
	verbTag = "tag-annotated"
)

// The slots of a verb's argv: where a value goes, and of which kind. A slot
// is a whole argument of the row, and a value is put there only by run.
const (
	// slotCommitOf is a rev, peeled to the commit it names.
	slotCommitOf = "\x00rev^{commit}"
	// slotOID is an object name read from git's own output.
	slotOID = "\x00oid"
	// slotNotOID is an object name whose commits a range leaves out.
	slotNotOID = "\x00^oid"
	// slotPath is an absolute, clean path: a source, a folder or a file.
	slotPath = "\x00path"
	// slotTag is the name of a tag.
	slotTag = "\x00tag"
	// slotTagSpec is the refspec that fetches the tag of that name to the
	// same name, and never moves one already there.
	slotTagSpec = "\x00tagspec"
	// slotURL is an https or ssh URL, held to parseURL.
	slotURL = "\x00url"
	// slotHelper is the credential helper line git hands its shell, made
	// only by helperLine (shell.go).
	slotHelper = "\x00helper"
	// slotSSH is the ssh command line git hands its shell, made only by
	// sshLine (shell.go).
	slotSSH = "\x00ssh"
)

// slots are the kind each slot takes and how its value is written there.
// A slot may follow fixed text in its argument, as -c key=value does.
func slots() map[string]struct {
	kind   valueKind
	render func(string) string
} {
	same := func(s string) string { return s }
	return map[string]struct {
		kind   valueKind
		render func(string) string
	}{
		slotCommitOf[1:]: {kindRev, func(s string) string { return s + "^{commit}" }},
		slotOID[1:]:      {kindOID, same},
		slotNotOID[1:]:   {kindOID, func(s string) string { return "^" + s }},
		slotPath[1:]:     {kindPath, same},
		slotTag[1:]:      {kindTag, same},
		slotTagSpec[1:]:  {kindTag, func(s string) string { return "refs/tags/" + s + ":refs/tags/" + s }},
		slotURL[1:]:      {kindURL, same},
		slotHelper[1:]:   {kindHelper, same},
		slotSSH[1:]:      {kindSSH, same},
	}
}

// The options of a fetch, in the one place a runner that replaces run can
// carry them. Every fetch refuses each protocol but its own,
// clears the credential helpers, checks every object and starts no gc, and
// runs with the account's git config unread (noAccount), so no url
// rewrite, proxy, header, cookie file, helper or ssh command of the
// account's reaches it.
// Each is a function, so no caller can change the list another run reads.

// fetchFile is the transport of a fetch from a path on this machine.
func fetchFile() []string {
	return []string{"-c", "protocol.allow=never", "-c", "protocol.file.allow=always", "-c", "credential.helper="}
}

// fetchHTTP is the transport of a fetch over https: the certificate
// checked, no proxy, not even the environment's (an empty http.proxy sets
// curl's to none), no redirect followed, no extra header or cookie file,
// and a host that sends under 1000 bytes a second for 30 seconds dropped,
// so a stalled transfer ends inside the fetch's deadline.
func fetchHTTP() []string {
	return []string{
		"-c", "protocol.allow=never", "-c", "protocol.https.allow=always", "-c", "credential.helper=",
		"-c", "http.sslVerify=true", "-c", "http.proxy=", "-c", "http.followRedirects=false",
		"-c", "http.extraHeader=", "-c", "http.cookieFile=",
		"-c", "http.lowSpeedLimit=1000", "-c", "http.lowSpeedTime=30",
	}
}

// fetchSSH is the transport of a fetch over ssh: git runs the ssh line, as
// plain OpenSSH, with no -G probe of what it is.
func fetchSSH() []string {
	return []string{
		"-c", "protocol.allow=never", "-c", "protocol.ssh.allow=always", "-c", "credential.helper=",
		"-c", "ssh.variant=ssh", "-c", "core.sshCommand=" + slotSSH,
	}
}

// fetchRow is a fetch's argv: its transport's pins, what every fetch pins
// after them, the fetch itself (one tag, nothing else, no submodule, no
// FETCH_HEAD and no gc) and its own options, then its source and the tag's
// refspec after --end-of-options.
func fetchRow(transport, own []string, source string) []string {
	checks := []string{"-c", "transfer.fsckObjects=true", "-c", "gc.auto=0", "-c", "maintenance.auto=false"}
	fetch := []string{"fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", "--no-auto-gc", "-q"}
	out := slices.Concat(transport, checks, fetch, own)
	return append(out, "--end-of-options", source, slotTagSpec)
}

// verb is one row of the table: the argv after the prefix, with its slots,
// the cap on what it prints, and whether it reads objects. A row that reads
// objects runs only once the git's band is read, and takes --no-lazy-fetch
// when the git has it (history-version-bands).
type verb struct {
	args    []string
	max     int
	objects bool
	// stdin is whether the run is handed bytes on its standard input, and
	// index whether it runs on an index of the run's own (GIT_INDEX_FILE).
	stdin, index bool
	// long is a run that may take twelve times the deadline: a fetch.
	long bool
	// writes is a verb that writes the repository, which runs with
	// core.hooksPath at the run's own empty folder, so no hook runs.
	writes bool
	// noAccount is a verb that reads no git config of the account's:
	// GIT_CONFIG_GLOBAL is the null device, so neither ~/.gitconfig nor the
	// XDG one is read.
	noAccount bool
	// noHome is a verb run with no home of the person's, go-proc's
	// gitenv.NoHome at the run's own empty folder, so curl reads no .netrc,
	// a credential the settings do not name. A fetch with the
	// helper keeps the home, which its helper may need.
	noHome bool
	// agent is a verb whose ssh may ask the account's agent, which is
	// handed SSH_AUTH_SOCK when the adapter was given one (Config.Agent).
	agent bool
	// tls is a verb that checks a server's certificate: in a test of this
	// package, and only there, it may be handed the test's own CA.
	tls bool
}

// verbs answers the closed table of what this adapter may run: each verb and
// its argv after the program and the prefix, fixed. A caller's value enters
// only at a slot, and only as a value whose kind was checked on its parsed
// form; a caller's path never enters. The folder a verb runs in is never in
// the argv. A verb not here is refused before anything starts
// (cli.process.1). Every verb prints a machine format: NUL-separated (-z,
// %00) where a value could hold a newline, and a fixed --format.
//
// config-identity reads user.name and user.email in one call, NUL-separated
// (-z), so a value can hold no line that reads as another key.
func verbs() map[string]verb {
	return map[string]verb{
		verbWho:      {args: []string{"config", "-z", "--get-regexp", `^user\.(name|email)$`}, max: maxStdout},
		verbVersion:  {args: []string{"version"}, max: maxStdout},
		verbPromisor: {args: []string{"config", "-z", "--get-regexp", `^(extensions\.partialclone|remote\..+\.promisor)$`}, max: maxStdout},
		verbFileMode: {args: []string{"config", "--type=bool", "--get", "core.fileMode"}, max: maxStdout},
		verbShallow:  {args: []string{"rev-parse", "--is-shallow-repository"}, max: maxStdout},
		verbResolve:  {args: []string{"rev-parse", "--verify", "-q", "--end-of-options", slotCommitOf}, max: maxStdout, objects: true},
		verbBase:     {args: []string{"merge-base", "--all", "--end-of-options", slotOID, slotOID}, max: maxStdout, objects: true},
		verbTree: {args: []string{
			"ls-tree", "-r", "-z", "-l", "--full-tree",
			"--end-of-options", slotOID,
		}, max: maxListing, objects: true},
		verbIndex: {args: []string{
			"ls-files", "-z", "--full-name", "--format=%(objectmode) %(objectname) %(objectsize) %(stage)%x09%(path)", "--", ":/",
		}, max: maxListing, objects: true},
		verbBlob: {args: []string{"cat-file", "blob", "--end-of-options", slotOID}, max: maxListing, objects: true},
		// the cap is the content's and a header's room beyond it, so a file Blob reads
		// is a file this reads, alone or with others (the chunking counts the headers)
		verbBlobs: {args: []string{"cat-file", "--batch"}, max: maxListing + blobHeader, objects: true, stdin: true},
		verbTags: {args: []string{
			"for-each-ref", "--format=%(refname)%00%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)", "refs/tags/",
		}, max: maxListing, objects: true},
		verbMergeHead: {args: []string{"rev-parse", "--git-path", "MERGE_HEAD"}, max: maxStdout},
		verbCommitsBetween: {args: []string{
			"rev-list", "--topo-order", "--reverse", "--parents", "--end-of-options", slotOID, "^" + slotOID,
		}, max: maxListing, objects: true},
		verbRange: {args: []string{"rev-list", "--end-of-options", slotNotOID, slotOID, "--"}, max: maxListing, objects: true},
		// the objects as stored: git log re-codes a message from the
		// encoding its author's header names and cuts it at a NUL, so a
		// message is read here, never from log
		verbCommits: {args: []string{"cat-file", "--batch"}, max: maxListing, objects: true, stdin: true},
		// --no-filters: no attribute is read for a path, so no end-of-line
		// conversion and no clean program the repository's config names runs,
		// and no -w, so nothing is written; it reads no object
		verbWork: {args: []string{"hash-object", "--no-filters", "--stdin-paths"}, max: maxListing, stdin: true},
		verbInit: {args: []string{"init", "--bare", "-q"}, max: maxStdout, writes: true, noAccount: true},
		verbFetch: {args: fetchRow(fetchFile(), []string{"--upload-pack=git-upload-pack --strict"}, slotPath),
			max: maxStdout, objects: true, long: true, writes: true, noAccount: true},
		// A host runs its own upload-pack: --upload-pack is a word for its
		// shell, and a host's git-shell refuses any but the plain one.
		verbFetchHTTPS: {args: fetchRow(fetchHTTP(), nil, slotURL),
			max: maxStdout, objects: true, long: true, writes: true, noAccount: true, noHome: true, tls: true},
		verbFetchHelper: {args: fetchRow(slices.Concat(fetchHTTP(), []string{"-c", "credential.helper=" + slotHelper}), nil, slotURL),
			max: maxStdout, objects: true, long: true, writes: true, noAccount: true, tls: true},
		verbFetchSSH: {args: fetchRow(fetchSSH(), nil, slotURL),
			max: maxStdout, objects: true, long: true, writes: true, noAccount: true, agent: true},
		verbPrefix:    {args: []string{"rev-parse", "--show-prefix"}, max: maxStdout},
		verbWorkTop:   {args: []string{"rev-parse", "--is-inside-work-tree", "--show-prefix"}, max: maxStdout},
		verbHash:      {args: []string{"hash-object", "-w", "--no-filters", "--stdin"}, max: maxStdout, objects: true, stdin: true, writes: true},
		verbReadTree:  {args: []string{"read-tree", slotOID}, max: maxStdout, objects: true, index: true, writes: true},
		verbIndexInfo: {args: []string{"update-index", "-z", "--index-info"}, max: maxStdout, objects: true, stdin: true, index: true, writes: true},
		verbIndexReal: {args: []string{"update-index", "-z", "--index-info"}, max: maxStdout, objects: true, stdin: true, writes: true},
		verbWriteTree: {args: []string{"write-tree"}, max: maxStdout, objects: true, index: true, writes: true},
		verbCommitTree: {args: []string{"commit-tree", "--no-gpg-sign", "-p", slotOID, "-F", "-", slotOID},
			max: maxStdout, objects: true, stdin: true, writes: true},
		verbUpdateRef: {args: []string{"update-ref", "-m", "gragas commit", "HEAD", slotOID, slotOID},
			max: maxStdout, objects: true, writes: true},
		verbTag: {args: []string{
			"-c", "tag.gpgSign=false", "-c", "tag.forceSignAnnotated=false",
			"tag", "-a", "-F", "-", "--end-of-options", slotTag, slotOID,
		}, max: maxStdout, objects: true, stdin: true, writes: true},
	}
}

// prefix is what every run puts before the verb: replace refs never swap
// one object for another, the commit-graph file, whose parent lists git
// trusts over the objects', is never read, a read takes no
// optional lock, and the
// repository's fsmonitor program is not started by a read of the index;
// the advice git prints for the graft file env names, which it opens even
// empty, is kept off stderr, where a run's exit is read.
// --no-lazy-fetch, for a row that reads objects on a git that has it, keeps
// a partial clone from fetching a missing object from its promisor remote,
// the network and the repository's own ssh command (history-version-bands).
// It is not go-proc's gitenv.ReadArgs: proc.Cmd's Dir already runs git in
// the folder, so ReadArgs's -C would say it twice, and a verb that writes
// names a hooks folder of its own (runWith), where ReadArgs names none.
func prefix(noLazy bool) []string {
	p := []string{"--no-replace-objects", "-c", "core.commitGraph=false", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "advice.graftFileDeprecated=false"}
	if noLazy {
		p = append([]string{"--no-lazy-fetch"}, p...)
	}
	return p
}

// valueKind is the kind of a value a slot takes.
type valueKind int

const (
	kindRev valueKind = iota + 1
	kindOID
	kindPath
	kindTag
	kindURL
	kindHelper
	kindSSH
)

// value is a caller's or git's value for a slot, made only by parseRev or
// parseOID, and checked again by run at the slot.
type value struct {
	kind valueKind
	text string
}

// errSlots is a run whose values do not fit its verb's slots.
var errSlots = errors.New("the values do not fit the verb's slots")

// errNoBand is a run of a verb that reads objects before the git's band
// allows it (history-version-bands).
var errNoBand = errors.New("a verb that reads objects runs only on a git whose version allows it")

// argv is the verb's args with each slot filled, in order, by vals.
func (v verb) argv(vals []value) ([]string, error) {
	out := make([]string, 0, len(v.args))
	next := 0
	table := slots()
	for _, a := range v.args {
		pre, name, isSlot := strings.Cut(a, "\x00")
		if !isSlot {
			out = append(out, a)
			continue
		}
		s, known := table[name]
		if !known || next >= len(vals) || vals[next].kind != s.kind || !vals[next].ok() {
			return nil, errSlots
		}
		out = append(out, pre+s.render(vals[next].text))
		next++
	}
	if next != len(vals) {
		return nil, errSlots
	}
	return out, nil
}

// ok checks the value again on its kind's rules.
func (v value) ok() bool {
	switch v.kind {
	case kindRev:
		_, err := parseRev(v.text)
		return err == nil
	case kindOID:
		_, err := parseOID(v.text)
		return err == nil
	case kindPath:
		_, err := parsePath(v.text)
		return err == nil
	case kindTag:
		_, err := parseTag(v.text)
		return err == nil
	case kindURL:
		_, err := parseURL(v.text)
		return err == nil
	case kindHelper:
		return helperOK(v.text)
	case kindSSH:
		return sshOK(v.text)
	}
	return false
}

// env is the whole environment of a run, go-proc's gitenv.Env over the
// system's part alone (proc.SystemEnv): the machine-wide names a program on
// this system needs to start or to find the system's own files, which go-proc
// names and gragas never does, so the ssh git starts finds its own
// configuration. Nothing else of this process's is inherited,
// so a hook's GIT_DIR or GIT_INDEX_FILE, in any case, a GIT_CONFIG_*
// override or a PATH never reaches git; GIT_CONFIG_NOSYSTEM keeps the
// machine-wide config out, as it names no person, and GIT_TERMINAL_PROMPT
// keeps git from waiting on one.
// GIT_GRAFT_FILE names the empty file for every verb, so the repository's
// info/grafts, which --no-replace-objects leaves on, never gives a commit
// other parents and a range or a base other commits. The home is the
// account's, go-proc's gitenv.Home of Config.Home, so its own config is
// read, except for a verb that reads none of the account's (noAccount):
// GIT_CONFIG_GLOBAL is the null device for it, and a verb with no home
// (noHome) is given gitenv.NoHome of the run's own empty folder, home, so
// curl reads no .netrc either. go-proc names the variable, on every system.
// A verb whose ssh may ask the agent is handed the adapter's
// agent socket, when it was given one, and a verb that runs on the run's own
// index is handed it.
func (g *Git) env(v verb, in input) ([]string, error) {
	add := []string{"GIT_GRAFT_FILE=" + os.DevNull, "LC_ALL=C"}
	var home []string
	var err error
	switch {
	case v.noHome:
		home, err = gitenv.NoHome(in.home.text)
	case g.cfg.Home != "":
		home, err = gitenv.Home(filepath.FromSlash(g.cfg.Home))
	}
	if err != nil {
		return nil, fmt.Errorf("git's home: %w", err)
	}
	add = append(add, home...)
	if v.agent && g.cfg.Agent != "" {
		add = append(add, "SSH_AUTH_SOCK="+g.cfg.Agent)
	}
	if v.index {
		add = append(add, "GIT_INDEX_FILE="+in.index.text)
	}
	env := gitenv.Env(proc.SystemEnv(), add...)
	// NoHome sets GIT_CONFIG_GLOBAL itself. A verb that keeps its home and reads none of the
	// account's config is given go-proc's NoGlobalConfig, which goes after Env's result, as
	// go-proc's contract says: Env drops every GIT_ name of the base environment it is given
	// and puts the caller's add last, and this add comes after both.
	if v.noAccount && !v.noHome {
		env = append(env, gitenv.NoGlobalConfig()...)
	}
	return env, nil
}

// testCA is the CA file a test of this package hands an https fetch, so it
// can fetch from its own loopback server: never set outside a test binary,
// and read only in one. Every other run checks the server's certificate
// against the system's own CAs.
func (g *Git) testCA() string {
	if g.ca != "" && testing.Testing() {
		return g.ca
	}
	return ""
}

// errUnknownVerb is a run of a verb the table does not hold.
var errUnknownVerb = errors.New("the verb is not in git's table")

// exited is a run that ended with a code other than 0.
type exited struct {
	code   int
	stderr []byte
}

func (e *exited) Error() string {
	// %q: git's own words go to logs and JSON escaped, never raw
	return fmt.Sprintf("git exited %d: %q", e.code, bytes.TrimSpace(e.stderr))
}

// errPastCap is a run that printed more than its cap.
var errPastCap = errors.New("git printed more than its cap")

// errDeadline is a run that did not end before the deadline.
var errDeadline = errors.New("git did not end before the deadline")

// errEscaped is a run a process outside git's tree still held the output
// of after the tree was ended: one that left the group with setsid.
var errEscaped = errors.New("git ended, and a process outside its tree still holds its output")

// run is the one place git is started, through go-proc's proc.Run: verb
// from the table, its slots filled by vals, in dir, at the checked path,
// with the prefix, the bare environment, the verb's cap and the deadline,
// in a tree of its own that is killed whole when the run ends, so an ssh or
// a helper git started ends with it. Every run that starts is recorded in
// g.ran.
func (g *Git) run(ctx context.Context, name, dir string, vals ...value) ([]byte, error) {
	return g.runWith(ctx, name, dir, input{}, vals...)
}

// input is what a run that writes is handed besides its slots: the bytes
// on its standard input, the run's own index, an absolute path, the empty
// folder it takes as its hooks, another, and the empty folder a verb with
// no home takes as its home, a third.
type input struct {
	stdin              []byte
	index, hooks, home value
}

// runWith is run with in, which must fit the verb: bytes only for a verb
// that reads stdin, an index exactly for a verb that runs on one, a hooks
// folder exactly for a verb that writes, and a home folder for a verb with
// no home, which a verb that writes may also be handed and leaves unread.
func (g *Git) runWith(ctx context.Context, name, dir string, in input, vals ...value) ([]byte, error) {
	v, err := g.fits(name, in)
	if err != nil {
		return nil, err
	}
	rest, err := v.argv(vals)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", err, name)
	}
	path, err := g.program(ctx)
	if err != nil {
		return nil, err
	}
	wait := g.wait
	if v.long {
		wait *= 12
	}
	var own []string
	if v.writes {
		own = append(own, "-c", "core.hooksPath="+in.hooks.text)
	}
	if ca := g.testCA(); v.tls && ca != "" {
		own = append(own, "-c", "http.sslCAInfo="+ca)
	}
	args := slices.Concat(prefix(v.objects && g.band == bandNoLazy), own, rest)
	env, err := g.env(v, in)
	if err != nil {
		return nil, err
	}
	c := proc.Cmd{Path: path, Args: args, Dir: dir, Env: env, Timeout: wait, MaxOutput: v.max, Drain: drain}
	if v.stdin {
		c.Stdin = bytes.NewReader(in.stdin)
	}
	// gosec's G204 sees no exec here; proc.Run starts the checked setting's
	// path with a fixed row of the table.
	res, err := proc.Run(ctx, c)
	g.ran = append(g.ran, Ran{Verb: name, Argv: append([]string{path}, args...), Env: c.Env, Dir: dir, Code: res.Code})
	return result(ctx, wait, res, err)
}

// fits answers the verb name names, when in fits it: bytes only for a verb
// that reads stdin, an index exactly for a verb that runs on one, a hooks
// folder exactly for a verb that writes, a home folder for a verb with no
// home and for none that does not write, and a verb that reads objects only
// in a band that says how.
func (g *Git) fits(name string, in input) (verb, error) {
	v, ok := verbs()[name]
	if !ok {
		return verb{}, fmt.Errorf("%w: %q", errUnknownVerb, name)
	}
	isPath := func(p value) bool { return p.kind == kindPath && p.ok() }
	home := isPath(in.home)
	if (in.stdin != nil && !v.stdin) || v.index != isPath(in.index) || v.writes != isPath(in.hooks) ||
		(v.noHome && !home) || (home && !v.writes) {
		return verb{}, fmt.Errorf("%w: %q", errSlots, name)
	}
	if v.objects && g.band != bandPartial && g.band != bandNoLazy {
		return verb{}, fmt.Errorf("%w: %q", errNoBand, name)
	}
	return v, nil
}

// result is what a run that ended with err answers: an ended context, the
// deadline, output past a cap, a process outside the tree that held the
// output, git's own exit, or its output.
func result(ctx context.Context, wait time.Duration, res proc.Result, err error) ([]byte, error) {
	ended := func(why string) bool { return errors.Is(err, proc.Ended{Why: why}) }
	switch {
	case ended(proc.WhyCancelled) && ctx.Err() != nil:
		return nil, ctx.Err()
	case ended(proc.WhyDeadline):
		return nil, fmt.Errorf("%w (%s)", errDeadline, wait)
	case ended(proc.WhyCap) || len(res.Stderr) > maxStderr:
		return nil, errPastCap
	case ended(proc.WhyEscaped):
		return nil, fmt.Errorf("%w: %w", errEscaped, err)
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return nil, &exited{code: ee.ExitCode(), stderr: res.Stderr}
	}
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, &exited{code: res.Code, stderr: res.Stderr}
	}
	return res.Stdout, nil
}
