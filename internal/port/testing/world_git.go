package porttest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Rev is one revision of a seeded repository: the commit it names and the
// files of its tree.
type Rev struct {
	Commit string
	Files  map[string]string
	// Links are the symbolic links of the tree, by path, each with the
	// path it names: Files lists them with Link set, and Blob never reads
	// one.
	Links map[string]string
}

// Repo is one seeded repository's history. A test seeds exactly the trees it
// reads; the fake never derives one.
type Repo struct {
	// Tags are the tags, by name.
	Tags map[string]Rev
	// Heads are the branches, by name: a rev as a tag is, and never a tag.
	Heads map[string]Rev
	// Index is the files of the index, the rev "", and IndexLinks its
	// symbolic links, as a Rev's Links are.
	Index, IndexLinks map[string]string
	// Who is the configured identity; nil is none configured.
	Who *model.Identity
	// Bases are the merge bases the test seeded, by the pair of revs in
	// either order, as the full object name; "" is histories with no commit
	// in common.
	Bases map[[2]string]string
	// Revs are the commits that are neither a tag nor a branch, such as the
	// commits a branch holds below its head, found by Commit.
	Revs []Rev
	// Parents are each commit's parents, in its order, by commit; Commits
	// walks them, and a commit it meets that is not here is unseeded.
	Parents map[string][]string
	// Merging are the commits a merge in progress brings in; nil is none.
	Merging []string
	// Logs are the messages of each range the test seeded, by its base and
	// head, each a full object name, in that order, each as its object
	// stores it.
	Logs map[[2]string][]model.Message
	// Encodings are the encodings the commits' headers name, by full object
	// name; a commit not here names none.
	Encodings map[string]string
}

// SeedRepo makes root a repository with the history r.
func (w *World) SeedRepo(root model.Root, r Repo) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.repos[root] = r
}

type historyPort struct{ w *World }

// repoOf is the seeded repository of root, or fails the test for call.
func (w *World) repoOf(call string, root model.Root) (Repo, error) {
	if r, ok := w.repos[root]; ok {
		return r, nil
	}
	return Repo{}, w.unseeded(call, "the root %q holds no seeded history; call SeedRepo", root)
}

// revFiles is the files at rev: a tag or branch name, a commit, or "" for
// the index.
func (r Repo) revFiles(rev string) (map[string]string, bool) {
	at, ok := r.rev(rev)
	return at.Files, ok
}

// rev is the tree at rev: a tag or branch name, a commit, or "" for the
// index.
func (r Repo) rev(rev string) (Rev, bool) {
	if rev == "" {
		return Rev{Files: r.Index, Links: r.IndexLinks}, true
	}
	if t, ok := r.Tags[rev]; ok {
		return t, true
	}
	if h, ok := r.Heads[rev]; ok {
		return h, true
	}
	for _, revs := range []map[string]Rev{r.Tags, r.Heads} {
		for _, t := range revs {
			if t.Commit == rev {
				return t, true
			}
		}
	}
	for _, t := range r.Revs {
		if t.Commit == rev {
			return t, true
		}
	}
	return Rev{}, false
}

// commitOf is the commit rev names: a tag's or a branch's, or rev itself
// where a seeded commit is it.
func (r Repo) commitOf(rev string) (string, bool) {
	if t, ok := r.Tags[rev]; ok {
		return t.Commit, true
	}
	if h, ok := r.Heads[rev]; ok {
		return h.Commit, true
	}
	if _, ok := r.Parents[rev]; ok {
		return rev, true
	}
	return "", false
}

func (x historyPort) Merging(_ context.Context, root model.Root) ([]string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Merging", root)
	if err != nil {
		return nil, err
	}
	return append([]string{}, r.Merging...), nil
}

