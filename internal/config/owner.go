package config

import (
	"path/filepath"

	"github.com/birdtable/go-files/appdirs"
)

// The owner's own settings file, for a member whose input is a trust input:
// the names member reads the private names list from the
// settings file's names-file key, because the key is in a file the owner
// holds. Which settings file is read is chosen by the environment, and an
// environment the repository controls (a Taskfile's env:, a hook) could point
// the member at a settings file of its own, whose key names an empty list, and
// the gate would look and find nothing. OwnersSettingsFile holds a run to the
// file the home alone gives.

// FolderVar is the variable that moves the config folder on its own on
// this system, in config's layout, as go-files' appdirs names it (Sources), or
// "" where none does. gragas names no such variable itself.
func FolderVar() string {
	s, err := appdirs.Sources(appdirs.XDG)
	if err != nil {
		return ""
	}
	return s.Config
}

// StateVar is the variable that moves the state folder on this system, in
// config's layout, as go-files' appdirs names it (Sources), or "" where none
// does. The gate's preset-plugins member points it at a folder of its own run,
// so an approval it records is never the account's. gragas names
// no such variable itself.
func StateVar() string {
	s, err := appdirs.Sources(appdirs.XDG)
	if err != nil {
		return ""
	}
	return s.State
}

// SystemHome is platform's call for the account's home, as the system records it
// and never as a variable says (go-files appdirs.AccountHome); gragas names no system.
func SystemHome() (string, error) { return appdirs.AccountHome() }

// OwnersSettingsFile holds a names run to the owner's own settings file: nil when
// the file a run reads is the one under the account's home as accountHome gives it
// (SystemHome in the program, never a variable). Otherwise it is not ready, and says
// to unset the variable, never naming a folder, a file or a list, when
//   - GRAGAS_CONFIG is set,
//   - the home variable names another folder than the account's home, or
//   - FolderVar names a config folder other than the one the home gives;
//
// or, where the system cannot say the account's home, to run as an account it holds.
// A home variable that is unset is left to Load, which is not ready with no home.
func OwnersSettingsFile(getenv func(string) string, accountHome func() (string, error)) *Problem {
	const why = "the names check reads its list only from the owner's own settings file, the config folder's settings.yaml under the account's own home as the system gives it, since a variable could name another file whose names-file key names a list that finds nothing"
	if getenv(ConfigVar) != "" {
		return notReady(unset(ConfigVar), "%s is set, and %s", ConfigVar, why)
	}
	ah, err := accountHome()
	if err != nil {
		return notReady("run it as an account the system holds a home for", "the system does not say this account's home, and %s", why)
	}
	if h := getenv(HomeVar); h != "" && filepath.Clean(h) != ah {
		return notReady(unset(HomeVar), "%s names a home other than the account's own, and %s", HomeVar, why)
	}
	v := FolderVar()
	if v == "" || getenv(v) == "" {
		return nil
	}
	var with, without Config
	with.folders(getenv)
	without.folders(func(k string) string {
		if k == v {
			return ""
		}
		return getenv(k)
	})
	if with.ConfigDir != without.ConfigDir || with.home != without.home {
		return notReady(unset(v), "%s names a config folder other than the one the home alone gives, and %s", v, why)
	}
	return nil
}
