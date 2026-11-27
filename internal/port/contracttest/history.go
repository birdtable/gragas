package contracttest

import (
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// FixtureCommit is one commit of the History suite's repository.
type FixtureCommit struct {
	// Name is the commit's name in the fixture, its message too.
	Name string
	// Branch is the branch it is made on. A branch not yet made starts at
	// the commit named From, or as an orphan when From is "".
	Branch, From string
	// Files is the whole tree of the commit, by slash path.
	Files map[string]string
	// ID is the full object name git gives the commit, made with the
	// fixture's identity and dates (FixtureWho, FixtureDate).
	ID string
	// Merge is the Name of a second parent: the commit merges it into the
	// branch's head. "" is a commit of one parent, or none.
	Merge string
	// Message is the commit's whole message, kept byte for byte; "" is
	// Name and a newline.
	Message string
	// Encoding is the encoding the commit's header names, so the message's
	// bytes are stored as they are; "" names none, UTF-8, in which git
	// commit stores a byte that is not UTF-8 as the Latin-1 character.
	Encoding string
	// Raw writes the commit's object with Message as it is, past what git
	// commit checks: a NUL, or bytes that are not UTF-8 under no header
	// that names an encoding, as other tools write them.
	Raw bool
	// Links are the tree's symbolic links, by slash path, each with the
	// path it names.
	Links map[string]string
}

// Stored is the commit's whole message, as its object holds it.
func (c FixtureCommit) Stored() string {
	if c.Message != "" {
		return c.Message
	}
	return c.Name + "\n"
}

// FixtureTag is one tag of the History suite's repository.
type FixtureTag struct {
	// Name is the tag's name, and Commit the Name of the commit it tags.
	Name, Commit string
	// Annotated makes a tag object, with the tag's name as its message.
	Annotated bool
}

// HistoryFixture is the repository the History suite reads: commits made in
// order, the tags, and the index. A builder makes it with git, or seeds a
// fake with it; the suite compares what the port answers with literals.
type HistoryFixture struct {
	Commits []FixtureCommit
	Tags    []FixtureTag
	// Index is the whole index: the files staged after the commits, the
	// branch Branch checked out. IndexLinks are its symbolic links, staged
	// with them, as a commit's Links are.
	Index, IndexLinks map[string]string
	Branch            string
	// Work is the whole work tree, made after the index is staged: its
	// files, one changed on disk and not staged, and WorkLinks its symbolic
	// links, none of them staged. A builder writes it over the index's.
	Work, WorkLinks map[string]string
}

// FixtureWho is the author and committer of every commit and tag of the
// fixture, and FixtureDate the date of each, so the object names are the
// same on every machine.
func FixtureWho() model.Identity {
	return model.Identity{Name: "Ada Example", Email: "ada@example.com"}
}

// FixtureDate is the date of every commit and tag of the fixture.
const FixtureDate = "2001-02-03T04:05:06Z"

// The fixture's commits, by their full object names under SHA-1.
const (
	commitOne  = "bf9414a92dbd8ac3abfc53ede9b2141c7d12b671"
	commitTwo  = "fc5fc79b053c8cef2900e0677ef63e1ad42740ad"
	commitSide = "67298d661cbab5b2640eac24b984b999593da1c9"
	commitLone = "27deaa946d96475fd4fd5c4e32c7435749744c26"
	commitJoin = "c5fd417fa1754e475f60b2728848c16e2f530eea"
	commitOdd  = "a0ecab899e2e64aea3ea8c7509593f3d5084e3a3"
	// latin, raw and hidden are leaves of odd, each on its own branch.
	commitLatin  = "21b53bacf81d52aa106c5cb1f32105b709e3b89f"
	commitRaw    = "e169fc9b73a9b36a7ce23f60a94c31b941629b2f"
	commitHidden = "86e5827dd06849a84609b682e4ed09d28ca8cc62"
)

// joined is the tree of join, side merged with two, and of odd.
func joined() map[string]string {
	return map[string]string{"rules/a.yaml": "two\n", "README": "read me\n", "empty": "", "side.txt": "side\n"}
}

// oddMessage is odd's message: a subject, a line that reads like a
// trailer, characters past ASCII, a line's trailing spaces and a blank
// last line. rawMessage is raw's, bytes that are not UTF-8 under no header
// that names an encoding; latinMessage is latin's, the same bytes under a
// header that names ISO-8859-1, from which git log would re-code them; and
// hiddenMessage is hidden's, a line past a NUL, at which git log stops.
const (
	oddMessage    = "odd: a subject\n\nSigned-off-by: a line that reads like a trailer\n\u00ff\u00fe are not ASCII  \n\n"
	rawMessage    = "raw: a subject\n\n\xff\xfe are kept as stored\n"
	latinMessage  = "latin: a subject\n\n\xff\xfe are ISO-8859-1\n"
	hiddenMessage = "hidden: a subject\n\x00Signed-off-by: a line past the NUL\n"
)

// TheHistory is the History suite's repository: main with two commits, v1
// a light tag of the first and v2 an annotated tag of the second; side, a
// branch from the first; lone, an orphan with no commit in common; join, a
// branch from side that merges two and then makes odd, whose message keeps
// odd characters and whose tree holds a symbolic link; latin, raw and
// hidden, leaves of odd whose messages git log reads otherwise than they
// are stored; a light tag named MERGE_HEAD of side, with no merge in
// progress; and an index that differs from main and holds a link.
func TheHistory() HistoryFixture {
	return HistoryFixture{
		Commits: []FixtureCommit{
			{Name: "one", Branch: "main", Files: map[string]string{"rules/a.yaml": "one\n", "README": "read me\n"}, ID: commitOne},
			{Name: "two", Branch: "main", Files: map[string]string{"rules/a.yaml": "two\n", "README": "read me\n", "empty": ""}, ID: commitTwo},
			{Name: "side", Branch: "side", From: "one", Files: map[string]string{"rules/a.yaml": "one\n", "side.txt": "side\n"}, ID: commitSide},
			{Name: "lone", Branch: "lone", Files: map[string]string{"lone.txt": "alone\n"}, ID: commitLone},
			{Name: "join", Branch: "join", From: "side", Merge: "two", Files: joined(), ID: commitJoin},
			{Name: "odd", Branch: "join", Files: joined(), Links: map[string]string{"ln": "README"}, Message: oddMessage, ID: commitOdd},
			{Name: "latin", Branch: "latin", From: "odd", Files: joined(), Message: latinMessage, Encoding: "ISO-8859-1", ID: commitLatin},
			{Name: "raw", Branch: "raw", From: "odd", Files: joined(), Message: rawMessage, Raw: true, ID: commitRaw},
			{Name: "hidden", Branch: "hidden", From: "odd", Files: joined(), Message: hiddenMessage, Raw: true, ID: commitHidden},
		},
		Tags:       []FixtureTag{{Name: "v1", Commit: "one"}, {Name: "v2", Commit: "two", Annotated: true}, {Name: "MERGE_HEAD", Commit: "side"}},
		Index:      map[string]string{"rules/a.yaml": "three\n", "README": "read me\n", "empty": "", "new.txt": "new\n"},
		IndexLinks: map[string]string{"ln": "README"},
		Branch:     "main",
		// rules/a.yaml edited on disk at the index's size, and not staged;
		// ln is staged and not on disk; wl, a link to a file, and dl, a
		// link to a folder, are on disk and not staged
		Work:      map[string]string{"rules/a.yaml": "thr3e\n", "README": "read me\n", "empty": "", "new.txt": "new\n"},
		WorkLinks: map[string]string{"wl": "README", "dl": "rules"},
	}
}

// HistoryMaker answers a History and the root of a repository made as f.
type HistoryMaker func(t *testing.T, f HistoryFixture) (port.History, model.Root)

// History runs the History suite: the merge base, the files of a tree and
// of the index, a blob at a tag and in the index, the tags, the merge in
// progress, the commits between two revs and the messages of a range, each
// against literal answers, and the errors the port promises.
func History(t *testing.T, build HistoryMaker) {
	t.Helper()
	h, root := build(t, TheHistory())
	historyBase(t, h, root)
	historyFiles(t, h, root)
	historyWork(t, h, root)
	historyBlobs(t, h, root)
	historyCommits(t, h, root)
	historyLinks(t, h, root)
	historyMessages(t, h, root)
	t.Run("the tags are every tag by name with its commit", func(t *testing.T) {
		want := []model.Tag{{Name: "MERGE_HEAD", Commit: commitSide}, {Name: "v1", Commit: commitOne}, {Name: "v2", Commit: commitTwo}}
		if got, err := h.Tags(t.Context(), root); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Tags = %+v, %v; want %+v", got, err, want)
		}
	})
}