// Commits walks the seeded parents from to, leaving out every commit from
// reaches, and answers each commit after its parents, a commit's parents in
// their order before it.
func (x historyPort) Commits(_ context.Context, root model.Root, from, to string) ([]model.Commit, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Commits", root)
	if err != nil {
		return nil, err
	}
	ends := [2]string{}
	for i, rev := range []string{from, to} {
		c, ok := r.commitOf(rev)
		if !ok {
			return nil, fmt.Errorf("the revision %q does not exist: %w", rev, port.ErrNoRev)
		}
		ends[i] = c
	}
	reached := map[string]bool{}
	if c, ok := r.walk(ends[0], reached, func(model.Commit) {}); !ok {
		return nil, x.w.unseeded("History.Commits", "no parents of %q were seeded", c)
	}
	out := []model.Commit{}
	if c, ok := r.walk(ends[1], reached, func(m model.Commit) { out = append(out, m) }); !ok {
		return nil, x.w.unseeded("History.Commits", "no parents of %q were seeded", c)
	}
	return out, nil
}

// walk visits c and every seeded parent it reaches that reached does not hold
// yet, marking each, and hands each to emit after its parents, in their
// order. It answers false and the commit whose parents were not seeded.
func (r Repo) walk(c string, reached map[string]bool, emit func(model.Commit)) (string, bool) {
	if reached[c] {
		return "", true
	}
	parents, ok := r.Parents[c]
	if !ok {
		return c, false
	}
	reached[c] = true
	for _, p := range parents {
		if bad, ok := r.walk(p, reached, emit); !ok {
			return bad, false
		}
	}
	emit(model.Commit{ID: c, Parents: slices.Clone(parents)})
	return "", true
}

func (x historyPort) Base(_ context.Context, root model.Root, a, b string) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Base", root)
	if err != nil {
		return "", err
	}
	for k, v := range r.Bases {
		if k == [2]string{a, b} || k == [2]string{b, a} {
			if v == "" {
				return "", fmt.Errorf("%q and %q have no commit in common: %w", a, b, port.ErrNoBase)
			}
			return v, nil
		}
	}
	return "", x.w.unseeded("History.Base", "no merge base of %q and %q was seeded", a, b)
}

