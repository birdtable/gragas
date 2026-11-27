package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/format"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// allocCeiling is the most CheckRepo may allocate on a run whose one
// tracked file is planted one byte past the cap: a quarter of maxRead. The
// run allocates well under it with the capped read; with the size check of
// the handle's Stat taken out, the same run allocates the cap and more, as
// it reads the whole file before it says it is too large.
const allocCeiling = coreapi.MaxRead / 4

// countFS is a work tree whose files count each byte a reader is handed, so
// a test says how much of a file a run read. A name in endless opens as a
// file whose Stat says it holds one byte and whose reads never end, until
// twice the cap, where the read fails so a plant is red at once and never
// hangs.
type countFS struct {
	fs.ReadFileFS
	read    *atomic.Int64
	endless string
}

func (c countFS) Open(name string) (fs.File, error) {
	if name == c.endless {
		return endlessFile{read: c.read}, nil
	}
	f, err := c.ReadFileFS.Open(name)
	if err != nil {
		return nil, err
	}
	return countFile{File: f, read: c.read}, nil
}

// ReadFile reads through Open, so a run that read the whole file by name
// would be counted too.
func (c countFS) ReadFile(name string) ([]byte, error) {
	f, err := c.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

type countFile struct {
	fs.File
	read *atomic.Int64
}

func (f countFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.read.Add(int64(n))
	return n, err
}

type endlessFile struct{ read *atomic.Int64 }

func (f endlessFile) Stat() (fs.FileInfo, error) { return oneByte{}, nil }
func (f endlessFile) Close() error               { return nil }
func (f endlessFile) Read(p []byte) (int, error) {
	if f.read.Load() > 2*coreapi.MaxRead {
		return 0, errors.New("the endless file was read past twice the cap")
	}
	for i := range p {
		p[i] = 'a'
	}
	f.read.Add(int64(len(p)))
	return len(p), nil
}

// oneByte is the endless file's Stat: a regular file of one byte.
type oneByte struct{}

func (oneByte) Name() string       { return "endless.txt" }
func (oneByte) Size() int64        { return 1 }
func (oneByte) Mode() fs.FileMode  { return 0 }
func (oneByte) ModTime() time.Time { return time.Time{} }
func (oneByte) IsDir() bool        { return false }
func (oneByte) Sys() any           { return nil }

// skippedRow is whether a run said path was too large and was not read.
func skippedRow(r coreapi.RepoFindings, path string) bool {
	for _, n := range r.NotChecked {
		if strings.Contains(n.Why, path+" is larger than 8 MiB and was not read") {
			return true
		}
	}
	return false
}

// plantedAudit is a run over one tracked file, big.txt, planted in a folder
// on disk one byte past the cap by a truncate, so nothing is written but its
// size; read counts every byte a reader of it is handed.
func plantedAudit(t *testing.T) (a coreapi.RepoAudit, read *atomic.Int64) {
	t.Helper()
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "big.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(f.Truncate(coreapi.MaxRead+1), f.Close()); err != nil {
		t.Fatal(err)
	}
	src, ok := os.DirFS(dir).(fs.ReadFileFS)
	if !ok {
		t.Fatal("os.DirFS is no fs.ReadFileFS")
	}
	read = new(atomic.Int64)
	a = trackedAudit(t, map[string]string{"big.txt": "x"}, nil)
	a.Src = countFS{ReadFileFS: src, read: read}
	return a, read
}

// TestAFileThatNeverEndsIsReadOneBytePastTheCapAndSkipped holds the bound on
// the read itself: a file whose Stat says one byte but which
// grows without end is read for no more than the cap and one byte, and is
// said to be too large, never kept in part.
// boundary: maxRead
// Risk: a file that grows as it is read holds the whole run's memory.
func TestAFileThatNeverEndsIsReadOneBytePastTheCapAndSkipped(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"endless.txt": "a"}, nil)
	read := new(atomic.Int64)
	a.Src = countFS{ReadFileFS: a.Src, read: read, endless: "endless.txt"}
	r := coreapi.CheckRepo(context.Background(), a)
	if got := read.Load(); got > coreapi.MaxRead+1 {
		t.Errorf("%d bytes of the endless file were read, want at most %d", got, coreapi.MaxRead+1)
	}
	if !skippedRow(r, "endless.txt") {
		t.Errorf("no row says endless.txt was too large to read; rows %v", r.NotChecked)
	}
}