// historyBase holds Base to the commit in common, and to an error for
// histories with none.
func historyBase(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("the merge base is the full object name of the commit in common", func(t *testing.T) {
		for _, pair := range [][2]string{{"v2", "side"}, {"side", "v2"}, {"main", "side"}, {commitTwo, commitSide}} {
			if got, err := h.Base(t.Context(), root, pair[0], pair[1]); err != nil || got != commitOne {
				t.Errorf("Base(%s, %s) = %q, %v; want %s", pair[0], pair[1], got, err, commitOne)
			}
		}
		if got, err := h.Base(t.Context(), root, "v1", "v2"); err != nil || got != commitOne {
			t.Errorf("Base(v1, v2) = %q, %v; want the older, %s", got, err, commitOne)
		}
	})
	t.Run("histories with no commit in common are an error", func(t *testing.T) {
		if got, err := h.Base(t.Context(), root, "v2", "lone"); err == nil || got != "" {
			t.Errorf("Base(v2, lone) = %q, %v; want an error and no name", got, err)
		}
	})
}

// historyFiles holds Files to each file of a tree or of the index, sorted,
// each with an object id that changes with its content and only then
// (historyObjects).
func historyFiles(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("the files of a tree are each file sorted by path", func(t *testing.T) {
		want := []model.Entry{{Path: "README", Size: 8}, {Path: "empty"}, {Path: "rules/a.yaml", Size: 4}}
		for _, rev := range []string{"v2", "main", commitTwo} {
			if got, err := h.Files(t.Context(), root, rev); err != nil || !reflect.DeepEqual(unnamed(got), want) {
				t.Errorf("Files(%s) = %+v, %v; want %+v", rev, unnamed(got), err, want)
			}
		}
		want = []model.Entry{{Path: "README", Size: 8}, {Path: "rules/a.yaml", Size: 4}}
		if got, err := h.Files(t.Context(), root, "v1"); err != nil || !reflect.DeepEqual(unnamed(got), want) {
			t.Errorf("Files(v1) = %+v, %v; want %+v", unnamed(got), err, want)
		}
	})
	t.Run("the files of the index are its own, not the tree's", func(t *testing.T) {
		want := []model.Entry{{Path: "README", Size: 8}, {Path: "empty"}, {Path: "ln", Size: 6, Link: true}, {Path: "new.txt", Size: 4}, {Path: "rules/a.yaml", Size: 6}}
		if got, err := h.Files(t.Context(), root, ""); err != nil || !reflect.DeepEqual(unnamed(got), want) {
			t.Errorf("Files(index) = %+v, %v; want %+v", unnamed(got), err, want)
		}
	})
	t.Run("a rev that does not exist is an error matching ErrNoRev", func(t *testing.T) {
		if got, err := h.Files(t.Context(), root, "v9"); !errors.Is(err, port.ErrNoRev) || got != nil {
			t.Errorf("Files(v9) = %+v, %v; want port.ErrNoRev and no files", got, err)
		}
	})
	historyObjects(t, h, root)
}

