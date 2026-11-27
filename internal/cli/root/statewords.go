package root

import (
	"strings"

	"github.com/birdtable/gragas/internal/config"
)

// helpVar is the root help's name for the variable that places the state
// folder: the state variable config.StateWords gives, else the home's where
// only the home moves it, else, on a system appdirs names no folders for,
// the plain words "the home".
func helpVar() string {
	state, home := config.StateWords()
	switch {
	case state != "":
		return state
	case home != "":
		return home
	}
	return "the home"
}

// stateVarHelp is helpVar padded to the help's column, ready for the words
// after it.
func stateVarHelp() string {
	const col = 19
	v := helpVar()
	if len(v) >= col {
		return v + " "
	}
	return v + strings.Repeat(" ", col-len(v))
}

// noStateFolder is the refusal's two parts, when no state folder can be
// made: what is missing, and what to set. They name the variables and carry
// no shell syntax, so no one shell's line is in them. Where appdirs names no
// variable at all, they say the home folder.
func noStateFolder() (why, fix string) {
	state, home := config.StateWords()
	if home == "" {
		home = "the home folder"
	}
	const nowhere = "gragas has nowhere to keep its lock and run record: "
	if state == "" {
		return nowhere + home + " is not set", "set " + home + " to your home folder"
	}
	return nowhere + "neither " + state + " nor " + home + " is set",
		"set " + state + " to an absolute folder to keep them in, or " + home + " to your home folder"
}
