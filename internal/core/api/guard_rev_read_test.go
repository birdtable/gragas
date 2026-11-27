package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// countingHistory counts the reads of file content a History is asked for:
// the files each Blobs call was asked for, and the Blob calls.
type countingHistory struct {
	port.History
	blobs int
	asked [][]string
}

func (c *countingHistory) Blob(ctx context.Context, root model.Root, rev, p string) ([]byte, error) {
	c.blobs++
	return c.History.Blob(ctx, root, rev, p)
}

func (c *countingHistory) Blobs(ctx context.Context, root model.Root, rev string, paths []string) ([][]byte, error) {
	c.asked = append(c.asked, slices.Clone(paths))
	return c.History.Blobs(ctx, root, rev, paths)
}

// TestAJudgeReadingAFolderOfACommitReadsItInOneRunOfTheHistory: the tree a commit is judged by reads a file together with the
// commit's next files in path order, in one read of the history, and a file
// read after is already held, in this folder or another. The bytes are those of the one-file read.
// Risk: one read of the repository, a git process, for each file of a commit,
// which made a commit's check cost a thousand processes.
func TestAJudgeReadingAFolderOfACommitReadsItInOneRunOfTheHistory(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "x\n"})
	files := map[string]string{"d/a.yaml": "a\n", "d/b.yaml": "b\n", "d/c.yaml": "c\n", "e/x.yaml": "x\n", "top.md": "t\n"}
	w.SeedRepo(root, porttest.Repo{Tags: map[string]porttest.Rev{"v1": {Commit: "c1", Files: files}}})
	h := &countingHistory{History: w.Ports().History}
	got, err := coreapi.ReadRevTree(context.Background(), w.Ports().Tree, h, root, "v1", []string{"d/a.yaml", "d/b.yaml", "d/c.yaml", "e/x.yaml", "top.md"})
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range []string{"d/a.yaml", "d/b.yaml", "d/c.yaml", "e/x.yaml", "top.md"} {
		if string(got[i]) != files[p] {
			t.Errorf("%s read as %q, want %q", p, got[i], files[p])
		}
	}
	want := [][]string{{"d/a.yaml", "d/b.yaml", "d/c.yaml", "e/x.yaml", "top.md"}}
	if !slices.EqualFunc(h.asked, want, slices.Equal) || h.blobs != 0 {
		t.Errorf("the history was asked for %v and %d single files, want %v and none", h.asked, h.blobs, want)
	}
}

// shapedHistory is a History whose Blobs is shaped by a test: it refuses a
// list that holds the path in bad, or, with short set, answers a list of more
// than one file one file short. A one-file list is answered as the fake does.
type shapedHistory struct {
	countingHistory
	bad   string
	short bool
}

func (s *shapedHistory) Blobs(ctx context.Context, root model.Root, rev string, paths []string) ([][]byte, error) {
	if s.bad != "" && slices.Contains(paths, s.bad) {
		s.asked = append(s.asked, slices.Clone(paths))
		return nil, errors.New("git has not the object the tree lists for " + s.bad)
	}
	got, err := s.countingHistory.Blobs(ctx, root, rev, paths)
	if s.short && len(got) > 1 {
		got = got[:len(got)-1]
	}
	return got, err
}

// readOne reads paths of rev v1 of a seeded tree through a shaped history.
func readOne(t *testing.T, rev porttest.Rev, s *shapedHistory, paths ...string) ([][]byte, error) {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "x\n"})
	w.SeedRepo(root, porttest.Repo{Tags: map[string]porttest.Rev{"v1": rev}})
	s.History = w.Ports().History
	return coreapi.ReadRevTree(context.Background(), w.Ports().Tree, s, root, "v1", paths)
}

// TestASiblingTheJudgeNeverAskedForCannotFailTheFileItDid: when the
// batch is refused because of a sibling, the file asked for is read alone and
// is the only error there is; the sibling fails only when it is itself asked
// for.
// Risk: a check that fails on a file of a folder no judge read.
func TestASiblingTheJudgeNeverAskedForCannotFailTheFileItDid(t *testing.T) {
	t.Parallel()
	rev := porttest.Rev{Commit: "c1", Files: map[string]string{"d/a.yaml": "a\n", "d/bad.yaml": "b\n", "d/z.yaml": "z\n"}}
	s := &shapedHistory{bad: "d/bad.yaml"}
	got, err := readOne(t, rev, s, "d/a.yaml", "d/z.yaml")
	if err != nil || string(got[0]) != "a\n" || string(got[1]) != "z\n" {
		t.Fatalf("reading d/a.yaml and d/z.yaml = %q, %v; want their content, the sibling's failure not theirs", got, err)
	}
	if _, err := readOne(t, rev, &shapedHistory{bad: "d/bad.yaml"}, "d/bad.yaml"); err == nil {
		t.Error("reading d/bad.yaml itself gave no error, want the history's")
	}
}