// TestAPlantedFileOneBytePastTheCapIsSkippedWithNothingRead holds the other
// half: a file on disk whose size is past the cap is said to be too large
// from its open handle's Stat, with no byte of it read.
// boundary: maxRead
// Risk: a planted file far past the cap is read whole before it is refused.
func TestAPlantedFileOneBytePastTheCapIsSkippedWithNothingRead(t *testing.T) {
	t.Parallel()
	a, read := plantedAudit(t)
	r := coreapi.CheckRepo(context.Background(), a)
	if got := read.Load(); got != 0 {
		t.Errorf("%d bytes of a file past the cap were read, want none", got)
	}
	if !skippedRow(r, "big.txt") {
		t.Errorf("no row says big.txt was too large to read; rows %v", r.NotChecked)
	}
}

// allocRuns is how many runs the allocation and the live rows take the least
// of. Both measure in a rerun child where theirs is the only test, so no other
// test's work is in a window; the least of the runs leaves out what the Go
// runtime itself does in one, and a run can only read more than its own,
// never less.
const allocRuns = 32

// allocated is the word the allocation row's child is given.
const allocated = "allocated"

// TestARunOverAPlantedFilePastTheCapAllocatesUnderTheCeiling holds what the
// run costs, not only what it reads: CheckRepo over the planted file
// allocates less than allocCeiling. TotalAlloc is the whole process's, so the
// measure is made in a rerun of this test binary that runs
// TestTheHelperRunsOnlyWhenRerun alone, where no parallel test allocates in
// the window, however many run beside this row and however busy the machine
// (beside them it once read 9773736 bytes at load 13). This row is
// parallel and fails with the child's output. A run that reads the file
// allocates past the cap in every window, so the least of them is past the
// ceiling too. The ceiling is never raised to quiet it. The counting reader of
// the tests above and the Tree contract's rows are the first proof; this row
// is the second.
// Risk: a run reads a file past the cap into memory before it refuses it.
func TestARunOverAPlantedFilePastTheCapAllocatesUnderTheCeiling(t *testing.T) {
	t.Parallel()
	code, out, err := configtest.Rerun(t.Context(), allocated)
	if err != nil || code != 0 {
		t.Fatalf("the child that measures the allocation ended %d, %v:\n%s", code, err, out)
	}
}

// leastAllocated is the least, over allocRuns runs of CheckRepo over the
// planted file, that one run allocated, failing if a run does not say the
// file was too large to read.
func leastAllocated(t *testing.T) uint64 {
	t.Helper()
	a, _ := plantedAudit(t)
	least := ^uint64(0)
	for range allocRuns {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		r := coreapi.CheckRepo(context.Background(), a)
		runtime.ReadMemStats(&after)
		if !skippedRow(r, "big.txt") {
			t.Fatalf("no row says big.txt was too large to read; rows %v", r.NotChecked)
		}
		least = min(least, after.TotalAlloc-before.TotalAlloc)
	}
	return least
}

// liveFS is a work tree whose files, each as it is closed, measure the live
// heap: the file's text is then built and held by the run, and whatever the
// run kept of the files before it is held too. Each file is a sparse planted
// one just under the cap, so nothing is written but its size. peak is the
// most the heap held live at any close of a run. HeapAlloc is the whole
// process's, and the package's other tests run beside this one, so a window
// can only read more than the run held, never less (see allocRuns).
type liveFS struct {
	fs.ReadFileFS
	peak *atomic.Uint64
}

func (l liveFS) Open(name string) (fs.File, error) {
	f, err := l.ReadFileFS.Open(name)
	if err != nil {
		return nil, err
	}
	return liveFile{File: f, peak: l.peak}, nil
}

type liveFile struct {
	fs.File
	peak *atomic.Uint64
}

// Close measures after a collection, so the texts the run let go are not
// counted: only a text it still holds is.
func (f liveFile) Close() error {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	for p := f.peak.Load(); m.HeapAlloc > p && !f.peak.CompareAndSwap(p, m.HeapAlloc); p = f.peak.Load() {
	}
	return f.File.Close()
}

