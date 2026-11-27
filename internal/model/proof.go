package model

// RunLevel is how a proof run was held apart from the machine it ran on (spec
// 16.1, S23): the run level the grid records for each cell.
type RunLevel string

// The levels of a run, as the contract's proof result names them.
const (
	RunFixture RunLevel = "fixture"
	RunPair    RunLevel = "pair"
	RunProject RunLevel = "project"
)

// RunLevels are the levels of a run, in the order of the contract.
func RunLevels() []RunLevel { return []RunLevel{RunFixture, RunPair, RunProject} }

// Reason is why a cell was not run, as the contract's proof result names it.
type Reason string

// The reasons a cell was not run.
const (
	ReasonNoPair      Reason = "no pair"
	ReasonToolMissing Reason = "tool missing"
	ReasonLimitHit    Reason = "limit hit"
	// ReasonSandboxMissing is the sandboxed level asked for and the machine has
	// no sandbox to give (port.ErrUnavailable from Prepare).
	ReasonSandboxMissing Reason = "sandbox missing"
)

// Reasons are the reasons a cell was not run, in the order of the contract.
func Reasons() []Reason {
	return []Reason{ReasonNoPair, ReasonToolMissing, ReasonLimitHit, ReasonSandboxMissing}
}

// Cell is one cell of the proof grid: the result of one binding under one
// tool version. A cell is never blank: it names its binding, its tool version
// and its result, and it carries the level it ran at. The grid holds no time
// and no host name or path (S57).
type Cell struct {
	// Binding is the text of the binding the cell is for, which the grid's
	// reader never looks inside: it is matched whole to the text of a
	// binding's key.
	Binding string
	// ToolVersion is the version of the tool the binding ran under.
	ToolVersion string
	// Result is pass, fail, not run or reviewed. A rung-6 cell is never pass
	// or fail (S9).
	Result Truth
	// Rung is the rung of the binding, 0 to 6.
	Rung int
	// Level is the run level the cell was proven at.
	Level RunLevel
	// Adapter is the version of the adapter that ran the tool, or empty for a
	// run no adapter made.
	Adapter string
	// Reason is why the cell was not run, and empty for every other result.
	Reason Reason
	// Inputs is the digest of what the cell was proven on. Decides: whether the
	// cell is run again; it keeps its result only while its inputs equal the
	// digest of what it would be proven on now. Empty is no digest, and a cell
	// with none is always run again.
	Inputs Digest
	// Tool is the tool the binding ran under, which ToolVersion alone does not
	// name. Display only.
	Tool string
}

// ProofGrid is the proof grid document: its cells, in the order it holds
// them, and the set of versions the rulebook was last proven on, where the
// grid names one. Nothing in it is a time or a host path (S57).
type ProofGrid struct {
	Cells []Cell
	// TestedSet is that set, or nil. Display only.
	TestedSet *Tested
}

// CellKey is what a cell is found by: its binding and its tool version. A
// grid holds one cell for a key.
type CellKey struct {
	Binding     string
	ToolVersion string
}

// Key is the key of c.
func (c Cell) Key() CellKey { return CellKey{Binding: c.Binding, ToolVersion: c.ToolVersion} }
