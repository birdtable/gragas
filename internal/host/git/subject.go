package git

import (
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
)

// SSHSetting is the settings key that names the ssh a fetch from an ssh://
// host runs, and CredentialSetting the one credential helper a fetch from an
// https:// host may ask.
const (
	SSHSetting        = "programs.ssh"
	CredentialSetting = "programs.credential"
)

// subject is a path the walk checks before it is used: the setting
// that names it, what a person calls it and its file, the kind its last
// name must be, what cannot happen while it fails, and what to set the
// setting to. A path no setting names, as a fetch's source, says instead
// what a refusal stops (not), the one fix for a path that is missing or
// fails (fix), and the fix for a name another account owns (owned).
type subject struct {
	setting, name, file string
	last                hosttrust.Kind
	cannot              string
	setTo, installed    string
	not, fix, owned     string
}

// fetchSource is the repository a fetch reads: upload-pack runs
// under its config, so a source another account could write could run what
// it likes as this one (CVE-2024-32004). It is a folder, named by the
// caller, not by a setting.
func fetchSource() subject {
	return subject{
		setting: "the source", name: "the source", file: "the repository", last: hosttrust.Folder, cannot: "nothing is fetched",
		not: ", so nothing is fetched from it", fix: "fetch from a repository or a mirror this account or root owns, which no other account can write",
		owned: "fetch from a mirror this account or root owns, made by its owner or by root: a clone of another account's repository runs its hooks (CVE-2024-32004)",
	}
}

// fetchGitDir is the git dir of a fetch's source work tree, its .git, held
// as the source is: its config and hooks are the ones upload-pack reads.
func fetchGitDir() subject {
	s := fetchSource()
	s.setting, s.name, s.file = "the source's git dir", "the source's git dir", "the git dir"
	return s
}

// gitProgram is git, named by programs.git.
func gitProgram() subject {
	return subject{
		setting: Setting, name: "git", file: "git", last: hosttrust.Program, cannot: "git cannot be run",
		setTo:     "the absolute path of git, as `command -v git` prints it",
		installed: "the path of git as installed by the system",
	}
}

// sshProgram is the ssh git runs to fetch from an ssh:// host, named by
// programs.ssh.
func sshProgram() subject {
	return subject{
		setting: SSHSetting, name: "ssh", file: "ssh", last: hosttrust.Program, cannot: "nothing is fetched from an ssh:// host",
		setTo:     "the absolute path of ssh, as `command -v ssh` prints it",
		installed: "the path of ssh as installed by the system",
	}
}

// credentialHelper is the one credential helper a fetch from an https://
// host may ask, named by programs.credential. With none, the
// fetch sends no credential.
func credentialHelper() subject {
	return subject{
		setting: CredentialSetting, name: "the credential helper", file: "the credential helper", last: hosttrust.Program,
		cannot:    "nothing is fetched from an https:// host",
		setTo:     "the absolute path of a git credential helper, or remove the line to fetch public hosts only",
		installed: "the path of the helper as installed by the system",
	}
}

// knownHosts is the account's known_hosts file, the only one an ssh fetch
// checks a host's key against. It is read and not run, and
// gragas never adds a key to it. Its fix gets the key of host, at port when
// the URL names one, by hand.
func knownHosts(host, port string) subject {
	scan := host
	if port != "" {
		scan = "-p " + port + " " + host
	}
	fix := "get the host's key with `ssh-keyscan " + scan + "`, check it against a fingerprint the host publishes, then add the line to the account's ~/.ssh/known_hosts"
	return subject{
		setting: "known_hosts", name: "known_hosts", file: "known_hosts", last: hosttrust.File, cannot: "nothing is fetched from an ssh:// host",
		not: ", so nothing is fetched from it", fix: fix,
	}
}

// missing is the not ready of a path the setting does not name, or names
// where nothing is.
func (s subject) missing(why string) error {
	fix := "set " + s.setting + " in the settings file to " + s.setTo
	if s.fix != "" {
		fix = s.fix
	}
	r, err := model.NewRefusal(model.NotReady, s.setting+" "+why+", so "+s.cannot, nil, fix)
	if err != nil {
		return err
	}
	return &NotReady{r}
}

// refused is the refusal of a path the check does not pass, with the line
// that fixes it, or the setting to change when no line does.
func (s subject) refused(observed, fix string) error {
	switch {
	case fix == "" && s.fix != "":
		fix = s.fix
	case fix == "":
		fix = "set " + s.setting + " in the settings file to " + s.installed
	}
	not := ", so it is not run"
	if s.not != "" {
		not = s.not
	}
	r, err := model.NewRefusal(model.Refused, observed+not, nil, fix)
	if err != nil {
		return err
	}
	return &Refused{r}
}

// trust is s as the walk names it, with the line that moves its file and
// sets its setting there. A path no setting names, as a fetch's source, has
// its own fix and no such line, so a refusal the walk has no fix for takes
// the fix of s (refused).
func (s subject) trust() hosttrust.Subject {
	point := "set " + s.setting + " to its path there"
	if s.fix != "" {
		point = ""
	}
	return hosttrust.Subject{Name: s.name, File: s.file, Last: s.last, Point: point}
}

// releasesCache is the folder a fetch keeps its repositories in,
// named by the caller, not by a setting. Each name of it that is there is
// walked before a folder or a repository is made in it: a
// cache reached through a link in a folder another account may write is
// refused, so nothing is fetched into the folder that account chose.
func releasesCache() subject {
	return subject{
		setting: "the releases cache", name: "the releases cache", file: "the releases cache", last: hosttrust.Folder,
		cannot: "nothing is fetched", not: ", so nothing is fetched into it",
		fix: "point the releases cache at a folder of this account's that no other account can write",
	}
}
