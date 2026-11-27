package model

// What the history, the cache and the journal answer, as data. The core fills
// them and the commands draw them; they live here so the kit, which a command
// package reaches the run through, can name them (cli.commands.24).

// HistoryAsk is what a history listing is asked for.
type HistoryAsk struct {
	// Root is the repository.
	Root Root
	// Rev is the tag, commit or branch to read the file at; "" reads the
	// working tree.
	Rev string
	// Rule is a rule id or one of its aliases; only the entries about the rule
	// are listed, under any id it has had. "" asks for all.
	Rule string
	// Entity is an entity as the entries write it, such as rule:go.errors.1;
	// only the entries about it are listed. "" asks for all.
	Entity string
}

// HistoryListing is the answer. A list is never null: with nothing in it, it
// is empty.
type HistoryListing struct {
	Schema int `json:"schema"`
	// Source says where the file was read: the working tree, or the rev.
	Source string `json:"source"`
	// File is the path of the file below the root.
	File string `json:"file"`
	// Total is how many entries the file holds, before the filters.
	Total int `json:"total"`
	// Next is the number the next entry takes.
	Next     int              `json:"next"`
	Entries  []HistoryRow     `json:"entries"`
	Skipped  []HistorySkipped `json:"skipped"`
	Findings []HistoryFinding `json:"findings"`
	// Notes say what a person should know about the listing: the file is not
	// there, or an id is not a rule of the book.
	Notes []string `json:"notes"`
}

// HistoryRow is one entry. Every row has every field: one that does not apply
// is null (cli.commands.24, cli.output.9).
type HistoryRow struct {
	Number   int        `json:"number"`
	Previous *string    `json:"previous"`
	Kind     string     `json:"kind"`
	Entity   string     `json:"entity"`
	Before   *Lifecycle `json:"before"`
	After    Lifecycle  `json:"after"`
	Who      string     `json:"who"`
	When     string     `json:"when"`
	Plan     string     `json:"plan"`
	Decision *string    `json:"decision"`
}

// HistorySkipped is a line that could not be read, and why.
type HistorySkipped struct {
	Line int    `json:"line"`
	Why  string `json:"why"`
}

// HistoryFinding is a broken link or a gap in the chain.
type HistoryFinding struct {
	Kind   string `json:"kind"`
	Number int    `json:"number"`
	Line   int    `json:"line"`
	Msg    string `json:"msg"`
}

// CacheListing is the answer: one row a kind, in the order asked.
type CacheListing struct {
	Schema int            `json:"schema"`
	Keep   string         `json:"keep"`
	Kinds  []CacheKindRow `json:"kinds"`
}

// CacheKindRow is one kind. A number that cannot be told is null, and the
// note beside it says why (cli.output.9).
type CacheKindRow struct {
	Kind  string `json:"kind"`
	Holds string `json:"holds"`
	Built bool   `json:"built"`
	// Files is how many files the kind holds.
	Files int `json:"files"`
	// Size is their length in bytes.
	Size     *int64 `json:"size"`
	SizeNote string `json:"size_note,omitempty"`
	// Oldest is when the oldest closed run ended, in seconds since the Unix
	// epoch, UTC; Age is how old that is, in seconds, by the clock.
	Oldest     *int64 `json:"oldest"`
	Age        *int64 `json:"age"`
	OldestNote string `json:"oldest_note,omitempty"`
	// Open is how many runs have no close. They are evidence: clear never
	// removes one, whatever its age.
	Open *int `json:"open,omitempty"`
	// ClearedAfter is the keep setting, the age at which clear removes an
	// entry when it is not named a kind.
	ClearedAfter string `json:"cleared_after"`
}

// ClearPlan is what a clear would do: the closed runs older than the keep
// age, the closed runs it keeps, and the open runs it never touches.
type ClearPlan struct {
	Schema int `json:"schema"`
	// Runs are the ids of the runs it would remove, oldest first.
	Runs   []string `json:"runs"`
	Remove int      `json:"remove"`
	Keep   int      `json:"keep"`
	Open   int      `json:"open"`
	// Scratch is how many scratch folders a stopped run left, which a clear
	// removes with the runs.
	Scratch int `json:"scratch"`
	// Tools is how many folders of a grant's caches, kept from one step to
	// the next, a clear removes with them.
	Tools int `json:"tools"`
}

