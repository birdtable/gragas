package plugin

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// The step caps are declared here and judged in two other places, each with
// its own at-and-over tests: core/wire's codec, through its aliases of these
// constants, judges what a plugin sends, and host/tool's step runner judges a
// run and a read at the read. This package judges nothing but what its
// generated decoder holds, so each test here holds a cap to the value the
// design gives it, written out, and the decoder's own bound where it has one.
// Moving a cap a step turns its test red. The judges' tests take the cap from
// here, so they would follow the move and stay green.

// schemaPath is the plugin contract's wire schema, the source of the
// generated decoder.
const schemaPath = "../../../contract/plugin/v1/wire.schema.json"

// resultOf is a read's result giving n files, each empty.
func resultOf(n int) canon.Value {
	files := canon.Array{}
	for i := range n {
		files = append(files, canon.Object{"path": canon.String("f" + strconv.Itoa(i)), "data": canon.Object{"text": canon.String("")}})
	}
	return canon.Object{"round": canon.Int(0), "step": canon.Int(0), "kind": canon.String("read"), "files": files}
}

// TestAReadsResultIsHeldAtMaxReadFilesByTheDecoderAndTheSchema
// (boundary: MaxReadFiles): the generated decoder takes a result of 256 files
// and refuses one of 257 at /files, and the schema it is generated from
// bounds a result's files at MaxReadFiles, so the codec, the step runner and
// the schema give a read the same number of files. Planted: MaxReadFiles at
// 255 and at 257.
// Risk: a read or a glob that gives back a whole tree.
func TestAReadsResultIsHeldAtMaxReadFilesByTheDecoderAndTheSchema(t *testing.T) {
	t.Parallel()
	if MaxReadFiles != 256 {
		t.Errorf("MaxReadFiles = %d, want 256", MaxReadFiles)
	}
	var at Result
	if err := at.FromCanon(resultOf(MaxReadFiles)); err != nil || len(at.Files) != MaxReadFiles {
		t.Errorf("a result of %d files: %d files, %v; want it taken", MaxReadFiles, len(at.Files), err)
	}
	var over Result
	err := over.FromCanon(resultOf(MaxReadFiles + 1))
	if f, ok := errors.AsType[*Fault](err); !ok || f.At != "/files" {
		t.Errorf("a result of %d files: %v, want a *Fault at /files", MaxReadFiles+1, err)
	}
	b, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	v, err := canon.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	node := v
	for _, k := range []string{"$defs", "result", "properties", "files", "maxItems"} {
		o, ok := node.(canon.Object)
		if !ok {
			t.Fatalf("%s has no /$defs/result/properties/files/maxItems: %s is not an object", schemaPath, k)
		}
		node = o[k]
	}
	if node != canon.Int(MaxReadFiles) {
		t.Errorf("the schema bounds a result's files at %v, want MaxReadFiles = %d", node, MaxReadFiles)
	}
}

// TestMaxStreamIsTheEightMiBItsJudgesHold (boundary: MaxStream): one stream
// of a run is 8 MiB. core/wire's
// TestTheStreamStdinAndFileCapsAreHeldAtTheBoundary takes a stream at it and
// refuses one a byte over, and host/tool's TestAStreamIsCutAtItsCap cuts a run
// printing past it at exactly that many bytes. Planted: 8<<20 - 1 and
// 8<<20 + 1.
// Risk: one step's output taking the memory of the engine.
func TestMaxStreamIsTheEightMiBItsJudgesHold(t *testing.T) {
	t.Parallel()
	if MaxStream != 8388608 {
		t.Errorf("MaxStream = %d, want 8 MiB, 8388608", MaxStream)
	}
}

// TestMaxStdinIsTheOneMiBItsJudgesHold (boundary: MaxStdin): a run's stdin is
// 1 MiB. core/wire's TestTheStreamStdinAndFileCapsAreHeldAtTheBoundary
// decodes a plan whose stdin is at it and refuses one a byte over, and
// host/tool's TestAStepRunsItsWordsAndItsStdin runs a step at it and refuses
// one past it with nothing started. Planted: 1<<20 - 1 and 1<<20 + 1.
// Risk: one step's input taking the memory of the engine.
func TestMaxStdinIsTheOneMiBItsJudgesHold(t *testing.T) {
	t.Parallel()
	if MaxStdin != 1048576 {
		t.Errorf("MaxStdin = %d, want 1 MiB, 1048576", MaxStdin)
	}
}

// TestMaxFileIsTheOneMiBItsJudgesHold (boundary: MaxFile): one file's data is
// 1 MiB. core/wire's TestTheStreamStdinAndFileCapsAreHeldAtTheBoundary takes a
// config file at it and refuses one a byte over, and host/tool's
// TestAReadIsHeldToItsCapsAtTheRead reads a file at it and refuses one a byte
// over. Planted: 1<<20 - 1 and 1<<20 + 1.
// Risk: one file of a read taking the memory of the engine.
func TestMaxFileIsTheOneMiBItsJudgesHold(t *testing.T) {
	t.Parallel()
	if MaxFile != 1048576 {
		t.Errorf("MaxFile = %d, want 1 MiB, 1048576", MaxFile)
	}
}

// TestMaxFilesIsTheEightMiBItsJudgesHold (boundary: MaxFiles): the files of
// one read are 8 MiB in all, a whole number of files at MaxFile, which both
// judges' at-the-cap rows are built from. core/wire's
// TestTheFilesOfOneReadAreHeldAtTheBoundary takes eight files of 1 MiB and
// refuses a byte more, and host/tool's TestAReadIsHeldToItsCapsAtTheRead
// reads 8 MiB in all and refuses one byte past it. Planted: 8<<20 - 1 and
// 8<<20 + 1.
// Risk: a read or a glob that gives back a whole tree.
func TestMaxFilesIsTheEightMiBItsJudgesHold(t *testing.T) {
	t.Parallel()
	if MaxFiles != 8388608 {
		t.Errorf("MaxFiles = %d, want 8 MiB, 8388608", MaxFiles)
	}
	if MaxFiles < MaxFile || MaxFiles%MaxFile != 0 {
		t.Errorf("MaxFiles = %d is not a whole number of files at MaxFile = %d", MaxFiles, MaxFile)
	}
}