// historyObjects holds each entry Files lists to an object id of its own
// content: one id while the content holds, and another when it changes.
func historyObjects(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("a file has one object id while its content holds, and another when it changes", func(t *testing.T) {
		v1, v2, side, index := objects(t, h, root, "v1"), objects(t, h, root, "v2"), objects(t, h, root, "side"), objects(t, h, root, "")
		same := [][3]string{
			{"README", v1["README"], v2["README"]},
			{"README", v2["README"], index["README"]},
			{"empty", v2["empty"], index["empty"]},
			{"rules/a.yaml", v1["rules/a.yaml"], side["rules/a.yaml"]},
		}
		for _, s := range same {
			if s[1] != s[2] {
				t.Errorf("%s holds the same bytes, yet its object ids are %q and %q; want one", s[0], s[1], s[2])
			}
		}
		changed := []string{v1["rules/a.yaml"], v2["rules/a.yaml"], index["rules/a.yaml"]}
		if changed[0] == changed[1] || changed[1] == changed[2] || changed[0] == changed[2] {
			t.Errorf("rules/a.yaml at v1, v2 and the index = %q; want three object ids for its three contents", changed)
		}
		if index["ln"] == index["README"] {
			t.Errorf("the link ln and the file README have one object id, %q; want a link's own, its target's text", index["ln"])
		}
	})
}

