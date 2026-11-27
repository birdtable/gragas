package tool

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-files/confine"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Step names where a tool was looked for, in the order they are tried.
type Step string

// The steps of a grant's find (step_grant.go).
const (
	// StepSettings is the path the settings' tool-paths give the tool at
	// the version for the project. The project state file's own entry joins
	// it in M6-02.
	StepSettings Step = "settings"
	// StepToolsFile is the path the per-machine tools file gives the tool
	// at the version.
	StepToolsFile Step = "tools-file"
)

// place is one path to look at, and the step that names it.
type place struct {
	step Step
	path string
	// script is the script grant of a place that is one, and
	// path is then its manifest; nil for any other place.
	script *plugin.ProgramScript
	// want is the version a grant's place must print, the tools file's key
	// it was chosen by; "" for a place held to the grant's range alone.
	want string
}

// reach is the program the place p names, resolved through the trust walk
// and judged, with the script it runs, ok; or, for a place that holds nothing
// and one a pinned find passes over, not ok, each named in nf. Anything the
// walk or the judgement refuses refuses the find, so an unsafe program never
// gives way to the next place unseen. A grant's find reaches
// each of its places so.
func (t *Tool) reach(root model.Root, p place, in within, pinned bool, nf *NotFound) (f model.Found, ok bool, err error) {
	target, script, err := t.resolve(root, p, in)
	if errors.Is(err, fs.ErrNotExist) {
		nf.Tried = append(nf.Tried, string(p.step)+" "+p.path+absent(p.path, err))
		return model.Found{}, false, nil
	}
	if err != nil {
		return model.Found{}, false, err
	}
	over, err := t.passedOver(pinned, target, root)
	if err != nil {
		return model.Found{}, false, err
	}
	if over {
		nf.Tried = append(nf.Tried, string(p.step)+" "+p.path+" (it is at or below the root, so a pinned find passes it over)")
		return model.Found{}, false, nil
	}
	if err := t.judged(target, script, in); err != nil {
		return model.Found{}, false, err
	}
	return model.Found{Path: target, Script: script}, true, nil
}

// absent is what a Tried line says after a place whose check answered fs.ErrNotExist: nothing for a
// place that is not there, and, for one that is (a place whose entry exists but whose walk or file
// check ended in a not-there answer, as a link or a name the system reads another way does), the
// answer itself, so a find that tried a place that is there never reads as one that is empty.
func absent(place string, err error) string {
	if _, statErr := os.Lstat(filepath.FromSlash(place)); statErr != nil {
		return ""
	}
	return " (the place is there, and the check of it answered: " + err.Error() + ")"
}

// judged is the check of the program a place names: a script's program is node,
// which findInterpreter has judged already, so only a program that is not one is read
// as an interpreter chain here.
func (t *Tool) judged(target, script string, in within) error {
	if script != "" {
		return nil
	}
	return t.interpreter(target, in)
}

// within is what a find's runs are given and held to: the PATH, and the
// root a pinned find holds the program env starts to, "" for an unpinned
// find, which holds none.
type within struct {
	path string
	root model.Root
}

// within is the PATH and root a find for root, pinned or not, runs with.
func (t *Tool) within(root model.Root, pinned bool) within {
	if !pinned {
		return within{path: t.cfg.Path}
	}
	return within{path: t.pinnedPath(root), root: root}
}

// empty is whether w is a pinned find's and its PATH keeps no folder:
// nothing is started with it, since a run given no PATH, or
// PATH= empty, reads the folder it runs in.
func (w within) empty() bool { return w.root != "" && w.path == "" }