// TestTheBatchOfAFolderHoldsNoLinkAndIsBoundedInFilesAndBytes: the files
// read with one are at most fetchFiles, no link, and, with their headers, no
// more than the history's cap.
// Risk: a batch the history refuses for its size or for a link, or one that
// reads a whole big folder for a file.
func TestTheBatchOfAFolderHoldsNoLinkAndIsBoundedInFilesAndBytes(t *testing.T) {
	t.Parallel()
	t.Run("a link beside the file is not asked for", func(t *testing.T) {
		t.Parallel()
		rev := porttest.Rev{Commit: "c1", Files: map[string]string{"d/a.yaml": "a\n", "d/b.yaml": "b\n"}, Links: map[string]string{"d/ln": "a.yaml"}}
		s := &shapedHistory{}
		if _, err := readOne(t, rev, s, "d/a.yaml"); err != nil {
			t.Fatal(err)
		}
		for _, ask := range s.asked {
			if slices.Contains(ask, "d/ln") {
				t.Errorf("the history was asked for the link in %v", ask)
			}
		}
	})
	t.Run("at most 256 files at a time", func(t *testing.T) {
		t.Parallel()
		files := map[string]string{}
		for i := range 300 {
			files[fmt.Sprintf("d/f%03d.yaml", i)] = "x\n"
		}
		s := &shapedHistory{}
		if _, err := readOne(t, porttest.Rev{Commit: "c1", Files: files}, s, "d/f000.yaml"); err != nil {
			t.Fatal(err)
		}
		if len(s.asked) != 1 || len(s.asked[0]) != 256 {
			t.Errorf("the history was asked for %d lists, the first of %d files; want one list of 256", len(s.asked), len(s.asked[0]))
		}
	})
	t.Run("at most the cap in bytes, headers counted", func(t *testing.T) {
		t.Parallel()
		big := strings.Repeat("x", 3<<20)
		files := map[string]string{"d/a.yaml": "a\n", "d/big1": big, "d/big2": big, "d/edge": strings.Repeat("y", 4<<20)}
		s := &shapedHistory{}
		if _, err := readOne(t, porttest.Rev{Commit: "c1", Files: files}, s, "d/a.yaml"); err != nil {
			t.Fatal(err)
		}
		for _, ask := range s.asked {
			var total int
			for _, p := range ask {
				total += len(files[p]) + 96
			}
			if total > 4<<20 {
				t.Errorf("a list of %v is %d bytes with its headers, past the history's cap", ask, total)
			}
		}
	})
}

// TestAHistoryThatAnswersAFileShortCannotMisassignTheOthers: a batch
// answered with fewer files than asked is refused as a whole, and each file is
// then read as it is, none with another's bytes.
func TestAHistoryThatAnswersAFileShortCannotMisassignTheOthers(t *testing.T) {
	t.Parallel()
	rev := porttest.Rev{Commit: "c1", Files: map[string]string{"d/a.yaml": "a\n", "d/b.yaml": "b\n", "d/c.yaml": "c\n"}}
	got, err := readOne(t, rev, &shapedHistory{short: true}, "d/a.yaml", "d/b.yaml", "d/c.yaml")
	if err != nil || string(got[0]) != "a\n" || string(got[1]) != "b\n" || string(got[2]) != "c\n" {
		t.Errorf("reading the folder = %q, %v; want a, b, c", got, err)
	}
}

// TestAnEmptiedTreeListsNothingWhateverItsSourceListedFirst: the tree a
// guard empties from the one it has listed holds no file, so its listing is
// empty and it lacks the folders of the source.
// Risk: the sorted-path cache copied into the emptied tree listed the source's
// paths over files the tree no longer holds, as entries with no path.
func TestAnEmptiedTreeListsNothingWhateverItsSourceListedFirst(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "x\n"})
	files := map[string]string{"fixture/base/rulebook.yaml": "a\n", "fixture/other/rulebook.yaml": "b\n", "note.txt": "n\n"}
	w.SeedRepo(root, porttest.Repo{Tags: map[string]porttest.Rev{"v1": {Commit: "c1", Files: files}}})
	got, err, serr := coreapi.EmptiedTreeListing(context.Background(), w.Ports().Tree, w.Ports().History, root, "v1", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("the emptied tree lists %d entries, want none: %v", len(got), got)
	}
	if !errors.Is(serr, fs.ErrNotExist) {
		t.Errorf("the emptied tree says of fixture: %v, want it does not exist", serr)
	}
}
