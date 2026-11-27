package config

import "github.com/birdtable/go-files/appdirs"

// StateWords are the two variables that place gragas's state folder on the
// system it is built for, as go-files' appdirs names them: the one the state
// folder is read from, and the one the home is read from. The state variable is
// "" where only the home moves the folder, and both are "" on a system appdirs
// names no folders for. No variable is spelled in gragas, which leaves each
// system's names to appdirs; the layers table lets this package and host/trust
// import appdirs, and this package is the one that gives these words.
func StateWords() (state, home string) {
	f, err := appdirs.Sources(appdirs.XDG)
	if err != nil {
		return "", ""
	}
	return f.State, appdirs.HomeVar
}

// DefaultState is whether c's state folder is the account's own: the one
// go-files' appdirs gives under the home home answers (SystemHome in the
// program, the system's record and never a variable), with no variable set
// that moves it. No clone and no fetched rulebook chooses that folder, so the
// Journal and the locks do not hold it against the folders a run works in:
// a working folder of the home, or a home that is a
// repository, is no refusal. A state folder a variable moved, either the
// state variable or a home variable naming another folder, is not the
// account's own, and neither is any doubt: no state folder, an error from
// home, or a home appdirs would not join. That folder is judged.
func (c Config) DefaultState(home func() (string, error)) bool {
	if c.StateDir == "" {
		return false
	}
	h, err := home()
	if err != nil {
		return false
	}
	var own Config
	own.folders(func(k string) string {
		if k == HomeVar {
			return h
		}
		return ""
	})
	return own.StateDir != "" && own.StateDir == c.StateDir
}