// pinnedPath is the PATH a pinned find's runs are given, and a run of what a
// pinned find stored: the PATH of the config with every folder left out that is relative,
// has a .. segment, lies, links followed, at or below root, cannot be
// compared with root, or holds the list separator or a double quote. A package runner started in the
// project puts its own folder first as a matter of course, so the folder is
// left out rather than the tool refused. A program the pinned tool starts by
// name, an interpreter env finds or a program a native tool starts, is then
// never one the project holds. A folder is absolute by filepath.IsAbs and its
// volume, as pathFault judges: on Windows C:\Go\bin is kept in
// the form the PATH gives it, compared with root in its slash form, and
// \bin or C:bin is left out as relative.
func (t *Tool) pinnedPath(root model.Root) string {
	var kept []string
	// Split at every list separator and keep each part as written: filepath.SplitList
	// on Windows reads a quoted part as one and strips its quotes, so a folder the
	// PATH wrote with a quote would reach pathFolder without one and be kept.
	for dir := range strings.SplitSeq(t.cfg.Path, string(filepath.ListSeparator)) {
		slash, ok := pathFolder(dir)
		if !ok {
			continue
		}
		if in, err := t.folderInRoot(slash, root, "the folder "+hosttrust.Quote(dir)+" of the PATH a run is given"); err != nil || in {
			continue
		}
		kept = append(kept, dir)
	}
	return strings.Join(kept, string(filepath.ListSeparator))
}

// pathFolder is a folder of the PATH in the slash form a root is compared
// in, cleaned, and whether a pinned find may keep it: absolute by
// filepath.IsAbs and its volume, with no .. segment, and no list separator
// or double quote in its name. The separator is read as two
// folders once joined back, the second of them relative to the folder the
// run is in. A quote is read as part of one folder on one system and kept as
// a letter on another, so the folder is left out on every system.
func pathFolder(dir string) (string, bool) {
	if strings.ContainsRune(dir, filepath.ListSeparator) || strings.ContainsRune(dir, '"') || !filepath.IsAbs(dir) || slices.Contains(strings.Split(filepath.ToSlash(dir), "/"), "..") {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(dir)), true
}

// emptyPinnedPath is why a pinned find, or a run of what one answered, has
// no PATH to give its runs: pinnedPath left no folder of
// pathEnv. A program started with no PATH reads its own default, which can
// hold the folder it runs in (bash's ends in "."), and PATH= empty is that
// folder too, so nothing is started; gragas writes no list of its own. The
// user's PATH is the only source.
func emptyPinnedPath(pathEnv string) string {
	return "the PATH gragas runs with, " + hosttrust.Quote(pathEnv) + ", keeps no folder outside the project once each folder that is relative, has a .. segment, lies at or below the root, cannot be compared with it, or holds the list separator or a double quote is left out, and a program started with no PATH reads a default that may hold the folder it runs in"
}

// emptyPinnedPathFix is the line that fixes emptyPinnedPath.
const emptyPinnedPathFix = "put an absolute folder outside the project, one this account can read, on the PATH gragas runs with"

// folderInRoot is whether the folder dir lies at or below root, by spelling
// or, links followed, as a folder (under). A dir that is not there is
// judged by the nearest folder above it that is, since a program could be
// made there; a name on the way that cannot be resolved refuses the find.
// what names dir in a refusal.
func (t *Tool) folderInRoot(dir string, root model.Root, what string) (bool, error) {
	r := string(root)
	if dir == r || strings.HasPrefix(dir, strings.TrimSuffix(r, "/")+"/") {
		return true, nil
	}
	for _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) && path.Dir(dir) != dir; _, err = os.Lstat(dir) {
		dir = path.Dir(dir)
	}
	return under(dir, root, what, true, t.cfg.Mounts)
}

// plainVersion is whether v is major.minor.patch, the form a tool's version
// is read in, so a version of another form could never be found.
func plainVersion(v string) bool {
	m := versionRE.FindStringSubmatch(v)
	return m != nil && m[1] == v
}

// versionRE finds the first major.minor.patch on a line, with a v before it
// or not, starting a word of its own. What follows the patch is not part of
// the match: readVersion judges it, so a word that goes on with a dot and a
// digit (1.2.3.4) is more after the patch and is refused, not skipped for a
// later version on the line.
var versionRE = regexp.MustCompile(`(?:^|[^0-9A-Za-z.])v?((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))`)

