package model

// The requests of the import steps: a guide a companion rulebook points at,
// and what is drafted from it. The guide is data from a repository the owner
// does not control (S50): it is read, never run, and none of its prose is
// copied beyond the one line a maintainer edits afterwards.

// GuideRef names a guide at one release.
type GuideRef struct {
	// Slug is the key of the source that records the guide.
	Slug string
	// Title names the guide.
	Title string
	// URL is where the guide is found.
	URL string
	// Tag is the release of the guide the drafts are read at.
	Tag string
	// Attribution says who wrote the guide; "" for none.
	Attribution string
}

// DraftGuide is the request of the draft step: the rules of one guide at its
// tag, drafted into one concern of one part.
type DraftGuide struct {
	// Part and Concern are the slugs the drafted ids are made of.
	Part, Concern string
	// Guide is the guide the drafts cite.
	Guide GuideRef
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// ImportEntry is one entry of a project's config as the import reads it: the
// config file it stands in and the entry as the tool keeps it, with the
// severity the adapter's scale says it stands for. It is data read from a file
// the owner wrote.
type ImportEntry struct {
	// Config is the path of the config file, relative to the root.
	Config string
	// Key, Value and Options are the entry as ToolEntry holds them.
	Key, Value, Options string
	// Severity is the engine's severity the tool's word stands for; an entry
	// that names none stands at error.
	Severity Severity
}

// ImportRequest is the request of import: the drafts of a guide, and the
// entries of the configs of one tool for one target type that the drafts are
// matched to. A draft with an entry enters active, grandfathered; one with
// none enters proposed.
type ImportRequest struct {
	// Draft is the guide and where its drafts go.
	Draft DraftGuide
	// Tool and Target are the tool the configs are of and what they are about.
	Tool   string
	Target TargetType
	// Entries are the entries of the configs, in file order.
	Entries []ImportEntry
}

// ImportInput is what a person gives import: the guide's folder and where it
// is drafted to, the configs to bring in and the tool they are of. Paths are
// relative to the root. The guide and the configs are read by the command, so
// the plan is made from what the files hold when it is made.
type ImportInput struct {
	// Part, Concern and Guide are as in DraftGuide.
	Part, Concern string
	Guide         GuideRef
	// GuideDir is the folder of the guide's Markdown files; "" for no guide.
	GuideDir string
	// Tool and Target say which tool the configs are of and what they are about.
	Tool   string
	Target TargetType
	// Configs are the config files, in the order their entries are read.
	Configs []string
}