// liveAudit is a run over n tracked files, each one byte under the cap, on
// disk; peak is the most live heap at a close. A .txt file is sparse (NUL, so
// binary to the names check and never counted); a .go file holds a //nolint
// on its first line and spaces after it, which the counts read and keep a
// row.
func liveAudit(t *testing.T, n int, ext string) (a coreapi.RepoAudit, peak *atomic.Uint64) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{}
	for i := range n {
		name := "f" + string(rune('a'+i)) + ext
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := errors.Join(fill(f, ext), f.Close()); err != nil {
			t.Fatal(err)
		}
		files[name] = "x"
	}
	src, ok := os.DirFS(dir).(fs.ReadFileFS)
	if !ok {
		t.Fatal("os.DirFS is no fs.ReadFileFS")
	}
	peak = new(atomic.Uint64)
	a = trackedAudit(t, files, nil)
	a.Src = liveFS{ReadFileFS: src, peak: peak}
	return a, peak
}

// fill sizes f to one byte under the cap: sparse for a .txt, text for a .go.
func fill(f *os.File, ext string) error {
	if ext != ".go" {
		return f.Truncate(coreapi.MaxRead - 1)
	}
	const head = "//nolint\npackage p\n"
	pad := []byte(strings.Repeat(" ", 1<<20))
	left := coreapi.MaxRead - 1
	if _, err := f.WriteString(head); err != nil {
		return err
	}
	for left -= len(head); left > 0; left -= len(pad) {
		if _, err := f.Write(pad[:min(left, len(pad))]); err != nil {
			return err
		}
	}
	return nil
}

// leastPeak is the least over liveRuns runs of the most live heap at a
// close of a run's files.
func leastPeak(t *testing.T, a coreapi.RepoAudit, peak *atomic.Uint64) uint64 {
	t.Helper()
	least := ^uint64(0)
	for range liveRuns {
		peak.Store(0)
		coreapi.CheckRepo(context.Background(), a)
		least = min(least, peak.Load())
	}
	return least
}

// liveFiles is how many files the run of the live row reads: with each text
// kept the run holds liveFiles times the cap, three caps, over liveCeiling's
// two; one text at a time it holds one cap, under it.
const liveFiles = 3

// liveRuns is how many runs the live row takes the least of. Each run reads
// files of the cap's size under the race detector, so it is fewer than
// allocRuns: a window can only be fuller than the run held, never emptier, so
// the least of a few holds the claim as the least of many does. The row's
// work is the claim and no more, so a slow runner is no reason for it to fail.
const liveRuns = 8

// liveCeiling is the most live heap a run of liveFiles files may hold at the
// close of any: two caps. One text, as the run holds it, is a cap.
const liveCeiling = 2 * coreapi.MaxRead

// liveHeap is the word the row's child is given.
const liveHeap = "live-heap"

// TestARunOverManyFilesJustUnderTheCapHoldsOneTextAtATime holds what the run
// keeps: each file is let go before the next is opened, so the
// live heap is one text under the cap and no total needs a bound. The
// control is one file, whose text the meter must see (a cap, so not a row
// that reads nothing) and that stays under the ceiling; the run is three,
// three caps if each text were kept in a slice, over the two the ceiling allows.
// The row takes the least of liveRuns runs, as the row above does of allocRuns: a busy
// machine can raise a window, never lower it, so a red with the texts let go
// is every window full of other tests' live data: run it again, and never
// raise the ceiling to quiet it.
//
// The live heap is the whole process's, so the measure is made in a rerun of
// this test binary that runs TestTheHelperRunsOnlyWhenRerun alone, where no
// parallel test's text is in the window; this row is parallel and holds no
// suppression. It fails with the child's output.
// Risk: a run keeps every text it read until the end, and holds the sum of
// the files in memory.
func TestARunOverManyFilesJustUnderTheCapHoldsOneTextAtATime(t *testing.T) {
	t.Parallel()
	code, out, err := configtest.Rerun(t.Context(), liveHeap)
	if err != nil || code != 0 {
		t.Fatalf("the child that measures the live heap ended %d, %v:\n%s", code, err, out)
	}
}

// TestTheHelperRunsOnlyWhenRerun is the rows' child: rerun with liveHeap it
// measures the live heap of the run above, and with allocated what the run
// over the planted file allocates; in a run go test started it is given
// nothing and does nothing.
func TestTheHelperRunsOnlyWhenRerun(t *testing.T) {
	t.Parallel() // in a rerun child this is the only test that runs
	args, ok := configtest.Helper()
	if !ok {
		return
	}
	switch {
	case len(args) == 1 && args[0] == liveHeap:
		measureLiveHeap(t)
	case len(args) == 1 && args[0] == allocated:
		least := leastAllocated(t)
		t.Logf("the least of %d runs allocated %d bytes; the ceiling is %d", allocRuns, least, allocCeiling)
		if least >= allocCeiling {
			t.Errorf("the least of %d runs allocated %d bytes, want under %d", allocRuns, least, allocCeiling)
		}
	default:
		t.Fatalf("the child was given %q, want %q or %q", args, liveHeap, allocated)
	}
}

