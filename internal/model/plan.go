package model

// The values of the write path (architecture 5, A35, A36): what a context
// decides, what a render makes of it, and the plan the two make together.
// They are data. The digest of a plan, the checks and the writes are
// internal/apply's.

// A Change is one entity or field that a plan changes, with its value before
// and after as text. A context decides it and knows nothing of bytes.
type Change struct {
	// Entity is the entity kind and its key, such as rule:store.schemas.2.
	Entity string
	// Field is the field that changes, "" when the entity itself is added or
	// removed.
	Field string
	// Before is the value now; "" when the entity or field is new.
	Before string
	// After is the value after the plan; "" when it goes away.
	After string
}

// A Delta is what a plan changes, in the order a person reads it. An empty
// Delta is a plan whose goal already holds (cli.commands.6).
type Delta struct {
	Changes []Change
}

// An Input is one file a plan read, and the digest it had. The set of them is
// the plan's input set (A44): the files it writes and every other file its
// result depended on.
type Input struct {
	// Path is relative to the root, slash-separated.
	Path string
	// Digest is the SHA-256 digest of the file's bytes as read. The empty
	// Digest says the file was not there, and must still not be.
	Digest Digest
}

// A Target is one file a plan writes, as format.Render makes it.
type Target struct {
	// Path is relative to the root, slash-separated, and inside the scope the
	// command names.
	Path string
	// Before is the digest of the bytes the write replaces or adds to; the
	// empty Digest says the file is not there yet.
	Before Digest
	// Data is the whole new content of the file, or, when Append is set, the
	// one line to add to it, without its newline.
	Data []byte
	// Append says the write adds Data as a line to the file instead of
	// replacing it. A file that is replaced is replaced whole.
	Append bool
	// Delete says the write takes the file away: Data is empty, Append is
	// not set and Before is the digest of the file to remove, which must be
	// there. The goal of a deleted file is that it is not there.
	Delete bool
}

// A Section is one part of the plan screen of spec 15.3: a heading and its
// lines.
type Section struct {
	Title string
	Lines []string
}

// A Goal is what must hold once the plan is applied: the digest each written
// file has afterwards. M0b-06 checks it; the write path carries it.
type Goal struct {
	Files []Input
}

// A Yes is the record of the yes a plan adds: a history line in a rulebook,
// a lock entry in a project (architecture 5.1). Who comes from git's
// configured identity and At from the Clock, both taken once at plan time and
// carried into the apply unchanged.
type Yes struct {
	// Who is the name and address the repository commits as.
	Who string
	// At is when the plan was made, in seconds since the Unix epoch, UTC.
	At int64
	// Command is the command that was agreed to, as a person types it.
	Command string
	// Plan is the digest of the plan the yes accepted. The write path sets it;
	// it is not part of the digest.
	Plan Digest
}

// A Plan is everything a changing command shows and then does: the delta, the
// files, the sections of spec 15.3, the input set, the goal and the digest
// over all of it (A36). The plan the dry run prints and the plan the apply
// runs are one value from one function (cli.commands.17).
type Plan struct {
	// Command is the command as a person types it.
	Command string
	Delta   Delta
	// Files are the data files, written in this order and before the record.
	Files    []Target
	Sections []Section
	// Inputs is the digest of every file the plan read.
	Inputs []Input
	Goal   Goal
	// Yes is the yes the plan adds; nil for a plan that adds none, such as a
	// report written to a file.
	Yes *Yes
	// Record is the target that holds the yes, written last; nil with Yes.
	// Its Data holds the plan digest, so the digest leaves it out.
	Record *Target
	// Digest is the canonical digest of the plan, which internal/apply sets.
	Digest Digest
}