func (x historyPort) Files(_ context.Context, root model.Root, rev string) ([]model.Entry, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Files", root)
	if err != nil {
		return nil, err
	}
	at, ok := r.rev(rev)
	if !ok {
		return nil, fmt.Errorf("the revision %q does not exist: %w", rev, port.ErrNoRev)
	}
	out := make([]model.Entry, 0, len(at.Files)+len(at.Links))
	for p, c := range at.Files {
		out = append(out, model.Entry{Path: p, Size: int64(len(c)), Object: objectOf(c)})
	}
	for p, to := range at.Links {
		out = append(out, model.Entry{Path: p, Size: int64(len(to)), Link: true, Object: objectOf(to)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// WorkObjects answers the Object of each of paths as the world's tree at
// root holds it now, the tree Seed made and the Writer changed: "" for a
// path that is not a file of it, one at or below a link, one that is not a
// path a root may name, and one that begins with a double quote, as the
// adapter answers them. A root never seeded is unseeded.
func (x historyPort) WorkObjects(_ context.Context, root model.Root, paths []string) ([]string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("History.WorkObjects", root)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		if p == "" || inside(p) != nil || strings.HasPrefix(p, `"`) || t.linked("work", p) != nil {
			continue
		}
		if c, ok := t.files[p]; ok {
			out[i] = objectOf(c)
		}
	}
	return out, nil
}

// objectOf is the fake's Object of content: the same for the same bytes and
// another for others, and never git's own name, so no test leans on how git
// makes one.
func objectOf(content string) string {
	return digestOf("blob", content)
}

func (x historyPort) Blob(_ context.Context, root model.Root, rev, p string) ([]byte, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Blob", root)
	if err != nil {
		return nil, err
	}
	files, ok := r.revFiles(rev)
	if !ok {
		return nil, fmt.Errorf("the revision %q does not exist: %w", rev, port.ErrNoRev)
	}
	if c, ok := files[p]; ok {
		return []byte(c), nil
	}
	return nil, notExist("blob", p)
}

func (x historyPort) Blobs(_ context.Context, root model.Root, rev string, paths []string) ([][]byte, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Blobs", root)
	if err != nil {
		return nil, err
	}
	files, ok := r.revFiles(rev)
	if !ok {
		return nil, fmt.Errorf("the revision %q does not exist: %w", rev, port.ErrNoRev)
	}
	out := make([][]byte, 0, len(paths))
	for _, p := range paths {
		c, ok := files[p]
		if !ok {
			return nil, notExist("blob", p)
		}
		out = append(out, []byte(c))
	}
	return out, nil
}

func (x historyPort) Tags(_ context.Context, root model.Root) ([]model.Tag, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Tags", root)
	if err != nil {
		return nil, err
	}
	out := make([]model.Tag, 0, len(r.Tags))
	for n, t := range r.Tags {
		out = append(out, model.Tag{Name: n, Commit: t.Commit})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Messages answers the seeded messages of base..head, sorted by commit. A
// base or head that is not a full object name, or names no commit the
// repository was seeded with, is refused, as the adapter refuses it; so is
// a range with a message holding a NUL, or under a header that names an
// encoding other than UTF-8, naming the commit.
func (x historyPort) Messages(_ context.Context, root model.Root, base, head string) ([]model.Message, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Messages", root)
	if err != nil {
		return nil, err
	}
	for _, end := range []string{base, head} {
		if !r.isCommit(end) {
			return nil, fmt.Errorf("an end of a range is the full object name of a commit of the repository: %q is not", end)
		}
	}
	msgs, ok := r.Logs[[2]string{base, head}]
	if !ok {
		return nil, x.w.unseeded("History.Messages", "no messages of %s..%s were seeded", base, head)
	}
	for _, m := range msgs {
		if enc := r.Encodings[m.Commit]; enc != "" && !strings.EqualFold(enc, "utf-8") && !strings.EqualFold(enc, "utf8") {
			return nil, fmt.Errorf("a commit's message cannot be read whole: the commit %s: its header names the encoding %q", m.Commit, enc)
		}
		if strings.ContainsRune(m.Text, 0) {
			return nil, fmt.Errorf("a commit's message cannot be read whole: the commit %s: its message holds a NUL", m.Commit)
		}
	}
	out := slices.Clone(msgs)
	slices.SortFunc(out, func(a, b model.Message) int { return strings.Compare(a.Commit, b.Commit) })
	if out == nil {
		out = []model.Message{}
	}
	return out, nil
}

// isCommit is whether name is the full object name of a commit the
// repository was seeded with: a tag's or a branch's, or an end of a seeded
// range.
func (r Repo) isCommit(name string) bool {
	if len(name) != 40 && len(name) != 64 || strings.Trim(name, "0123456789abcdef") != "" {
		return false
	}
	for _, revs := range []map[string]Rev{r.Tags, r.Heads} {
		for _, t := range revs {
			if t.Commit == name {
				return true
			}
		}
	}
	for k := range r.Logs {
		if k[0] == name || k[1] == name {
			return true
		}
	}
	return false
}

func (x historyPort) Who(_ context.Context, root model.Root) (model.Identity, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r, err := x.w.repoOf("History.Who", root)
	if err != nil {
		return model.Identity{}, err
	}
	if r.Who == nil {
		return model.Identity{}, errors.New("the repository has no identity configured")
	}
	return *r.Who, nil
}

// Commit is one commit the Remote made.
type Commit struct {
	Root    model.Root
	ID      string
	Paths   []string
	Message string
}

// TagMade is one tag the Remote made.
type TagMade struct {
	Root                  model.Root
	Name, Commit, Message string
}

// commitLog is the content last committed in a root, for "no change".
type commitLog struct {
	last map[string]string
	head string
}

// SeedRelease makes the tag of the source from fetchable, holding files.
func (w *World) SeedRelease(from, tag string, files map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.releases[from] == nil {
		w.releases[from] = map[string]map[string]string{}
	}
	w.releases[from][tag] = maps.Clone(files)
}

// digestOf is a short stable name for parts, so nothing the fake makes is a
// constant.
func digestOf(parts ...string) string {
	h := digest.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return digest.HexOf(h)
}

type remotePort struct{ w *World }

func (x remotePort) Fetch(_ context.Context, from, tag string) (model.Root, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if from == "" || tag == "" {
		return "", errors.New("a fetch names a source and a tag")
	}
	tags, ok := x.w.releases[from]
	if !ok {
		return "", x.w.unseeded("Remote.Fetch", "the source %q has no seeded release; call SeedRelease", from)
	}
	files, ok := tags[tag]
	if !ok {
		return "", fmt.Errorf("the tag %q is not in %q", tag, from)
	}
	return x.w.seed("/cache/releases/"+digestOf(from, tag)[:16], files), nil
}

func (x remotePort) Commit(_ context.Context, root model.Root, paths []string, message string) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Remote.Commit", root)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 || message == "" {
		return "", errors.New("a commit names paths and has a message")
	}
	log := x.w.commits[root]
	if log == nil {
		log = &commitLog{last: map[string]string{}}
		x.w.commits[root] = log
	}
	changed := false
	parts := []string{string(root), log.head, message}
	for _, p := range paths {
		if err := inside(p); err != nil || p == "" {
			return "", firstErr(err, errors.New("a path is named"))
		}
		c, ok := t.files[p]
		if !ok {
			return "", notExist("commit", p)
		}
		prev, had := log.last[p]
		changed = changed || !had || prev != c
		parts = append(parts, p, c)
	}
	if !changed {
		return "", errors.New("nothing to commit: the paths hold what was last committed")
	}
	id := digestOf(parts...)[:40]
	for _, p := range paths {
		log.last[p] = t.files[p]
	}
	log.head = id
	x.w.Commits = append(x.w.Commits, Commit{Root: root, ID: id, Paths: slices.Clone(paths), Message: message})
	return id, nil
}

func (x remotePort) Tag(_ context.Context, root model.Root, name, commit, message string) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Remote.Tag", root); err != nil {
		return err
	}
	if message == "" || name == "" || commit == "" {
		return errors.New("a tag needs a name, a commit and a message")
	}
	for _, m := range x.w.Tags {
		if m.Root == root && m.Name == name {
			return fmt.Errorf("the tag %q exists", name)
		}
	}
	x.w.Tags = append(x.w.Tags, TagMade{Root: root, Name: name, Commit: commit, Message: message})
	return nil
}

// SeedTool makes the tool locatable, answering found: the Steps fake finds
// a grant's program by its id here.
func (w *World) SeedTool(tool string, found model.Found) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tools[tool] = found
}