// measureLiveHeap is the live row's measure, made in its child: one file's
// text is seen and held under liveCeiling, and so are liveFiles files' of
// each kind.
func measureLiveHeap(t *testing.T) {
	t.Helper()
	one, peak := liveAudit(t, 1, ".txt")
	control := leastPeak(t, one, peak)
	t.Logf("one file: the least of %d runs held %d live bytes; the ceiling is %d", liveRuns, control, liveCeiling)
	if control < coreapi.MaxRead-1 || control >= liveCeiling {
		t.Errorf("one file held %d live bytes, want from %d to under %d", control, coreapi.MaxRead-1, liveCeiling)
	}
	for _, ext := range []string{".txt", ".go"} {
		many, peak := liveAudit(t, liveFiles, ext)
		least := leastPeak(t, many, peak)
		t.Logf("%d %s files: the least of %d runs held %d live bytes; the ceiling is %d", liveFiles, ext, liveRuns, least, liveCeiling)
		if least >= liveCeiling {
			t.Errorf("%d %s files held %d live bytes, want under %d: a text is kept past its turn", liveFiles, ext, least, liveCeiling)
		}
	}
}

// TestAFileThatFailsInTheMiddleLeavesTheOnesBeforeCountedAndTheOnesAfterRead:
// b.go is tracked and gone from the work tree; a.go before it and c.go after
// it each hold a name and a new directive, and each is said by both checks,
// and the one that failed is said once, as a file that could not be read.
// Risk: a read that fails stops the run, or drops what it took of the files
// before it.
func TestAFileThatFailsInTheMiddleLeavesTheOnesBeforeCountedAndTheOnesAfterRead(t *testing.T) {
	t.Parallel()
	was := "package p\n\nfunc f() { g() }\n"
	now := "package p\n\nfunc f() { g() } //nolint:errcheck // secretname\n"
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.go": now, "b.go": now, "c.go": now},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.go": was, "b.go": was, "c.go": was}}}})
	names, err := format.ParseNames([]byte("secretname\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: names, Counting: linterCounting(),
		Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte(now)}, "c.go": &fstest.MapFile{Data: []byte(now)}}}
	r := coreapi.CheckRepo(context.Background(), a)
	var got []string
	for _, f := range r.Findings {
		got = append(got, f.Key+" "+f.File)
	}
	want := []string{audit.KeyDirective + " a.go", audit.KeyNames + " a.go", audit.KeyDirective + " c.go", audit.KeyNames + " c.go"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("findings %v, want %v", got, want)
	}
	if rows := notChecked(r, audit.KeyNames); len(rows) != 1 || !strings.Contains(rows[0].Why, "1 tracked files could not be read") {
		t.Errorf("names rows %v, want the one that says a file could not be read", rows)
	}
}

// statInfo is a Stat of a given mode and size.
type statInfo struct {
	mode fs.FileMode
	size int64
}

func (s statInfo) Name() string       { return "planted" }
func (s statInfo) Size() int64        { return s.size }
func (s statInfo) Mode() fs.FileMode  { return s.mode }
func (s statInfo) ModTime() time.Time { return time.Time{} }
func (s statInfo) IsDir() bool        { return s.mode.IsDir() }
func (s statInfo) Sys() any           { return nil }

// handlePipeFS is a work tree in which one name, pipe, opens as a handle whose
// Stat says a named pipe, while a Stat of the name says a regular file of
// five bytes: the handle and the name disagree, as they would for a file
// put in place of a FIFO between the two. A read of the handle fails t.
type handlePipeFS struct {
	fs.ReadFileFS
	t    *testing.T
	pipe string
}

func (p handlePipeFS) Open(name string) (fs.File, error) {
	if name == p.pipe {
		return handlePipeFile{t: p.t}, nil
	}
	return p.ReadFileFS.Open(name)
}

func (p handlePipeFS) Stat(name string) (fs.FileInfo, error) {
	if name == p.pipe {
		return statInfo{size: 5}, nil
	}
	return fs.Stat(p.ReadFileFS, name)
}