// unnamed is entries with no object id, for the rows that hold the rest of
// an entry: ids are each implementation's own, and objects holds them.
func unnamed(entries []model.Entry) []model.Entry {
	if entries == nil {
		return nil
	}
	out := make([]model.Entry, len(entries))
	for i, e := range entries {
		e.Object = ""
		out[i] = e
	}
	return out
}

// objects is the object id of each file Files lists at rev, by path, and
// fails the test on an entry with none.
func objects(t *testing.T, h port.History, root model.Root, rev string) map[string]string {
	t.Helper()
	got, err := h.Files(t.Context(), root, rev)
	if err != nil {
		t.Fatalf("Files(%q) = %v; want its files", rev, err)
	}
	ids := make(map[string]string, len(got))
	for _, e := range got {
		if e.Object == "" {
			t.Errorf("Files(%q): %s has no object id; want the name of its content", rev, e.Path)
		}
		ids[e.Path] = e.Object
	}
	return ids
}

// historyWork holds WorkObjects to the id of each regular file of the work
// tree as its bytes stand, the same as a listing's for the same bytes, and
// to no id for anything else.
func historyWork(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("a work-tree file's object id is a listing's for the same bytes, and another once edited", func(t *testing.T) {
		v2, index := objects(t, h, root, "v2"), objects(t, h, root, "")
		paths := []string{"README", "empty", "new.txt", "rules/a.yaml"}
		got, err := h.WorkObjects(t.Context(), root, paths)
		if err != nil || len(got) != len(paths) {
			t.Fatalf("WorkObjects(%q) = %q, %v; want one id a path", paths, got, err)
		}
		for i, want := range []string{index["README"], v2["empty"], index["new.txt"]} {
			if got[i] != want {
				t.Errorf("WorkObjects: %s = %q; want %q, the listing's for the same bytes", paths[i], got[i], want)
			}
		}
		if got[3] == "" || got[3] == index["rules/a.yaml"] || got[3] == v2["rules/a.yaml"] {
			t.Errorf("WorkObjects: rules/a.yaml edited at the index's size and not staged = %q; want an id of its own, not the index's %q", got[3], index["rules/a.yaml"])
		}
	})
	historyWorkNone(t, h, root)
}

// historyWorkNone holds WorkObjects to no id for a path that is no regular
// file of the work tree reached through no link, and to an empty list for
// no path.
func historyWorkNone(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("a path that is no regular file of the work tree, reached through no link, has no object id", func(t *testing.T) {
		paths := []string{"ln", "wl", "dl/a.yaml", "rules", "nowhere/x", "../README", "README/x", `"README`, ""}
		got, err := h.WorkObjects(t.Context(), root, paths)
		if err != nil || len(got) != len(paths) {
			t.Fatalf("WorkObjects(%q) = %q, %v; want one answer a path", paths, got, err)
		}
		for i, id := range got {
			if id != "" {
				t.Errorf("WorkObjects: %q = %q; want no id", paths[i], id)
			}
		}
	})
	t.Run("no path is an empty list", func(t *testing.T) {
		if got, err := h.WorkObjects(t.Context(), root, nil); err != nil || got == nil || len(got) != 0 {
			t.Errorf("WorkObjects(none) = %#v, %v; want an empty list", got, err)
		}
	})
}