// SandboxReaches makes a Prepare that asks for level reach only reached, a
// lower level, as a platform that has no more would.
func (w *World) SandboxReaches(asked, reached model.Level) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reach[asked] = reached
}

// SandboxMissing makes a Prepare that asks for level answer ErrUnavailable.
func (w *World) SandboxMissing(level model.Level) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.missing[level] = true
}

type sandboxPort struct{ w *World }

func (x sandboxPort) Prepare(_ context.Context, p model.Prepare) (model.Scratch, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if p.Level != model.Isolated && p.Level != model.Sandboxed {
		return model.Scratch{}, fmt.Errorf("the level %q is neither isolated nor sandboxed", p.Level)
	}
	if x.w.missing[p.Level] {
		return model.Scratch{}, fmt.Errorf("no %s level here: %w", p.Level, port.ErrUnavailable)
	}
	if p.Seconds < 0 || p.MemoryMiB < 0 {
		return model.Scratch{}, fmt.Errorf("a limit of %d seconds or %d MiB is below none", p.Seconds, p.MemoryMiB)
	}
	files := map[string]string{}
	for _, f := range append(slices.Clone(p.Fixtures), p.Config...) {
		if err := inside(f.Path); err != nil || f.Path == "" {
			return model.Scratch{}, firstErr(err, errors.New("a file is named by a path"))
		}
		if err := placeable(files, f.Path); err != nil {
			return model.Scratch{}, err
		}
		files[f.Path] = string(f.Data)
	}
	if p.Resolve != (model.Link{}) {
		if err := inside(p.Resolve.Path); err != nil || p.Resolve.Path == "" || !path.IsAbs(p.Resolve.Dir) {
			return model.Scratch{}, firstErr(err, errors.New("a link is placed at a path, to an absolute folder"))
		}
		if err := placeable(files, p.Resolve.Path); err != nil {
			return model.Scratch{}, err
		}
	}
	level := p.Level
	if r, ok := x.w.reach[p.Level]; ok {
		level = r
	}
	x.w.scratch++
	dir := ScratchDir(x.w.scratch)
	x.w.seed(dir, files)
	x.w.Prepared = append(x.w.Prepared, p)
	return model.Scratch{Dir: dir, Level: level, Seconds: bound(p.Seconds, FakeSeconds), MemoryMiB: p.MemoryMiB}, nil
}