type handlePipeFile struct{ t *testing.T }

func (f handlePipeFile) Stat() (fs.FileInfo, error) {
	return statInfo{mode: fs.ModeNamedPipe, size: 5}, nil
}
func (f handlePipeFile) Close() error { return nil }
func (f handlePipeFile) Read([]byte) (int, error) {
	f.t.Error("the pipe's handle was read; a handle that is not a regular file is refused before any read")
	return 0, errors.New("a read of the pipe")
}

// unreadRow is whether a run counted n tracked files it could not read.
func unreadRow(r coreapi.RepoFindings, n int) bool {
	for _, row := range r.NotChecked {
		if strings.Contains(row.Why, fmt.Sprint(n)+" tracked files could not be read") {
			return true
		}
	}
	return false
}

// TestAHandleThatIsNotARegularFileIsUnreadWithNothingRead holds the first of
// the two promises the read brought into the engine from the adapter's
// ReadFile: the open
// handle's Stat decides, never a Stat of the name, and a handle that is not
// a regular file is counted in unread before any read. Here the name says a
// small regular file and the handle says a pipe; a read of the handle fails
// the test. With the check of the handle gone, or the name statted in its
// place, the handle is read and the row is red.
// Risk: a FIFO put where a tracked file was is read, and a run waits on it.
func TestAHandleThatIsNotARegularFileIsUnreadWithNothingRead(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"pipe.md": "hello"}, nil)
	a.Src = handlePipeFS{ReadFileFS: a.Src, t: t, pipe: "pipe.md"}
	r := coreapi.CheckRepo(context.Background(), a)
	if !unreadRow(r, 1) {
		t.Errorf("no row counts pipe.md in unread; rows %v", r.NotChecked)
	}
	if skippedRow(r, "pipe.md") {
		t.Errorf("pipe.md is said to be too large, want unread; rows %v", r.NotChecked)
	}
}

// partFS is a work tree in which one name, part, opens as a regular file of
// len(text) bytes whose reads give text and then end with end.
type partFS struct {
	fs.ReadFileFS
	part, text string
	end        error
}

func (p partFS) Open(name string) (fs.File, error) {
	if name == p.part {
		return &partFile{r: strings.NewReader(p.text), size: int64(len(p.text)), end: p.end}, nil
	}
	return p.ReadFileFS.Open(name)
}

type partFile struct {
	r    *strings.Reader
	size int64
	end  error
}

func (f *partFile) Stat() (fs.FileInfo, error) { return statInfo{size: f.size}, nil }
func (f *partFile) Close() error               { return nil }
func (f *partFile) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if errors.Is(err, io.EOF) {
		return 0, f.end
	}
	return n, err
}

// TestAReadThatFailsPartWayKeepsNothing holds the second promise: a file
// that gives some of its bytes and then an error is counted in unread and
// gives no document, never the part it gave. The bytes it gives hold a name
// from the private list, so a part kept is a names finding on part.md. The
// control ends the same bytes with io.EOF and finds the name. With the read's
// error dropped, the part is kept and the row is red.
// Risk: a check judges half a file as the whole and says it is clean.
func TestAReadThatFailsPartWayKeepsNothing(t *testing.T) {
	t.Parallel()
	names, err := format.ParseNames([]byte("zorbquux\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		end    error
		kept   bool
		unread int
	}{
		{"the control, read to its end", io.EOF, true, 0},
		{"a read that fails part way", errors.New("the disk went away"), false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := trackedAudit(t, map[string]string{"part.md": "x"}, nil)
			a.Src = partFS{ReadFileFS: a.Src, part: "part.md", text: "about zorbquux\n", end: tc.end}
			a.Names = names
			r := coreapi.CheckRepo(context.Background(), a)
			kept := false
			for _, f := range r.Findings {
				kept = kept || (f.Key == audit.KeyNames && f.File == "part.md")
			}
			if kept != tc.kept {
				t.Errorf("a names finding on part.md: %v, want %v; findings %v", kept, tc.kept, r.Findings)
			}
			if tc.unread > 0 && !unreadRow(r, tc.unread) {
				t.Errorf("no row counts part.md in unread; rows %v", r.NotChecked)
			}
			if tc.unread == 0 && unreadRow(r, 1) {
				t.Errorf("part.md is counted in unread, want it read; rows %v", r.NotChecked)
			}
		})
	}
}
