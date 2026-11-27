package wire

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// edge holds one cap at its boundary: the value that is exactly at the cap is
// taken, and the one a byte (or a level, or a file) over is refused.
func edge(t *testing.T, name string, at, over func() error) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		if err := at(); err != nil {
			t.Errorf("at the cap: %v", err)
		}
		if err := over(); err == nil {
			t.Error("one over the cap was taken")
		}
	})
}

// TestTheRequestAndResponseCapsAreHeldAtTheBoundary holds the two size caps of
// a message: a request is taken at 32 MiB and refused a byte over, a response
// likewise at 16 MiB (a *plugin.LimitError each).
// boundary: MaxRequest
// boundary: MaxResponse
// Risk: a plugin, or the engine, made to hold a message of any size in memory.
func TestTheRequestAndResponseCapsAreHeldAtTheBoundary(t *testing.T) {
	t.Parallel()
	c := codec(t)
	pad := func(n int) canon.Object { return canon.Object{"x": canon.String(strings.Repeat("a", n))} }
	base, err := c.EncodeRequest(planRequest(pad(0)))
	if err != nil {
		t.Fatal(err)
	}
	room := MaxRequest - len(base)
	edge(t, "request", func() error { _, err := c.EncodeRequest(planRequest(pad(room))); return err },
		func() error { _, err := c.EncodeRequest(planRequest(pad(room + 1))); return err })
	// A response is padded with white space, which JSON allows anywhere.
	reply := func(n int) []byte {
		return []byte(`{"ok":{"words":[]}}` + strings.Repeat(" ", n-len(`{"ok":{"words":[]}}`)))
	}
	edge(t, "response", func() error { _, err := c.DecodeRank(reply(MaxResponse), 0); return err },
		func() error {
			_, err := c.DecodeRank(reply(MaxResponse+1), 0)
			if _, isLimit := errors.AsType[*plugin.LimitError](err); !isLimit {
				t.Errorf("one over the response cap: %T %v, want a *plugin.LimitError", err, err)
			}
			return err
		})
}

// TestTheStreamStdinAndFileCapsAreHeldAtTheBoundary holds a stream of a run
// (8 MiB), the stdin of a run (1 MiB) and a config file's data (1 MiB): each is
// taken at its cap and refused a byte over.
// boundary: MaxStream
// boundary: MaxStdin
// boundary: MaxFile
// Risk: one step's output, input or file taking the memory of the engine.
func TestTheStreamStdinAndFileCapsAreHeldAtTheBoundary(t *testing.T) {
	t.Parallel()
	c := codec(t)
	text := func(n int) string { return strings.Repeat("a", n) }
	stream := func(n int) error {
		_, err := c.EncodeRequest(planRequest(haveOf(`{"round":0,"step":0,"kind":"read","stdout":{"text":"` + text(n) + `"},"files":[]}`)))
		return err
	}
	edge(t, "stream", func() error { return stream(MaxStream) }, func() error { return stream(MaxStream + 1) })
	stdin := func(n int) error {
		_, err := c.DecodePlan(run(`,"stdin":{"text":"`+text(n)+`"}`), 0, grantsOf())
		return err
	}
	edge(t, "stdin", func() error { return stdin(MaxStdin) }, func() error { return stdin(MaxStdin + 1) })
	file := func(n int) error {
		_, err := c.EncodeRequest(planRequest(canon.Object{"config": canon.Array{canon.Object{"path": canon.String("a"), "data": canon.Object{"text": canon.String(text(n))}}}}))
		return err
	}
	edge(t, "file", func() error { return file(MaxFile) }, func() error { return file(MaxFile + 1) })
}

// TestTheFilesOfOneReadAreHeldAtTheBoundary holds the files of one read: 256
// of them, and 8 MiB in all, are taken, and a file or a byte over is refused.
// boundary: MaxReadFiles
// boundary: MaxFiles
// Risk: a read or a glob that gives back a whole tree.
func TestTheFilesOfOneReadAreHeldAtTheBoundary(t *testing.T) {
	t.Parallel()
	c := codec(t)
	file := func(i, n int) string {
		return `{"path":"f` + strconv.Itoa(i) + `","data":{"text":"` + strings.Repeat("a", n) + `"}}`
	}
	have := func(files ...string) error {
		_, err := c.EncodeRequest(planRequest(haveOf(`{"round":0,"step":0,"kind":"read","files":[` + strings.Join(files, ",") + `]}`)))
		return err
	}
	count := func(n int) error {
		var fs []string
		for i := range n {
			fs = append(fs, file(i, 0))
		}
		return have(fs...)
	}
	edge(t, "files", func() error { return count(MaxReadFiles) }, func() error { return count(MaxReadFiles + 1) })
	total := func(extra int) error {
		var fs []string
		for i := range MaxFiles / MaxFile {
			fs = append(fs, file(i, MaxFile))
		}
		if extra > 0 {
			fs = append(fs, file(len(fs), extra))
		}
		return have(fs...)
	}
	edge(t, "total", func() error { return total(0) }, func() error { return total(1) })
}

// TestAnOpaqueValueIsHeldAtItsBoundary holds a value: 64 KiB in canonical form
// and 32 levels deep are taken, one byte and one level over are refused.
// boundary: MaxValue
// boundary: MaxValueDepth
// Risk: a value that costs the engine more to hold than the contract allows.
func TestAnOpaqueValueIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	c := codec(t)
	value := func(v string) error {
		_, err := c.DecodeEntries(ok(`{"entries":[{"key":"a","on":true,"value":` + v + `}]}`))
		return err
	}
	// A string is its length and two quotes in canonical form.
	size := func(n int) string { return `"` + strings.Repeat("a", n-2) + `"` }
	edge(t, "size", func() error { return value(size(MaxValue)) }, func() error { return value(size(MaxValue + 1)) })
	edge(t, "depth", func() error { return value(nested(MaxValueDepth)) }, func() error { return value(nested(MaxValueDepth + 1)) })
}
