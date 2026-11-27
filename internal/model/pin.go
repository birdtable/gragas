package model

// Pin is a project's pin, the one small file in which a repository names its
// rulebook (.gragas/pin.yaml). Release 1 reads four keys.
type Pin struct {
	// Rulebook is the rulebook's short name, never empty.
	Rulebook string
	// Source is where the rulebook is: a folder of the repository, relative to
	// the folder that holds .gragas/, slash-separated, with no empty, "." or
	// ".." step and no slash at either end; "." alone is that folder itself.
	// A git host or a mirror is not read in release 1.
	Source string
	// Parts are the parts in play for the project, at least one, none empty,
	// in the pin's order.
	Parts []string
	// Format is the pin file's own version, a whole number from 1 up to the
	// one this gragas reads. It is not the rulebook's format.
	Format int
}
