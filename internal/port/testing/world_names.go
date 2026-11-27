package porttest

import "github.com/birdtable/go-files/confine"

// fileNamesPort is the fake of port.FileNames. It holds no world: a name's key
// is seeded by nothing and read from no disk.
type fileNamesPort struct{}

// Portable answers name's key, or refuses it, as go-files' confine.Portable
// does and as host/fs answers: the one table of what a disk does with a name
// is go-files', so the world and the disk never disagree on one.
func (fileNamesPort) Portable(name string) (string, error) {
	return confine.Portable(name)
}