// versionAsk is what versionBy asks a program found at a place: the name it
// is known by, want for a refusal, the argv that asks its version, the
// pattern its version is read by and env, the PATH it is given.
type versionAsk struct {
	name, want string
	argv       []string
	pattern    *regexp.Regexp
	env        string
}

// versionBy asks the program found at a place its version, by the argv q
// gives, and reads the first major.minor.patch of the first line it prints. It
// runs in an empty folder of its own, made for it and removed after, never in
// root: nothing the project holds is in the folder a tool reads when asked its
// version. A folder that is not absolute, or that lies at or
// below root, is refused before anything starts (versionFolder). With no Home
// in the config it is given one of its own, as a step is (runHome), kept apart
// from root and its folder; a Home is judged against root either way, and
// q.env is the PATH it is given. pattern is the
// version_pattern of a grant, nil for the first
// major.minor.patch of the first line: with one, the version is what its one group holds at its first
// match in all the program prints, and it must be a release major.minor.patch.
// want names what was asked for, in a refusal.
func (t *Tool) versionBy(ctx context.Context, root model.Root, at model.Found, q versionAsk) (got string, err error) {
	name, want, argv, pattern, env := q.name, q.want, q.argv, q.pattern, q.env
	path, script := at.Path, at.Script
	if script != "" {
		argv = append([]string{script}, argv...)
	}
	// the folder is made fresh and private by go-files' confine.MkdirFresh in
	// the system's temp folder held as a root, and removed through it
	top, err := os.OpenRoot(os.TempDir())
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, top.Close()) }()
	made, fresh, err := confine.MkdirFresh(top, "gragas-version-")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, confine.RemoveAll(top, fresh)) }()
	if err := made.Close(); err != nil {
		return "", err
	}
	dir := filepath.Join(top.Name(), fresh)
	if err := t.versionFolder(dir, root); err != nil {
		return "", err
	}
	l := launch{name: name, shape: "version", path: path, argv: argv, dir: filepath.ToSlash(dir), root: string(root), env: env, wait: cmp.Or(t.cfg.VersionWait, versionDeadline)}
	if t.cfg.Home == "" {
		// The probe is given a Home of its own, made for this run and removed
		// with it, as a step is: a program that needs one to start is found,
		// and a probe, a step and a plugin record see the one environment.
		home, end, herr := t.runHome("version probe", string(root), l.dir)
		if herr != nil {
			return "", herr
		}
		defer end()
		l.home = filepath.ToSlash(home)
	}
	out, err := t.start(ctx, l)
	if err != nil {
		return "", err
	}
	if out.Code != 0 {
		return "", refused(name+" at "+path+" exits "+itoa(out.Code)+" when asked its version", runFix(t.cfg.Dialect, path, argv))
	}
	if pattern != nil {
		return patternVersion(pattern, out.Stdout, name, path, want)
	}
	first, _, _ := strings.Cut(string(out.Stdout), "\n")
	v, more, found := readVersion(first)
	switch {
	case !found:
		return "", refused(name+" at "+path+" prints no version major.minor.patch on its first line", "check that "+hosttrust.Quote(path)+" is "+name)
	case more != "":
		return "", refused(name+" at "+path+" prints the version "+hosttrust.Quote(more)+", which holds more after the patch (a prerelease, a local build or a fourth part), so it is not the release "+want+" that is asked for",
			"install the release of "+name+" the tested set pins, not a prerelease or a build of your own")
	}
	return v, nil
}

// patternVersion is the version a grant's version_pattern reads from what the
// program printed: its one group at the first match. No match, and a group
// that is not a release major.minor.patch (a prerelease, a build, a fourth
// part), are refused by name.
func patternVersion(pattern *regexp.Regexp, out []byte, name, path, want string) (string, error) {
	m := pattern.FindSubmatch(out)
	if m == nil {
		return "", refused(name+" at "+path+" prints nothing its grant's version_pattern "+hosttrust.Quote(pattern.String())+" matches", "check that "+hosttrust.Quote(path)+" is "+name)
	}
	v := string(m[1])
	if !plainVersion(v) {
		return "", refused(name+" at "+path+" prints the version "+hosttrust.Quote(v)+" where its grant's version_pattern reads one, which is no release major.minor.patch, so it is not "+want,
			"install a release of "+name+", not a prerelease or a build of your own")
	}
	return v, nil
}

