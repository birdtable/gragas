package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/port"
)

// What a repository run reads: the tracked files from the work tree, the texts
// given by path, and the merge base.

// take is what the three checks need of one text, made while the text is
// held and kept after it is let go: nothing of a run keeps a text past its
// own turn, so a run holds one text at a time, under maxRead, and no total of
// the files needs a bound.
type take struct {
	Path string
	// Binary says the text holds a NUL byte, which the names check passes
	// over.
	Binary bool
	// Hits are the names findings of the text, none with no list.
	Hits []audit.Finding
	// Counted are the text's counted lines, none for a file the counts do
	// not read.
	Counted []audit.Counted
}

// takeOf is the take of one text, made while the caller still holds it.
func (a RepoAudit) takeOf(count func(audit.Doc) []audit.Counted, path, text string) take {
	t := take{Path: path, Binary: strings.IndexByte(text, 0) >= 0}
	if a.Names != nil {
		t.Hits, _ = audit.CheckNames(audit.NamesInput{Rules: audit.RepoRules(a.Rules), List: matcher{a.Names}, Files: []audit.Doc{{Path: path, Text: text}}})
	}
	if a.counted(path) {
		t.Counted = count(audit.Doc{Path: path, Text: text})
	}
	return t
}

// readTracked is the take of each tracked file that is not a link, read one
// at a time from the work tree; a file gone from the work tree, or one that
// is not a regular file, is left out and counted in unread, and one past
// maxRead is named in skipped. A file that fails leaves the ones before it
// taken and the ones after it read.
func (a RepoAudit) readTracked(tracked []audit.Tracked) (takes []take, skipped []string, unread int) {
	count := audit.NewCounter(countings(a.Counting))
	for _, t := range tracked {
		if t.Link {
			continue
		}
		text, err := readCapped(a.Src, t.Path)
		switch {
		case errors.Is(err, errPastCap):
			skipped = append(skipped, t.Path)
		case err != nil:
			unread++
		default:
			takes = append(takes, a.takeOf(count, t.Path, text))
		}
	}
	return takes, skipped, unread
}

// errPastCap says a file holds more than maxRead bytes, so none of it is kept.
var errPastCap = errors.New("the file is larger than the read cap")

// readCapped is the whole text of the regular file at name in src, read
// through its open handle and never past maxRead+1 bytes. The
// handle is statted, never the name: a folder, a FIFO or a device is refused
// before a read can block on it, as the adapter's ReadFile refuses them, and
// a size past the cap is errPastCap with nothing read. A file that grows
// after the stat is still read no further than one byte past the cap, and is
// errPastCap too, never kept in part.
func readCapped(src fs.FS, name string) (text string, err error) {
	f, err := src.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", name)
	}
	if info.Size() > maxRead {
		return "", errPastCap
	}
	var b strings.Builder
	b.Grow(int(info.Size()))
	if _, err := io.Copy(&b, io.LimitReader(f, maxRead+1)); err != nil {
		return "", err
	}
	if b.Len() > maxRead {
		return "", errPastCap
	}
	return b.String(), nil
}

// binaryCount is how many of the takes hold a NUL byte: the names check takes
// them for binary and reads none, so a run says how many it passed over.
func binaryCount(takes []take) int {
	n := 0
	for _, t := range takes {
		if t.Binary {
			n++
		}
	}
	return n
}

// readTexts is the take of each of Texts, read one at a time by ReadOwned. A path that is not
// absolute and clean, one another account owns or can write, one the port
// cannot read and one past maxRead is named in skipped, with the reason, and
// is no pass.
func (a RepoAudit) readTexts() (takes []take, skipped []string) {
	count := audit.NewCounter(countings(a.Counting))
	for i, p := range a.Texts {
		// A text is named by its place in Texts, never by its path: a path
		// outside the repository is the likeliest to hold a login.
		at := "given text " + fmt.Sprint(i+1)
		b, err := a.Owned.ReadOwned(p)
		switch {
		case errors.Is(err, port.ErrNotOwned):
			skipped = append(skipped, at+" was not read: it is not owned by this account, or another account can write it")
		case errors.Is(err, fs.ErrNotExist):
			skipped = append(skipped, at+" was not read: it is not there")
		case err != nil:
			skipped = append(skipped, at+" could not be read")
		case len(b) > maxRead:
			skipped = append(skipped, at+" is larger than "+fmt.Sprint(maxRead>>20)+" MiB and was not read")
		default:
			takes = append(takes, a.takeOf(count, at, string(b)))
		}
	}
	return takes, skipped
}

// mergeBase is the full name of the merge base of HEAD and the base revision.
func (a RepoAudit) mergeBase(ctx context.Context) (string, error) {
	rev := a.Base
	if rev == "" {
		rev = DefaultBase
	}
	b, err := a.History.Base(ctx, a.Root, rev, "HEAD")
	if err != nil {
		return "", fmt.Errorf("the merge base of %s and HEAD could not be found: %w", rev, err)
	}
	return b, nil
}
