package model

// PluginBundle is one installed plugin's bundle as port.Bundles read it: the
// digests the opener holds the files to, the two files it judges and runs, and
// every other file of the folder, which the digest covers. It is bytes and digests only,
// and the opener makes every refusal on them; a reader refuses only what it
// cannot read.
type PluginBundle struct {
	// Locked is the bundle's digest as the lock holds it, in the digest's
	// form. Decides: the digest approval is asked for, and the one the sum of
	// the files here must equal for the bundle to open.
	Locked Digest
	// Read is the digest of the bundle as the reader took it, in the
	// digest's form, over the very files here, from one read. Decides:
	// nothing on its own: the opener sums the files itself and refuses a Read
	// that is not that sum, so a reader that reports another digest never
	// opens a bundle.
	Read Digest
	// Manifest is the bytes of plugin.yaml, whole and unparsed. Decides:
	// the manifest the opener reads, judges and shows.
	Manifest []byte
	// Module is the bytes of plugin.wasm, whole. Decides: the module the
	// opener holds to the manifest's wasm digest, compiles and runs.
	Module []byte
	// Other is every other regular file of the bundle's folder, testdata/
	// among them, each by its path below the folder, slash-separated, in the
	// order of its path as bytes; none is nil. Decides: nothing the opener
	// runs, only the digest, so a file added after the lock is refused.
	Other []File
}
