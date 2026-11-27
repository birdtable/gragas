package model

// PluginGrant is one program a plugin may ask the engine to run, as `plugin
// approve` shows it before the yes. Every text is the
// manifest's own, in the form the screen draws: the core escapes it.
type PluginGrant struct {
	// ID is the grant's name: what a run asks for it by.
	ID string `json:"id"`
	// Where is how its program is found: "pinned" for the pinned places
	// alone, "path" for those and one place below the project, "script" for an
	// interpreted script a package manifest names.
	Where string `json:"where"`
	// Path is the place below the project, for Where "path"; else "".
	Path string `json:"path"`
	// Manifest, ScriptAt and Interpreter are a script's package manifest, the
	// JSON pointer of its path there, and the program that runs it; else "".
	Manifest    string `json:"manifest"`
	ScriptAt    string `json:"script_at"`
	Interpreter string `json:"interpreter"`
	// Versions is the range of versions the program is held to; VersionArgv
	// and VersionPattern say how its version is asked and read ("" for the
	// default pattern).
	Versions       string   `json:"versions"`
	VersionArgv    []string `json:"version_argv"`
	VersionPattern string   `json:"version_pattern"`
	// WholeLine is true for a grant with no script: the plugin hands the
	// program the whole command line, and the screen says so.
	WholeLine bool `json:"whole_line"`
	// CacheVars is the variables the program reads where to keep its cache
	// by, each of which the engine points at a folder of its own choosing; empty for none. A trust input as resolve_from is: the
	// screen shows them, and the yes covers them.
	CacheVars []string `json:"cache_vars"`
}

// PluginApproval is what an approval of one installed plugin covers, read from
// its manifest alone: the screen before the yes, and the
// record of it once given. A manifest that is refused has none.
type PluginApproval struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Digest is the bundle's digest as it lies on disk: the one thing an
	// approval is of.
	Digest Digest `json:"digest"`
	// GrantsDigest is the digest of the grants, kept with the approval.
	GrantsDigest Digest `json:"grants_digest"`
	// Held is the grants one by one, by id and digest, sorted: what Approve
	// records and a later screen compares with the prior approval's. Not in
	// the document.
	Held []GrantHeld `json:"-"`
	// Since is how the grants compare with the plugin's prior approval.
	Since GrantsSince `json:"since"`
	Tool  string      `json:"tool"`
	// ToolVersions is the tool versions the plugin serves.
	ToolVersions string        `json:"tool_versions"`
	Grants       []PluginGrant `json:"grants"`
	// Preset is the path the preset is written to, "" for none; ResolveFrom the
	// folder a run links in, "" for none; Install the tool's install words,
	// which the engine prints and never runs.
	Preset      string   `json:"preset"`
	ResolveFrom string   `json:"resolve_from"`
	Install     []string `json:"install"`
	// Approved is whether Digest is approved on this machine now. Recorded is
	// true on the answer of a run that wrote the approval.
	Approved bool `json:"approved"`
	Recorded bool `json:"recorded"`
}

// Grant changes, as a later approval is compared with the plugin's prior one
// by the grants' ids and digests.
const (
	GrantAdded   = "added"
	GrantRemoved = "removed"
	// GrantChanged is a grant whose id is in both and whose digest is not:
	// its scope or its program changed in place.
	GrantChanged = "changed"
)

// GrantChange is one grant that differs between the prior approval of a
// plugin and the bundle in hand: Change is GrantAdded, GrantRemoved or
// GrantChanged. ID is shown text.
type GrantChange struct {
	ID     string `json:"id"`
	Change string `json:"change"`
}

// GrantsSince is how the grants of a bundle compare with the newest approval
// of a plugin of that name: what `plugin approve` shows beside the grants.
type GrantsSince struct {
	// Prior is the reference the prior approval was shown by, as shown text;
	// "" for a first approval, which has none.
	Prior string `json:"prior"`
	// Recorded is false for a prior approval that kept no grants by id
	// and for a first approval: no comparison was made, and the screen shows
	// every grant.
	Recorded bool `json:"recorded"`
	// Changes is the grants added, removed or changed, sorted by id; empty
	// when Recorded and nothing changed, and always empty when not Recorded.
	Changes []GrantChange `json:"changes"`
}
