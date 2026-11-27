package config

// The shell setting: the dialect every line to paste is written for, the plan,
// apply, next and way-back lines. The names are go-console's
// (shell.ParseDialect, shell.Dialects), each but shell.Windows, the dialect the
// trust fix line is written for and no shell a person pastes into, which the
// setting does not take. auto is gragas's word alone, for host/term's
// HereDialect, which reads the system and so is main's to call, not this
// package's.

import (
	"fmt"
	"strings"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/gragas/internal/ordered"
)

// auto is the shell value that leaves the dialect to the shell gragas was
// started from.
const auto = "auto"

// shellRefused is what the shell key and GRAGAS_SHELL refuse, as
// docs/settings.md says it.
const shellRefused = "a value that is not auto or a name of the shells this build has, naming them"

// shellNames are the values the shell key takes: auto, then the kit's
// dialects in its order, each as String writes it, but shell.Windows, which is
// the fix line's dialect and no shell a person starts.
func shellNames() []string {
	names := []string{auto}
	for _, d := range shell.Dialects() {
		if d == shell.Windows {
			continue
		}
		names = append(names, d.String())
	}
	return names
}

// shellName is s as the shell key holds it, auto or a dialect's own name,
// and ok false when s is neither, shell.Windows among them. Both are matched
// in any case, as ParseDialect matches the kit's names, so PowerShell from a
// variable works.
func shellName(s string) (name string, ok bool) {
	if strings.EqualFold(s, auto) {
		return auto, true
	}
	d, err := shell.ParseDialect(s)
	if err != nil || d == shell.Windows {
		return "", false
	}
	return d.String(), true
}

// readShell reads the shell key.
func readShell(s *Settings, p place, v any) *Problem {
	str, isString := v.(string)
	name, ok := shellName(str)
	if !isString || !ok {
		return p.refuse("must be %s, and it is %s", either(shellNames()), ordered.Repr(v))
	}
	s.Shell = name
	return nil
}

// badShell says why s, GRAGAS_SHELL's value, is refused, "" when it is not.
func badShell(s string) string {
	if _, ok := shellName(s); ok {
		return ""
	}
	return fmt.Sprintf("must be %s, and it is %q", either(shellNames()), s)
}

// Dialect is the dialect the lines to paste are written for: GRAGAS_SHELL's,
// else the shell key's. here is true when that is auto, or when neither
// names a dialect, which is a run Load refused, and then d is 0: the caller
// asks host/term's HereDialect, which reads the system.
func (c Config) Dialect() (d shell.Dialect, here bool) {
	name := c.shellEnv
	if name == "" {
		name = c.Settings.Shell
	}
	d, err := shell.ParseDialect(name)
	if err != nil {
		return 0, true
	}
	return d, false
}