// historyBlobs holds Blob to the whole content at a rev, and to
// fs.ErrNotExist for a path the rev lacks and only then.
func historyBlobs(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("a blob is the file's whole content at the rev", func(t *testing.T) {
		cases := []struct{ rev, path, want string }{
			{"v1", "rules/a.yaml", "one\n"}, {"v2", "rules/a.yaml", "two\n"}, {"side", "side.txt", "side\n"}, {"", "rules/a.yaml", "three\n"}, {"", "new.txt", "new\n"},
		}
		for _, c := range cases {
			if got, err := h.Blob(t.Context(), root, c.rev, c.path); err != nil || string(got) != c.want {
				t.Errorf("Blob(%q, %s) = %q, %v; want %q", c.rev, c.path, got, err, c.want)
			}
		}
	})
	t.Run("an empty file is an empty slice", func(t *testing.T) {
		if got, err := h.Blob(t.Context(), root, "v2", "empty"); err != nil || got == nil || len(got) != 0 {
			t.Errorf("Blob(v2, empty) = %#v, %v; want an empty slice", got, err)
		}
	})
	t.Run("a path the rev lacks matches fs.ErrNotExist", func(t *testing.T) {
		for _, c := range [][2]string{{"v1", "empty"}, {"v1", "rules"}, {"", "side.txt"}, {"v2", "nowhere/x"}} {
			if got, err := h.Blob(t.Context(), root, c[0], c[1]); !errors.Is(err, fs.ErrNotExist) || got != nil {
				t.Errorf("Blob(%q, %s) = %q, %v; want fs.ErrNotExist", c[0], c[1], got, err)
			}
		}
	})
	t.Run("a blob at a rev that does not exist is not fs.ErrNotExist", func(t *testing.T) {
		if _, err := h.Blob(t.Context(), root, "v9", "rules/a.yaml"); !errors.Is(err, port.ErrNoRev) || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Blob(v9, rules/a.yaml): %v; want port.ErrNoRev, not fs.ErrNotExist", err)
		}
	})
	historyBlobsMany(t, h, root)
}

// historyBlobsMany holds Blobs to Blob's answer for each path, in the order
// asked, and to Blob's refusals with no content.
func historyBlobsMany(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("blobs are each file's whole content, in the order asked, as Blob answers them", func(t *testing.T) {
		got, err := h.Blobs(t.Context(), root, "v2", []string{"rules/a.yaml", "empty", "README"})
		if err != nil || len(got) != 3 {
			t.Fatalf("Blobs(v2) = %q, %v; want three files", got, err)
		}
		for i, p := range []string{"rules/a.yaml", "empty", "README"} {
			want, werr := h.Blob(t.Context(), root, "v2", p)
			if werr != nil || got[i] == nil || string(got[i]) != string(want) {
				t.Errorf("Blobs(v2)[%s] = %#v; want %#v (%v), an empty file an empty slice", p, got[i], want, werr)
			}
		}
		if got, err := h.Blobs(t.Context(), root, "v2", nil); err != nil || got == nil || len(got) != 0 {
			t.Errorf("Blobs(v2, no path) = %#v, %v; want an empty list", got, err)
		}
		if got, err := h.Blobs(t.Context(), root, "", []string{"new.txt", "rules/a.yaml"}); err != nil || len(got) != 2 || string(got[0]) != "new\n" || string(got[1]) != "three\n" {
			t.Errorf("Blobs(index) = %q, %v; want the index's own content", got, err)
		}
	})
	historyBlobsRefused(t, h, root)
}

// historyBlobsRefused holds Blobs to Blob's refusals, with no content.
func historyBlobsRefused(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("blobs of a path the rev lacks, a link or a missing rev answer as Blob does, with no content", func(t *testing.T) {
		for _, c := range [][2]string{{"v1", "empty"}, {"v1", "rules"}, {"v2", "nowhere/x"}, {commitOdd, "ln"}, {"", "ln"}} {
			if got, err := h.Blobs(t.Context(), root, c[0], []string{"README", c[1]}); !errors.Is(err, fs.ErrNotExist) || got != nil {
				t.Errorf("Blobs(%q, README and %s) = %q, %v; want fs.ErrNotExist and no content", c[0], c[1], got, err)
			}
		}
		if got, err := h.Blobs(t.Context(), root, "v9", []string{"README"}); !errors.Is(err, port.ErrNoRev) || errors.Is(err, fs.ErrNotExist) || got != nil {
			t.Errorf("Blobs(v9) = %q, %v; want port.ErrNoRev and no content", got, err)
		}
	})
}