// readVersion is the first major.minor.patch on line, found, and, when the
// version holds more after the patch (a prerelease as 1.2.3-rc.1, a build as
// 1.2.3+local or 1.2.3a1), that whole word as more: such a tool is not the
// release, so no caller may take it for 1.2.3.
func readVersion(line string) (v, more string, found bool) {
	loc := versionRE.FindStringSubmatchIndex(line)
	if loc == nil {
		return "", "", false
	}
	v = line[loc[2]:loc[3]]
	rest := line[loc[3]:]
	if !endsCleanly(rest) {
		more = line[loc[2]:]
		if i := strings.IndexAny(more, " \t\r,;)"); i >= 0 {
			more = more[:i]
		}
	}
	return v, more, true
}

// endsCleanly is whether rest, what follows a version's patch, ends it: the
// end of the line, a space, a tab or a return, a closing bracket or quote, a
// comma or a semicolon, or a full stop that ends the sentence. Anything else
// (a hyphen, a plus, a tilde, a letter of any alphabet, a dot and more) is more
// after the patch, so the version is not the release: the test is for what
// ends a version, not for each mark a prerelease may use.
func endsCleanly(rest string) bool {
	if rest == "" {
		return true
	}
	if rest[0] == '.' {
		return len(rest) == 1 || strings.ContainsRune(" \t\r", rune(rest[1]))
	}
	return strings.ContainsRune(" \t\r),;]\"'>", rune(rest[0]))
}

// versionFolder refuses the folder a version run was made in when it is not
// absolute, as a relative TMPDIR makes it, or lies, links followed, at or
// below root, as a TMPDIR in the project makes it: nothing the project holds
// is above the folder a tool reads.
func (t *Tool) versionFolder(dir string, root model.Root) error {
	d := filepath.ToSlash(dir)
	what := "the folder " + hosttrust.Quote(d) + " a version run is made in"
	if !filepath.IsAbs(dir) {
		return refused(what+" is not an absolute path, so it would be read from the folder gragas runs in", "set TMPDIR to an absolute folder outside the project")
	}
	in, err := t.folderInRoot(path.Clean(d), root, what)
	if err != nil {
		return err
	}
	if in {
		return refused(what+" lies at or below the root "+hosttrust.Quote(string(root)), "set TMPDIR to a folder outside the project")
	}
	return nil
}

// passedOver is whether a pinned find passes the program at target over, as
// lying at or below root (inRoot). An unpinned find passes nothing over.
func (t *Tool) passedOver(pinned bool, target string, root model.Root) (bool, error) {
	if !pinned {
		return false, nil
	}
	return t.inRoot(target, root)
}

// inRoot is whether the program at target, the file the trust walk resolved
// it to with every link followed, lies at or below root: by spelling, or as
// a folder, by device and inode, so a link, a case or a normalization the
// volume ignores that names the root another way names it all the same. A
// root or a folder on the way that cannot be read refuses the find: the
// check fails closed.
func (t *Tool) inRoot(target string, root model.Root) (bool, error) {
	r := string(root)
	if target == r || strings.HasPrefix(target, strings.TrimSuffix(r, "/")+"/") {
		return true, nil
	}
	return under(path.Dir(target), root, "the program "+hosttrust.Quote(target)+" a pinned find resolved", false, t.cfg.Mounts)
}

