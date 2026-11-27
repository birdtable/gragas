package model

// PluginRef names one installed plugin: the folder name of its bundle and the
// plugin's own version (plugins/<name>/<version>/). It is what
// port.Plugins lists and opens; the tool a plugin serves is its manifest's, so
// a ref names none.
type PluginRef struct {
	// Name is the bundle's folder name. Decides: which plugin Open reads.
	Name string
	// Version is the plugin's own version. Decides: which of a name's
	// versions Open reads.
	Version string
}
