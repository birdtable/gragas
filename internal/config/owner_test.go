package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
)

// TestOnlyTheOwnersSettingsFileIsTheNamesMembers: the names
// member's settings file is the owner's own, the config folder's settings.yaml
// as the home alone gives it. GRAGAS_CONFIG set, or the variable appdirs names
// for the config folder moving it elsewhere, is not ready, naming the variable
// and "unset", and never the value. That variable at exactly the folder the home
// alone gives (the home's .config in the XDG layout), and neither variable, are
// the owner's. No variable or folder is named here: the variable is appdirs'
// (config.FolderVar), the owner's folder config's own answer. Planted
// failure: the folder compare dropped, so a planted folder reads as the owner's.
func TestOnlyTheOwnersSettingsFileIsTheNamesMembers(t *testing.T) {
	t.Parallel()
	home := filepath.Join(t.TempDir(), "h")
	planted := filepath.Join(t.TempDir(), "planted")
	v := config.FolderVar()
	// The value of v that names the folder the home alone gives: the config
	// folder's parent, as config resolves it with v unset.
	owners := filepath.Dir(folders(t, map[string]string{config.HomeVar: home}).Config)
	cases := map[string]struct {
		env     map[string]string
		refused string
	}{
		"neither set":                       {env: map[string]string{}},
		"GRAGAS_CONFIG set":                 {env: map[string]string{config.ConfigVar: filepath.Join(planted, "settings.yaml")}, refused: config.ConfigVar},
		"GRAGAS_CONFIG at the owner's file": {env: map[string]string{config.ConfigVar: filepath.Join(owners, "gragas", "settings.yaml")}, refused: config.ConfigVar},
	}
	if v != "" {
		cases["the config folder's variable at the owner's folder"] = struct {
			env     map[string]string
			refused string
		}{env: map[string]string{v: owners}}
		cases["the config folder's variable at a planted folder"] = struct {
			env     map[string]string
			refused string
		}{env: map[string]string{v: planted}, refused: v}
	}
	for name, c := range cases {
		c.env[config.HomeVar] = home
		p := config.OwnersSettingsFile(env(c.env), func() (string, error) { return home, nil })
		switch {
		case c.refused == "" && p != nil:
			t.Errorf("%s: %v, want the owner's file", name, p)
		case c.refused != "" && (p == nil || p.Kind != config.NotReady || !strings.Contains(p.What, c.refused) || p.Next != "unset "+c.refused):
			t.Errorf("%s: %+v, want not ready naming %s and unset it", name, p, c.refused)
		case p != nil && (strings.Contains(p.What+p.Next, planted) || strings.Contains(p.What+p.Next, home)):
			t.Errorf("%s: %q names the folder", name, p.What+" "+p.Next)
		}
	}
}

// TestTheOwnersHomeIsTheAccountsNotTheVariables: the home the
// names member reads under is the one the system gives (the function handed in,
// appdirs.AccountHome in the program). The home variable at another folder is
// not ready, naming it and unset; the variable unset is left to Load;
// a system that cannot say is not ready; none names a folder. Planted failure:
// the variable taken in place of the account's home.
func TestTheOwnersHomeIsTheAccountsNotTheVariables(t *testing.T) {
	t.Parallel()
	account := filepath.Join(t.TempDir(), "acct")
	planted := filepath.Join(t.TempDir(), "planted")
	says := func() (string, error) { return account, nil }
	if p := config.OwnersSettingsFile(env(map[string]string{config.HomeVar: planted}), says); p == nil || p.Kind != config.NotReady || p.Next != "unset "+config.HomeVar || strings.Contains(p.What+p.Next, planted) || strings.Contains(p.What+p.Next, account) {
		t.Errorf("the variable at a planted home: %+v", p)
	}
	for name, vars := range map[string]map[string]string{"unset": {}, "at the account's home": {config.HomeVar: account}} {
		if p := config.OwnersSettingsFile(env(vars), says); p != nil {
			t.Errorf("%s: %+v, want the owner's file", name, p)
		}
	}
	nope := func() (string, error) { return "", errors.New("no account") }
	if p := config.OwnersSettingsFile(env(map[string]string{config.HomeVar: account}), nope); p == nil || p.Kind != config.NotReady || strings.Contains(p.What+p.Next, account) {
		t.Errorf("the system cannot say: %+v", p)
	}
}

// TestSystemHomeIsTheSystemsAnswerAndNotTheVariable: SystemHome,
// the call the names tool's main() hands the run, answers an absolute folder
// from the system's own record, and the isolated process's home variable, which
// names a folder of the test's own, is not its answer. Planted failure:
// SystemHome returning the variable.
func TestSystemHomeIsTheSystemsAnswerAndNotTheVariable(t *testing.T) {
	t.Parallel()
	got, err := config.SystemHome()
	if err != nil || !filepath.IsAbs(got) {
		t.Fatalf("SystemHome = %q, %v; want an absolute folder", got, err)
	}
	if v := os.Getenv(config.HomeVar); v == "" || got == filepath.Clean(v) {
		t.Errorf("SystemHome = the home variable (%q is set: %v)", config.HomeVar, v != "")
	}
}

// TestTheStateVarMovesTheStateFolder: the variable StateVar
// names is the one that moves the state folder config gives, which the gate's
// preset-plugins member points at a folder of its own run, so an approval it
// records is never the account's. Set to a planted folder, the state folder is
// below it; unset, it is not. No variable or folder is named here: the
// variable is appdirs' (config.StateVar).
func TestTheStateVarMovesTheStateFolder(t *testing.T) {
	t.Parallel()
	v := config.StateVar()
	if v == "" {
		t.Fatal("no variable moves the state folder on this system, so the gate's preset-plugins member cannot hold an approval apart from the account's")
	}
	home := filepath.Join(t.TempDir(), "h")
	planted := filepath.Join(t.TempDir(), "planted")
	moved, err := config.Load(env(map[string]string{config.HomeVar: home, v: planted}), "", reader(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(planted, moved.StateDir); err != nil || !filepath.IsLocal(rel) {
		t.Errorf("with %s set, the state folder is %s, not below the planted folder %s", v, moved.StateDir, planted)
	}
	kept, err := config.Load(env(map[string]string{config.HomeVar: home}), "", reader(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(kept.StateDir, planted) {
		t.Errorf("with %s unset, the state folder %s is below the planted folder", v, kept.StateDir)
	}
}