// under is whether the folder dir is root or a folder below it: go-files's
// walk of dir as a folder, Who{Apart: root}, which compares every name from
// the top of its volume down with root, read through its links, by its file
// ID, so a link, a case or a normalization the volume ignores that names the
// root another way names it all the same. With links the walk
// follows each link it finds, judging the link's owner and its own access
// list and then its target from the top again, as the program walk does;
// without, a link on the way is refused (Who.NoLinks), for a
// path the trust walk has already resolved. Each caller decides by it
// whether a folder the project may hold is read, so the walk's refusal of a
// name on the way, one another account can write or a link it does not
// trust, refuses the find in its words: no name below it can be trusted to
// say whether dir lies inside. what names dir in a refusal; a root or a
// folder on the way that cannot be read refuses the find too, so the check
// fails closed.
func under(dir string, root model.Root, what string, links bool, mounts []string) (bool, error) {
	r := filepath.FromSlash(string(root))
	s := hosttrust.Subject{Name: what, File: what, Last: hosttrust.Folder, Bare: true}
	_, err := hosttrust.Walk(s, filepath.FromSlash(dir), hosttrust.Who{NoLinks: !links, Apart: []string{r}, Mounts: mounts})
	if err == nil {
		return false, nil
	}
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		if u.Inside != "" {
			return true, nil
		}
		return false, wayRefused(what, root, u)
	}
	pe, ok := errors.AsType[*fs.PathError](err)
	switch {
	case ok && pe.Path == r:
		return false, refused("the root "+hosttrust.Quote(string(root))+" cannot be compared with "+what, "run in a project folder this system can read")
	case ok:
		return false, refused("the folder "+hosttrust.Quote(pe.Path)+" on the way of "+what+" cannot be compared with the root", "point the setting at a program in a folder this system can read")
	}
	return false, refused(what+" cannot be compared with the root "+hosttrust.Quote(string(root))+": "+err.Error(), "point the setting at a program in a folder this system can read")
}

// wayRefused is the refusal of what when the walk that compares it with
// root stops at a name on its way: one it does not trust, in its words, or
// one whose writers it cannot judge, said so, as a folder on a 9p share or a
// volume that ignores ownership is no folder another account was seen to
// write.
func wayRefused(what string, root model.Root, u *hosttrust.Unsafe) error {
	why, fix := "a folder on its way is not trusted", "use folders that root or this account alone can write, with no link on their way"
	if u.Unjudged {
		why, fix = "the walk cannot judge who may write a folder on its way", "use folders on a disk whose owners this system keeps and reads"
	}
	if u.Fix != "" {
		fix = u.Fix
	}
	return refused(what+" cannot be compared with the root "+hosttrust.Quote(string(root))+", as "+why+": "+u.Observed, fix)
}

// notAbsClean is the words every path that is not an absolute, clean,
// slash-separated path is refused with.
const notAbsClean = "is not an absolute, clean path"

// pathFault is why p is not an absolute, clean, slash-separated path, as the
// model's folders are, or "" when it is one. A path is absolute by
// filepath.IsAbs, never by a leading /: on Windows a rooted path with no
// volume, or a volume with no root, is no absolute path and gets the same
// words as any other.
func pathFault(p string) string {
	if !filepath.IsAbs(p) || filepath.ToSlash(filepath.Clean(p)) != p {
		return notAbsClean
	}
	return ""
}

// itoa is n in decimal.
func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// runFix is the fix of a version run that exited non-zero: the run's line, path
// and then argv, quoted by shell.Quote for d, to paste. With no dialect (Quote refuses the
// zero Dialect as unknown), or an argv Quote refuses (a word that does not show as itself, or one the dialect
// cannot pass), it is the step in words, each word in Go's quoting, so nothing
// in it is a line to paste and no rune of it reaches the terminal raw.
func runFix(d shell.Dialect, path string, argv []string) string {
	if s, err := shell.Quote(d, append([]string{path}, argv...)); err == nil {
		return "run " + s + " and fix what it says"
	}
	words := make([]string, 0, len(argv))
	for _, w := range argv {
		words = append(words, strconv.Quote(w))
	}
	if len(words) == 0 {
		return "run the program at " + strconv.Quote(path) + " with no words and fix what it says"
	}
	return "run the program at " + strconv.Quote(path) + " with the words " + strings.Join(words, ", ") + " and fix what it says"
}
