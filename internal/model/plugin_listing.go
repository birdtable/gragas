package model

// PluginRow is one installed plugin as `plugin list` shows it.
type PluginRow struct {
	// Name and Version are the lock's own. Decides: which plugin the row is.
	Name    string `json:"name"`
	Version string `json:"version"`
	// Tool is the manifest's tool, "" when the manifest was not read whole
	// (Note says why). Display only.
	Tool string `json:"tool"`
	// Digest is the digest of the bundle as it lies on disk now. Decides:
	// approval, which is by this digest alone.
	Digest Digest `json:"digest"`
	// Approved is whether Digest was approved on this machine. A bundle that
	// changed since it was locked has another digest and is not.
	Approved bool `json:"approved"`
	// Note says what is wrong with the row, "" for nothing: the bundle
	// changed since the lock, or its manifest is refused. Display only.
	Note string `json:"note"`
}

// PluginsListing is the plugins installed under the working folder, by name
// and then version as bytes; none is a nil list.
type PluginsListing struct {
	Rows []PluginRow `json:"rows"`
}
