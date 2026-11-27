package sandbox

import "io/fs"

// Judge is judge, for the owner a test cannot plant without root.
func Judge(path, owner, me string) error { return judge(path, owner, me) }

// Owned is owned, for a description of a folder the test hands it.
func Owned(path string, fi fs.FileInfo, mounts []string) error { return owned(path, fi, mounts) }

// Unheld is unheld, for a refusal of trust.Held a test cannot plant without
// mounting a disk.
func Unheld(path string, err error) error { return unheld(path, err) }

// Untrusted is untrusted, for a refusal of the walk a test cannot plant
// without mounting a disk.
func Untrusted(dir string, err error) error { return untrusted(dir, err) }
