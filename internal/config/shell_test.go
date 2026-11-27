package config_test

import (
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-files/appdirs"

	"github.com/birdtable/gragas/internal/config"
)

// TestTheShellIsTheVariablesElseTheKeysElseAuto: GRAGAS_SHELL decides the
// dialect over the shell key, the key over auto; a name is matched in any
// case; auto, from either, leaves the dialect to the shell gragas was started
// from, which config does not read. Risk: a journey
// that sets GRAGAS_SHELL=posix still gets the lines of the shell it was
// started from, or a file's shell: cmd is lost under a variable left unset.
func TestTheShellIsTheVariablesElseTheKeysElseAuto(t *testing.T) {
	t.Parallel()
	e, file := shellSettings(t)
	for _, tc := range []struct {
		name, env, key string
		want           shell.Dialect
		here           bool
	}{
		{"neither", "", "", 0, true},
		{"the key alone", "", "cmd", shell.Cmd, false},
		{"the key in another case", "", "POSIX", shell.POSIX, false},
		{"the key's auto", "", "auto", 0, true},
		{"the variable alone", "PowerShell", "", shell.PowerShell, false},
		{"the variable over the key", "posix", "cmd", shell.POSIX, false},
		{"the variable's auto over the key", "AUTO", "cmd", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{}
			if tc.key != "" {
				files[file] = "version: 1\nshell: " + tc.key + "\n"
			}
			c, err := config.Load(env(with(e, config.ShellVar, tc.env)), "", reader(files, nil))
			if err != nil {
				t.Fatal(err)
			}
			if d, here := c.Dialect(); d != tc.want || here != tc.here {
				t.Errorf("Dialect() = %v, %v; want %v, %v", d, here, tc.want, tc.here)
			}
		})
	}
}

// TestTheShellRefusalNamesEveryDialectTheKitHas: a shell key that names no
// shell is refused, and the refusal names auto and each of go-console's
// shell.Dialects as its String writes them, so a dialect the kit adds is
// named with no change here; the run falls back to the defaults, auto. The
// kit's own ParseDialect refuses the same name. Risk: a refusal that leaves
// out a value the key takes, or a list of names gragas keeps that drifts from
// the kit's.
func TestTheShellRefusalNamesEveryDialectTheKitHas(t *testing.T) {
	t.Parallel()
	e, file := shellSettings(t)
	// windows is the trust fix line's dialect, not a shell a person sets, so it
	// is refused like any other name the setting does not take.
	for _, v := range []string{"bash", `""`, "' posix'", "[posix]", "1", "windows", "Windows"} {
		c, err := config.Load(env(e), "", reader(map[string]string{file: "version: 1\nshell: " + v + "\n"}, nil))
		p, ok := errors.AsType[*config.Problem](err)
		if !ok || p.Kind != config.Refused || !strings.HasPrefix(p.What, file+":2: shell must be auto, ") {
			t.Fatalf("shell: %s: %v; want refused on line 2", v, err)
		}
		// The values the refusal offers, not the value it echoes back: a refused
		// "windows" is quoted after ", and it is ", and is no value offered.
		_, rest, _ := strings.Cut(p.What, "shell must be ")
		offered, _, _ := strings.Cut(rest, ", and it is ")
		for _, d := range shell.Dialects() {
			named := strings.Contains(offered, d.String())
			if d == shell.Windows {
				if named {
					t.Errorf("shell: %s: %q offers windows, which is no setting value", v, offered)
				}
				continue
			}
			if !named {
				t.Errorf("shell: %s: %q does not offer %s", v, offered, d)
			}
		}
		if d, here := c.Dialect(); d != 0 || !here {
			t.Errorf("shell: %s: Dialect() = %v, %v; want auto, from the defaults", v, d, here)
		}
	}
	_, err := config.Load(env(with(e, config.ShellVar, "bash")), "", reader(nil, nil))
	if p, ok := errors.AsType[*config.Problem](err); !ok || p.Next != "unset GRAGAS_SHELL, or set it to auto, posix, powershell or cmd" {
		t.Errorf("GRAGAS_SHELL=bash: %v; want the fix to name the variable and the values", err)
	}
	if _, err := shell.ParseDialect("bash"); !errors.Is(err, shell.ErrDialect) {
		t.Errorf("the kit takes bash: %v", err)
	}
}

// shellSettings is a test's environment, a home and each variable
// appdirs.Sources names for gragas's folders under a temporary folder
// (moving), and the settings file config reads under it: the config folder
// appdirs answers for that environment, so the file is where Load looks on
// the system the test runs on, Windows included, and no layout is spelled
// here: a layout spelled here turned the shell key tests red on Windows.
func shellSettings(t *testing.T) (e map[string]string, file string) {
	t.Helper()
	dir := t.TempDir()
	e = moving(t, map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}, func(f string) string { return filepath.Join(dir, f) })
	return e, filepath.Join(folders(t, e).Config, "settings.yaml")
}

// with is a copy of e with k set to v.
func with(e map[string]string, k, v string) map[string]string {
	out := maps.Clone(e)
	out[k] = v
	return out
}