// historyCommits holds Merging to none while no merge is in progress, a tag
// named MERGE_HEAD there too, and Commits to the commits between two revs,
// each after its parents.
func historyCommits(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("with no merge in progress there is none, though a tag is named MERGE_HEAD", func(t *testing.T) {
		if got, err := h.Merging(t.Context(), root); err != nil || got == nil || len(got) != 0 {
			t.Errorf("Merging = %#v, %v; want an empty list", got, err)
		}
	})
	t.Run("the commits between two revs are each after its parents", func(t *testing.T) {
		one := model.Commit{ID: commitOne}
		two := model.Commit{ID: commitTwo, Parents: []string{commitOne}}
		side := model.Commit{ID: commitSide, Parents: []string{commitOne}}
		lone := model.Commit{ID: commitLone}
		cases := []struct {
			from, to string
			want     []model.Commit
		}{
			{"main", "side", []model.Commit{side}},
			{"side", "v2", []model.Commit{two}},
			{commitSide, commitTwo, []model.Commit{two}},
			{"lone", "main", []model.Commit{one, two}},
			{"main", "lone", []model.Commit{lone}},
			{"main", "v1", []model.Commit{}},
			{"v2", "v2", []model.Commit{}},
		}
		for _, c := range cases {
			if got, err := h.Commits(t.Context(), root, c.from, c.to); err != nil || got == nil || !reflect.DeepEqual(got, c.want) {
				t.Errorf("Commits(%s, %s) = %+v, %v; want %+v", c.from, c.to, got, err, c.want)
			}
		}
	})
	t.Run("the commits from or to a rev that does not exist match port.ErrNoRev", func(t *testing.T) {
		for _, pair := range [][2]string{{"v9", "main"}, {"main", "v9"}} {
			if got, err := h.Commits(t.Context(), root, pair[0], pair[1]); !errors.Is(err, port.ErrNoRev) || got != nil {
				t.Errorf("Commits(%s, %s) = %+v, %v; want port.ErrNoRev", pair[0], pair[1], got, err)
			}
		}
	})
}

// historyLinks holds Files to a symbolic link of a tree or of the index
// marked as one, with the length of the path it names, and Blob to never
// reading it (a link staged in the index among them).
func historyLinks(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	t.Run("a link of a tree is an entry marked Link, which Blob never reads", func(t *testing.T) {
		want := []model.Entry{{Path: "README", Size: 8}, {Path: "empty"}, {Path: "ln", Size: 6, Link: true}, {Path: "rules/a.yaml", Size: 4}, {Path: "side.txt", Size: 5}}
		if got, err := h.Files(t.Context(), root, commitOdd); err != nil || !reflect.DeepEqual(unnamed(got), want) {
			t.Errorf("Files(odd) = %+v, %v; want %+v", unnamed(got), err, want)
		}
		if got, err := h.Blob(t.Context(), root, commitOdd, "ln"); !errors.Is(err, fs.ErrNotExist) || got != nil {
			t.Errorf("Blob(odd, ln) = %q, %v; want fs.ErrNotExist", got, err)
		}
	})
	t.Run("a link of the index is never read by Blob as a file", func(t *testing.T) {
		if got, err := h.Blob(t.Context(), root, "", "ln"); !errors.Is(err, fs.ErrNotExist) || got != nil {
			t.Errorf("Blob(index, ln) = %q, %v; want fs.ErrNotExist", got, err)
		}
	})
}

