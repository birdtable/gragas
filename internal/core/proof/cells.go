package proof

import (
	"bytes"
	"cmp"
	"slices"
	"strconv"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
)

// Inputs is the digest of everything the cell of c under j is a result of: the
// pair's digest, the binding's text, the tool, its version and the adapter's,
// the config set (each file's path and bytes, in path order), the path the
// tool is told to read in it, and, for a case whose examples are placed, the
// path and pair digest of every placed case of j, in j's order. Two cells with
// one Inputs were proven on the same inputs, so the later keeps the earlier's
// result; a change to any one of them is another digest, and the cell is run
// again.
//
// The placed cases of a job are in the key because a cell's examples are run
// in one folder with theirs (runBatches), and a check that reads past one file
// (unused names, imports, a package's declarations) answers on the folder, not
// on the file. Which folder a case lands in depends on the paths and the order
// of the others, so the key holds them all and their order, not the folder
// alone. A review and a case with no pair place nothing, so they hold none and
// stand in none's key.
//
// Each part is framed by its length, so no two lists of parts make one text:
// bytes moved from one part into the next change the digest.
func Inputs(j Job, c Case) model.Digest {
	var b bytes.Buffer
	part := func(s string) {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
	}
	part(string(c.Pair.Digest))
	part(BindingText(c.Binding.Ref(c.Rule)))
	part(j.Tool)
	part(j.Version)
	part(j.Adapter)
	part(j.ConfigPath)
	files := slices.SortedFunc(slices.Values(j.Config), func(x, y model.File) int { return cmp.Compare(x.Path, y.Path) })
	part(strconv.Itoa(len(files)))
	for _, f := range files {
		part(f.Path)
		part(string(f.Data))
	}
	var beside []Case
	if places(c) {
		beside = slices.DeleteFunc(slices.Clone(j.Cases), func(m Case) bool { return !places(m) })
	}
	part(strconv.Itoa(len(beside)))
	for _, m := range beside {
		part(m.Binding.PairPath)
		part(string(m.Pair.Digest))
	}
	return model.Digest(digest.Prefixed(b.Bytes()))
}

// places is whether runJob places c's examples in a folder and runs the tool
// on them: a binding below the reviewed rung, with a pair path.
func places(c Case) bool {
	return c.Binding.Rung != model.MaxRung && c.Binding.PairPath != ""
}