// scratchRoot is the folder the fake's scratch folders are made in:
// slash-separated, and absolute in this system's form, as the adapter's are
// (C:/scratch where a path names a volume, /scratch where it does not), so the
// contract's check of an absolute folder holds of the fake on every system.
// Abs fails only where the working folder cannot be read; the slash form is
// then kept, and the contract's check says so.
func scratchRoot() string {
	abs, err := filepath.Abs(filepath.FromSlash("/scratch"))
	if err != nil {
		return "/scratch"
	}
	return filepath.ToSlash(abs)
}

// ScratchDir is the folder the fake sandbox's n-th Prepare makes, counted
// from 1, for a test that scripts a tool by the folder it runs in.
func ScratchDir(n int) string { return fmt.Sprintf("%s/%d", scratchRoot(), n) }

// FakeSeconds is the fake sandbox's own deadline, for a Prepare that asks
// for 0. Like the adapter, it has no memory limit of its own: a Prepare that
// asks for 0 MiB is held to none.
const FakeSeconds = 60

// bound is v, or b for a v of 0.
func bound(v, b int) int {
	if v == 0 {
		return b
	}
	return v
}

// placeable refuses p where files already holds it, a file above it, or a
// file below it: one place holds one file.
func placeable(files map[string]string, p string) error {
	for f := range files {
		if f == p || strings.HasPrefix(f, p+"/") || strings.HasPrefix(p, f+"/") {
			return fmt.Errorf("two files of the run are placed at %q and %q", f, p)
		}
	}
	return nil
}

func (x sandboxPort) Release(s model.Scratch) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if s == (model.Scratch{}) || !absolute(s.Dir) {
		return fmt.Errorf("release: %q is no scratch folder", s.Dir)
	}
	delete(x.w.roots, model.Root(s.Dir))
	return nil
}

// Sweep removes every folder Prepare made and Release has not, as the adapter
// removes every scratch folder no run holds, and answers how many.
func (x sandboxPort) Sweep() (int, error) { return x.left(true), nil }

// Left counts the folders Sweep would remove, and removes none.
func (x sandboxPort) Left() (int, error) { return x.left(false), nil }

func (x sandboxPort) left(remove bool) int {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	n := 0
	for r := range x.w.roots {
		if path.Dir(string(r)) == scratchRoot() {
			if remove {
				delete(x.w.roots, r)
			}
			n++
		}
	}
	return n
}

// Scratch is every file in the folder a Sandbox.Prepare answered, sorted,
// and whether the folder is there: false once it was released.
func (w *World) Scratch(s model.Scratch) ([]string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.roots[model.Root(s.Dir)]
	if !ok {
		return nil, false
	}
	return slices.Sorted(maps.Keys(t.files)), true
}
