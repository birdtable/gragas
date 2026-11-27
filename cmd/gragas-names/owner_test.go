package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
)

// TestTheSettingsFileIsTheOwnersAlone: the
// names member reads its settings file only from the owner's own place. A
// variable that points it elsewhere, at a planted settings file whose key names
// a planted empty list, is not ready (exit 4), in the check and in -not-asked,
// naming the variable and "unset", and never the planted file's path or the
// list's. So is one pointed at a planted settings file whose list is not empty
// and holds no name of the text, which would pass, and a -not-asked pointed at a
// planted settings file with no key, which would read as not asked. The config
// folder's variable at exactly the folder the home alone gives is the owner's
// place and is read. With neither set the owner's file is read, and an empty
// list there stays not ready (supply.secrets.4). The owner's list holds a planted
// name the phrase carries, so a run that read it ends in a finding.
func TestTheSettingsFileIsTheOwnersAlone(t *testing.T) {
	t.Parallel()
	// The repository carries the stand-in in a statement, so a run that read
	// the owner's list ends in a finding.
	upper := strings.ToUpper(standIn()[:1]) + standIn()[1:]
	repoFiles := replaced(t, fixture(t), "core/base/rules/errors.yaml", "Return an error with the step", "Ask "+upper+" to return an error with the step")
	emptyList := home("planted", "empty-names.txt")
	elsewhere := home("planted", "settings.yaml")
	xdg := home("planted-xdg")
	redirected := "version: 1\nnames-file: " + emptyList + "\n"
	// v is the variable appdirs names for the config folder on this system, and
	// owners the value of it that names the folder the home alone gives (the
	// config folder's parent): neither is named here.
	v := config.FolderVar()
	owners := filepath.Dir(filepath.Dir(settingsPath()))
	// The settings file v would choose at the planted folder, as config resolves it.
	vSettings := config.FromEnv(func(k string) string {
		switch k {
		case config.HomeVar:
			return home()
		case v:
			return xdg
		}
		return ""
	}).SettingsFile
	// Two more planted settings files: one whose list holds a name the text does
	// not, so a run that read it would pass, and one with no key, so a -not-asked
	// that read it would say the check is not asked here.
	quiet, quietList, noKey := home("planted", "quiet.yaml"), home("planted", "quiet-names.txt"), home("planted", "no-key.yaml")
	plants := map[string]string{elsewhere: redirected, vSettings: redirected, quiet: "version: 1\nnames-file: " + quietList + "\n", noKey: "version: 1\n"}
	lists := map[string]string{listPath(): hashOf(standIn()) + "\n", emptyList: "", quietList: hashOf("zebra") + "\n"}
	base := func(env map[string]string, args []string) machine {
		return machine{settings: keyed(), lists: lists, settingsFiles: plants, files: repoFiles, env: env, args: args}
	}
	notAsked := []string{"-C", repo, "-not-asked"}
	type row struct {
		env    map[string]string
		args   []string
		code   int
		errHas []string
	}
	rows := map[string]row{
		"GRAGAS_CONFIG names a planted file":                 {env: map[string]string{config.ConfigVar: elsewhere}, code: exit.NotReady, errHas: []string{config.ConfigVar + " is set", "unset " + config.ConfigVar}},
		"-not-asked, GRAGAS_CONFIG names a planted file":     {env: map[string]string{config.ConfigVar: elsewhere}, args: notAsked, code: exit.NotReady, errHas: []string{config.ConfigVar + " is set"}},
		"GRAGAS_CONFIG names a file whose list would pass":   {env: map[string]string{config.ConfigVar: quiet}, code: exit.NotReady, errHas: []string{config.ConfigVar + " is set"}},
		"-not-asked, GRAGAS_CONFIG names a file with no key": {env: map[string]string{config.ConfigVar: noKey}, args: notAsked, code: exit.NotReady, errHas: []string{config.ConfigVar + " is set"}},
		"neither set: the owner's file is read":              {code: exit.Failed},
	}
	if v != "" {
		rows["the config folder's variable names a planted folder"] = row{env: map[string]string{v: xdg}, code: exit.NotReady, errHas: []string{v + " names a config folder", "unset " + v}}
		rows["-not-asked, the config folder's variable names a planted folder"] = row{env: map[string]string{v: xdg}, args: notAsked, code: exit.NotReady, errHas: []string{v + " names a config folder"}}
		rows["the config folder's variable at the owner's folder is read"] = row{env: map[string]string{v: owners}, code: exit.Failed}
	}
	for name, c := range rows {
		code, out, errs := base(c.env, c.args).run(t)
		if code != c.code {
			t.Errorf("%s: exit %d, out %q, err %q; want exit %d", name, code, out, errs, c.code)
		}
		for _, w := range c.errHas {
			if !strings.Contains(errs, w) {
				t.Errorf("%s: stderr %q lacks %q", name, errs, w)
			}
		}
		if c.code == exit.NotReady && out != "" {
			t.Errorf("%s: a refused run printed %q", name, out)
		}
		for _, p := range []string{elsewhere, xdg, emptyList, listPath(), quiet, quietList, noKey} {
			if strings.Contains(errs, p) || strings.Contains(out, p) {
				t.Errorf("%s: the run names %s", name, p)
			}
		}
	}
	t.Run("an empty list in the owner's file stays not ready", func(t *testing.T) {
		t.Parallel()
		m := machine{settings: keyed(), lists: map[string]string{listPath(): ""}, files: repoFiles}
		if code, out, errs := m.run(t); code != exit.NotReady || out != "" {
			t.Errorf("exit %d, out %q, err %q; want not ready", code, out, errs)
		}
	})
}
