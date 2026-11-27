package model

import "strings"

// SameName says whether two names of one step of a path name the same file on
// some file system gragas runs on: equal once ASCII letters are folded to one
// case and trailing dots are dropped. The Mac's file system and Windows' open
// .git by .GIT and .Git, and Windows opens it by ".git." too, so a refusal of
// a reserved folder that compares the bytes is walked around by the spelling.
// It is asked alike on every system, so a manifest refused on one is refused
// on all.
//
// It is the one compare for a reserved folder's name (.git, .bin): the
// manifest's judgement, the step runner and the package script's refusal ask
// it and keep no fold of their own.
//
// It folds ASCII alone, and drops dots alone. A caller whose names can hold
// more than a path of the manifest can (a space, a colon, a tilde, a letter
// outside ASCII) holds those itself: Windows also drops a trailing space,
// reads "a::$DATA" as a, and may know .git as GIT~1, and none of those is
// judged here. A name of dots alone is the same as the empty name, which is no
// step: the path's own grammar refuses both.
func SameName(a, b string) bool {
	a, b = strings.TrimRight(a, "."), strings.TrimRight(b, ".")
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