// Cleared is how a clear ended.
type Cleared struct {
	Schema  int  `json:"schema"`
	Word    Word `json:"word"`
	Removed int  `json:"removed"`
	// Scratch is how many scratch folders it removed.
	Scratch int `json:"scratch"`
	// Tools is how many folders of a grant's caches it removed.
	Tools int `json:"tools"`
	// Observed says why a clear ended busy.
	Observed string `json:"observed,omitempty"`
}

// JournalRun is one run, as its records say.
type JournalRun struct {
	Run     string `json:"run"`
	Command string `json:"command"`
	// Plan is the digest of the plan the run carried out.
	Plan string `json:"plan"`
	// State is "open" for a run with no close, and otherwise the outcome its
	// close records: done, rolled back, completed, failed or a refusal.
	State string `json:"state"`
	// Started and Ended are seconds since the Unix epoch, UTC; Ended is null
	// for an open run.
	Started *int64 `json:"started"`
	Ended   *int64 `json:"ended"`
	// Files are the paths the run kept a copy of, in the order it did.
	Files []string `json:"files"`
}

// JournalListing is the runs of one root, oldest first.
type JournalListing struct {
	Schema int `json:"schema"`
	// Root is the digest of the root the journal is of, "" when it has no
	// record.
	Root string `json:"root"`
	// Torn is how many lines were skipped as torn.
	Torn int          `json:"torn"`
	Runs []JournalRun `json:"runs"`
}

// RecoverAsk is the run id the recovery is filed under and the engine's
// version, for the journal.
type RecoverAsk struct{ Run, Engine string }

// Recovered is one open run a recovery settled.
type Recovered struct {
	Run     string `json:"run"`
	Command string `json:"command"`
	// Completed says the completion finished the run; otherwise it was
	// rolled back.
	Completed bool `json:"completed"`
}

// Recovery is how a recover ended.
type Recovery struct {
	Schema    int         `json:"schema"`
	Word      Word        `json:"word"`
	Run       string      `json:"run"`
	Recovered []Recovered `json:"recovered"`
	// Observed and Hint say why a refused or busy run ended so, and Why what
	// a failed run could not do.
	Observed string `json:"observed,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Why      string `json:"why,omitempty"`
}

// RestoreAsk is the run to restore and the state folder the journal keeps its
// copies in.
type RestoreAsk struct{ Run, StateDir string }

// Moved is a file that does not hold what the run left, so putting its copy
// back would undo something else.
type Moved struct {
	Path string `json:"path"`
	// Now is the digest the file holds, "" when it is gone; Expected is the
	// digest the run left.
	Now      string `json:"now"`
	Expected string `json:"expected"`
	// By are the later runs that kept a copy of the file, oldest first. None
	// means the change was made outside gragas.
	By []string `json:"by"`
}

// Damaged is a file whose kept copy is missing or does not hold the bytes the
// copy record names, so a cp of it would put the wrong bytes back. Got is the
// digest the copy holds, "" when it could not be read.
type Damaged struct {
	Path     string `json:"path"`
	Copy     string `json:"copy"`
	Expected string `json:"expected"`
	Got      string `json:"got"`
}

// RestoreFile is one file a restore puts back: "restore" its copy, or "remove"
// the file the run made. It holds as data what its line says, so a reader
// of --json never parses the line.
type RestoreFile struct {
	// Path is where the file is written back, or the file removed.
	Path string `json:"path"`
	Op   string `json:"op"`
	// From is the kept copy a restore writes to Path, "" for a remove.
	From string `json:"from"`
	// Digest is the digest of the file before the run, "" for a file it made.
	Digest string `json:"digest"`
}

// Restoration is what a restore of one run says.
type Restoration struct {
	Schema int           `json:"schema"`
	Run    string        `json:"run"`
	Root   string        `json:"root"`
	Files  []RestoreFile `json:"files"`
	// Moved and Damaged are not empty when the restore is refused; Lines is
	// then empty.
	Moved   []Moved   `json:"moved"`
	Damaged []Damaged `json:"damaged"`
	// Lines are the lines to type, in the folder of the root, that put each
	// file back, newest first. They are written for a POSIX shell only, so
	// in PowerShell or cmd Lines is empty and Files says each step.
	Lines []string `json:"lines"`
}