// historyMessages holds Messages to the message of each commit of head
// that base lacks, merges followed through both parents, byte for byte as
// its object stores it; to refusing a range with a message git log reads
// otherwise, naming the commit; and to refusing an end that is not the
// full name of a commit.
func historyMessages(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	text := map[string]string{}
	for _, c := range TheHistory().Commits {
		text[c.ID] = c.Stored()
	}
	of := func(ids ...string) []model.Message {
		out := []model.Message{}
		for _, id := range ids {
			out = append(out, model.Message{Commit: id, Text: text[id]})
		}
		slices.SortFunc(out, func(a, b model.Message) int { return strings.Compare(a.Commit, b.Commit) })
		return out
	}
	t.Run("a range is each commit head has and base lacks, once each, sorted by commit", func(t *testing.T) {
		messagesOfRanges(t, h, root, of)
	})
	t.Run("a range with no commit is an empty list", func(t *testing.T) {
		for _, c := range [][2]string{{commitTwo, commitTwo}, {commitTwo, commitOne}, {commitOdd, commitSide}} {
			if got, err := h.Messages(t.Context(), root, c[0], c[1]); err != nil || got == nil || len(got) != 0 {
				t.Errorf("Messages(%s, %s) = %#v, %v; want an empty list", c[0], c[1], got, err)
			}
		}
	})
	t.Run("a message is whole, byte for byte as its object stores it", func(t *testing.T) {
		for _, c := range []struct{ base, head, want string }{{commitJoin, commitOdd, oddMessage}, {commitOdd, commitRaw, rawMessage}} {
			want := []model.Message{{Commit: c.head, Text: c.want}}
			if got, err := h.Messages(t.Context(), root, c.base, c.head); err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("Messages(%s, %s) = %q, %v; want %q", c.base, c.head, got, err, want)
			}
		}
	})
	t.Run("a range with a message git log reads otherwise is refused", func(t *testing.T) {
		messagesHidden(t, h, root)
	})
	t.Run("an end that is not the full name of a commit is refused", func(t *testing.T) {
		messagesRefused(t, h, root)
	})
}

// messagesOfRanges holds Messages to each commit of a range, through a
// merge's both parents and across histories with none in common.
func messagesOfRanges(t *testing.T, h port.History, root model.Root, of func(ids ...string) []model.Message) {
	t.Helper()
	cases := []struct {
		base, head string
		want       []model.Message
	}{
		{commitOne, commitTwo, of(commitTwo)},
		{commitTwo, commitJoin, of(commitSide, commitJoin)},
		{commitSide, commitJoin, of(commitTwo, commitJoin)},
		{commitOne, commitOdd, of(commitTwo, commitSide, commitJoin, commitOdd)},
		{commitTwo, commitLone, of(commitLone)},
	}
	for _, c := range cases {
		if got, err := h.Messages(t.Context(), root, c.base, c.head); err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Messages(%s, %s) = %q, %v; want %q", c.base, c.head, got, err, c.want)
		}
	}
}

// messagesHidden holds Messages to refusing, with no list, a range that
// holds a message past a NUL, at which git log stops, or under a header
// that names an encoding other than UTF-8, from which git log re-codes it.
// That the error
// names the commit is held by each implementation's own test: this suite is
// not a test file, and it reads no error by its text (go.errors.8).
func messagesHidden(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	for _, c := range [][2]string{{commitOdd, commitHidden}, {commitOne, commitHidden}, {commitOdd, commitLatin}} {
		if got, err := h.Messages(t.Context(), root, c[0], c[1]); err == nil || got != nil {
			t.Errorf("Messages(%s, %s) = %q, %v; want refused, and no list", c[0], c[1], got, err)
		}
	}
}

// messagesRefused holds Messages to refusing each end of notCommits, as
// base and as head, with no list.
func messagesRefused(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	for _, end := range notCommits() {
		for _, c := range [][2]string{{end, commitTwo}, {commitOne, end}} {
			if got, err := h.Messages(t.Context(), root, c[0], c[1]); err == nil || got != nil {
				t.Errorf("Messages(%q, %q) = %q, %v; want refused, and no list", c[0], c[1], got, err)
			}
		}
	}
}

// notCommits are ends of a range that name no commit by its full name: a
// tag, a branch, HEAD, a short name, capitals, an option, a range, an
// object of no commit, a name of the other hash's length, and nothing.
func notCommits() []string {
	return []string{
		"v1", "main", "HEAD", commitOne[:12], strings.ToUpper(commitOne), "--all", "-n1",
		"^" + commitOne, commitOne + "..", commitOne + "\n", commitOne + "^{commit}",
		strings.Repeat("0", 40), commitOne + strings.Repeat("0", 24), "",
	}
}

// NoTags runs the part of the History suite for a repository with no tag:
// an empty list, not an error.
func NoTags(t *testing.T, h port.History, root model.Root) {
	t.Helper()
	if got, err := h.Tags(t.Context(), root); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Tags with none = %#v, %v; want an empty list", got, err)
	}
}
