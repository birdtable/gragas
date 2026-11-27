package plugin

import (
	"context"

	"github.com/birdtable/gragas/internal/model"
)

// Scope is what the steps of one read are run under: the
// manifest whose grants name every program a step may run, the project the
// programs are found for, the prepared folder every step runs and reads in,
// and the engine's floor. The host's step runner is bound to one Scope, and
// holds no table of tools: what it runs is the manifest's.
type Scope struct {
	// Manifest is the opened plugin's.
	Manifest Manifest
	// Root is the project, absolute and slash-separated, the grants' programs
	// are found for, in the order settings, the grant's own place, the tools
	// file.
	Root model.Root
	// OwnPlace lets the find take the grant's own place below the project, a
	// program at its path or a script under the manifest, for a project the
	// person wrote. The zero value is the pinned find, over the pinned places
	// alone, so a Scope filled with no thought for it never runs a program the
	// repository supplies.
	OwnPlace bool
	// Scratch is the prepared folder, with the config set mirrored into it,
	// and the level it reached.
	Scratch model.Scratch
	// Floor is the lowest level a step runs at: a step that asks for less
	// runs at the floor, one that asks for more runs at its own.
	Floor Level
	// Note journals a step before it starts (Noted); the step does not start
	// when it answers an error.
	Note func(ctx context.Context, n Noted) error
}

// Noted is a step as it is about to run, the line it is journalled by:
// the grant, the program and the version it was found at, the words as the
// program is given them, and the level it runs at.
type Noted struct {
	// Round and Step are the step's place in the read.
	Round, Step int
	// Grant is the id the step named its program by.
	Grant string
	// Program is the absolute path started, and Version the version it
	// printed. A script grant's Program is the interpreter, and Script the
	// script put first in its words; "" for any other grant.
	Program, Script, Version string
	// Words are the words after the program's own, as it is given them.
	Words []string
	// Level is the level the step runs at; "" for a version ask, which runs
	// in a fresh folder of its own and at no step's level.
	Level Level
	// VersionAsk is a start that asks a grant's program its version, the
	// first time the read finds it: at Bind for the tool's grant, where
	// Round and Step are 0, and at the first step that names any other
	// grant, with that step's Round and Step. Words are the grant's version
	// argv, and Version is "": it is what the start is to print. Every start
	// is journalled first, this one included.
	VersionAsk bool
}
