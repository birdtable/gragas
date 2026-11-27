package main

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/assets"
	"github.com/birdtable/gragas/internal/host/jsonschema"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// fixtureDir is the format 3 rulebook every planted case starts from: its
// statements, sources, bindings, owners and paths are the places a name can
// stand.
const fixtureDir = "../../internal/host/assets/fixture/base"

// repo is the folder the world seeds the rulebook at, and the run's -C.
const repo = "repo"

// home is a folder absolute on every host (a rooted "/home/test" has no volume
// on Windows, so config refuses it there: gragas#1625's Windows leg). Nothing is
// written under it: the settings file and the lists are planted by path.
func home(elem ...string) string {
	return filepath.Join(append([]string{os.TempDir(), "gragas-names-test"}, elem...)...)
}

// settingsPath is where the settings file is planted: the owner's own, the
// config folder's settings.yaml as config resolves it from the home alone, the
// one the member reads (gragas#1629). It is config's answer, never a shape
// written here.
func settingsPath() string {
	return config.FromEnv(func(k string) string {
		if k == config.HomeVar {
			return home()
		}
		return ""
	}).SettingsFile
}

// listPath is the list the settings file's key names.
func listPath() string { return home("names.txt") }

// standIn is the stand-in word of tests.doubles.7, put together at run time
// so no word of the list is a literal of the repository. The fixture does not
// hold it, so the clean control is clean.
func standIn() string { return strings.Join([]string{"quor", "vex"}, "") }

// hashOf is the line the scans hasher writes for a one-word name.
func hashOf(word string) string {
	return digest.Hex([]byte(word))
}

// fixture is the fixture rulebook by slash path.
func fixture(t *testing.T) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := fs.WalkDir(os.DirFS(fixtureDir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join(fixtureDir, filepath.FromSlash(p)))
		m[p] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// replaced is files with the first old in the file at p made new; the test
// stops if p does not hold old, so a plant that missed never passes as clean.
func replaced(t *testing.T, files map[string]string, p, old, new string) map[string]string {
	t.Helper()
	if !strings.Contains(files[p], old) {
		t.Fatalf("%s holds no %q to plant in", p, old)
	}
	out := maps.Clone(files)
	out[p] = strings.Replace(files[p], old, new, 1)
	return out
}

// lineHolding is the 1-based line of text that holds word.
func lineHolding(t *testing.T, text, word string) int {
	t.Helper()
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(l), word) {
			return i + 1
		}
	}
	t.Fatalf("no line holds the plant")
	return 0
}

// machine is a run's world: the settings file holding the key (or not), the
// environment, the list at each path, and the repository's files.
type machine struct {
	settings string // text of the settings file; "" for none
	// lists is the list at each path; a path with none is not there.
	lists map[string]string
	// account is the system's answer for the account's home; nil is home().
	account func() (string, error)
	// refused is the paths the owned read refuses as another account's.
	refused []string
	env     map[string]string
	files   map[string]string // the repository; nil is the fixture
	args    []string          // the run's flags; nil is -C repo
	// settingsFiles is more settings files the read answers by path: one
	// planted where a variable points, away from the owner's.
	settingsFiles map[string]string
}

// listed is the one list, at listPath, holding text.
func listed(text string) map[string]string { return map[string]string{listPath(): text} }

func (m machine) run(t *testing.T) (code int, out, errs string) {
	t.Helper()
	files := m.files
	if files == nil {
		files = fixture(t)
	}
	w := porttest.NewWorld(t)
	w.Seed(repo, files)
	for p, l := range m.lists {
		w.SeedOutside(p, l)
	}
	for _, p := range m.refused {
		w.SeedRefused(p, porttest.Refused{Why: "can be written by another account", Fix: "chmod go-w " + p})
	}
	account := m.account
	if account == nil {
		account = func() (string, error) { return home(), nil }
	}
	env := Env{
		AccountHome: account,
		Getenv: func(k string) string {
			// The folders come from the home (gragas#1234): a run with none is
			// not ready, so the test names one, a folder it never writes under,
			// by the one variable this system reads the home from.
			if k == config.HomeVar && m.env[k] == "" {
				return home()
			}
			return m.env[k]
		},
		Read: func(p string, _ []string) ([]byte, error) {
			if s, ok := m.settingsFiles[p]; ok {
				return []byte(s), nil
			}
			if p == settingsPath() && m.settings != "" {
				return []byte(m.settings), nil
			}
			return nil, fs.ErrNotExist
		},
		// The world's tree, with the schemas the command embeds: the seeds
		// are checked by the real format 3 schemas.
		World: func(config.Config) (port.Ports, func() error) {
			p := w.Ports()
			p.Schema = jsonschema.Embedded(assets.New(), "format/schema")
			return p, func() error { return nil }
		},
	}
	var o, e bytes.Buffer
	args := m.args
	if args == nil {
		args = []string{"-C", repo}
	}
	code = run(t.Context(), args, env, &o, &e)
	return code, o.String(), e.String()
}

// keyed is a settings file whose key names listPath.
func keyed() string { return "version: 1\nnames-file: " + listPath() + "\n" }

// saysNoName fails the test when what the run said holds the plant, in any
// case.
func saysNoName(t *testing.T, out, errs string) {
	t.Helper()
	if strings.Contains(strings.ToLower(out+errs), standIn()) {
		t.Errorf("the run says the name: out %q, err %q", out, errs)
	}
}

// TestAListedNameAnywhereTheLoadReadsIsFoundAtItsPlaceAndNeverSaid plants the
// name in each kind of place the old check never read, and a statement, which
// it did: each run is red with one finding at the planted file and line (the
// path, for a path), and nothing it prints holds the name (gragas#1741).
func TestAListedNameAnywhereTheLoadReadsIsFoundAtItsPlaceAndNeverSaid(t *testing.T) {
	t.Parallel()
	const rules = "core/base/rules/errors.yaml"
	base := fixture(t)
	upper := strings.ToUpper(standIn()[:1]) + standIn()[1:]
	for name, c := range map[string]struct {
		file, old, new string
	}{
		"a statement": {rules, "Return an error with the step", "Ask " + upper + " to return an error with the step"},
		// The fixture's sources are flow mappings, {slug: ..., title: ...}:
		// braces that are no term reference.
		"a source's title":       {"sources.yaml", "A standard of errors", "A standard of " + upper},
		"an undefined reference": {rules, "Return an error with the step", "Ask {" + standIn() + "} to return an error with the step"},
		"a binding remedy":       {rules, "remedy: Return the error you were given.", "remedy: Return the error " + standIn() + " gave you."},
		"a renamed field":        {rules, "owner: alex", "owner: " + standIn()},
		"a term definition":      {"terms.yaml", "definition: ", "definition: As " + upper + " says, "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := replaced(t, base, c.file, c.old, c.new)
			for _, list := range []string{hashOf(standIn()) + "\n", "\\b" + standIn() + "\\b\n"} {
				code, out, errs := machine{settings: keyed(), lists: listed(list), files: files}.run(t)
				want := c.file + ":" + strconv.Itoa(lineHolding(t, files[c.file], standIn())) + ": " + format.NamesMsg + "\n"
				if code != exit.Failed || out != want {
					t.Errorf("list %s: exit %d, out %q, err %q; want red with %q", strings.TrimSpace(list[:4]), code, out, errs, want)
				}
				saysNoName(t, out, errs)
			}
		})
	}
	t.Run("a path", func(t *testing.T) {
		t.Parallel()
		files := maps.Clone(base)
		const from = "bindings/std-errors-1.yaml"
		files["bindings/std-"+standIn()+"-1.yaml"] = files[from]
		delete(files, from)
		code, out, errs := machine{settings: keyed(), lists: listed(hashOf(standIn()) + "\n"), files: files}.run(t)
		if code != exit.Failed || !strings.Contains(out, ": "+format.NamesPathMsg+"\n") {
			t.Errorf("exit %d, out %q, err %q; want red with the path finding", code, out, errs)
		}
		saysNoName(t, out, errs)
	})
}

func TestACleanRulebookWithAListPassesAndSaysWhatItRead(t *testing.T) {
	t.Parallel()
	code, out, errs := machine{settings: keyed(), lists: listed(hashOf(standIn()) + "\n")}.run(t)
	if code != exit.Done || !strings.HasPrefix(out, "names: ") || !strings.Contains(out, " rules read, no line and no path holds a listed name") {
		t.Fatalf("control: exit %d, out %q, err %q", code, out, errs)
	}
}

// TestAListAnotherAccountCanWriteIsRefusedAndTheScanIsRed (#1741 line 5): the
// list is read by the owned read alone, so a list it refuses is never read.
// The list holds the plant the text carries, so a run that read it anyway
// would end on a finding (exit 1), not on not ready; and the refusal's line
// says neither the list's path nor a word of it.
func TestAListAnotherAccountCanWriteIsRefusedAndTheScanIsRed(t *testing.T) {
	t.Parallel()
	files := replaced(t, fixture(t), "sources.yaml", "A standard of errors", "A standard of "+standIn())
	m := machine{settings: keyed(), lists: listed(hashOf(standIn()) + "\n"), files: files}
	if code, out, errs := m.run(t); code != exit.Failed || out == "" {
		t.Fatalf("control: the same list, not refused: exit %d, out %q, err %q; want a finding", code, out, errs)
	}
	m.refused = []string{listPath()}
	code, out, errs := m.run(t)
	if code != exit.NotReady || out != "" || !strings.Contains(errs, "nothing was scanned") || !strings.Contains(errs, "another account") {
		t.Errorf("exit %d, out %q, err %q; want not ready, nothing scanned, the refusal said", code, out, errs)
	}
	if strings.Contains(errs, listPath()) || strings.Contains(errs, hashOf(standIn())) {
		t.Errorf("the refusal says the list's path or a line of it: %q", errs)
	}
	saysNoName(t, out, errs)
}

// TestAScanThatReadNothingIsNotClean: a run with no list to hold the rulebook
// to, or no rule read to hold to it, is not ready and says nothing was
// scanned, never a pass, so a gate that could not look never reads as one
// that looked and found nothing (supply.secrets.4).
func TestAScanThatReadNothingIsNotClean(t *testing.T) {
	t.Parallel()
	base := fixture(t)
	noRules := map[string]string{"rulebook.yaml": base["rulebook.yaml"], "structure.yaml": base["structure.yaml"]}
	cases := map[string]machine{
		"no settings file":           {},
		"no key":                     {settings: "version: 1\ncolor: auto\n"},
		"a missing list":             {settings: keyed()},
		"an empty list":              {settings: keyed(), lists: listed("# no name yet\n\n")},
		"a line that does not parse": {settings: keyed(), lists: listed("(unclosed\n")},
		"a rulebook with no rule":    {settings: keyed(), lists: listed(hashOf(standIn()) + "\n"), files: noRules},
		"no rulebook":                {settings: keyed(), lists: listed(hashOf(standIn()) + "\n"), files: map[string]string{"README.md": "# a repository\n"}},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, out, errs := m.run(t)
			if code != exit.NotReady || out != "" || !strings.Contains(errs, "nothing was scanned") {
				t.Errorf("exit %d, out %q, err %q; want not ready, saying nothing was scanned", code, out, errs)
			}
			if name != "a rulebook with no rule" && name != "no rulebook" && !strings.Contains(errs, "names-file") {
				t.Errorf("err %q; want the key named", errs)
			}
			if strings.Contains(errs, "unclosed") {
				t.Errorf("the line quotes the list: %q", errs)
			}
		})
	}
}

// TestTheVariableIsNotReadAndTheKeysListIs (gragas#1625): with the variable
// set to one planted list and the key naming another, the run reads the key's
// list only, says on stderr that the variable is not read, and never prints
// what the variable holds. The variable's list holds a word of the clean text,
// so a run that read it would be red: the control proves that list fires.
func TestTheVariableIsNotReadAndTheKeysListIs(t *testing.T) {
	t.Parallel()
	varPath := home("other-names.txt")
	lists := map[string]string{listPath(): hashOf(standIn()) + "\n", varPath: hashOf("failed") + "\n"}
	set := map[string]string{config.NamesFileVar: varPath}
	planted := replaced(t, fixture(t), "sources.yaml", "A standard of errors", "A standard of "+standIn())
	said := func(t *testing.T, errs string) {
		t.Helper()
		if !strings.Contains(errs, config.NamesFileVar+" is set and not read") || !strings.Contains(errs, "names-file") || strings.Contains(errs, varPath) {
			t.Errorf("stderr %q: want one line that names the variable as not read and the key, and not the variable's value", errs)
		}
	}
	t.Run("the variable's list would fire", func(t *testing.T) {
		t.Parallel()
		m := machine{settings: "version: 1\nnames-file: " + varPath + "\n", lists: lists}
		if code, out, errs := m.run(t); code != exit.Failed || out == "" {
			t.Fatalf("control: the variable's list as the key's: exit %d, out %q, err %q; want a finding", code, out, errs)
		}
	})
	t.Run("a clean text passes on the key's list", func(t *testing.T) {
		t.Parallel()
		code, out, errs := machine{settings: keyed(), lists: lists, env: set}.run(t)
		if code != exit.Done || !strings.HasPrefix(out, "names: ") {
			t.Errorf("exit %d, out %q, err %q; want done: the variable's list was read", code, out, errs)
		}
		said(t, errs)
	})
	t.Run("a plant of the key's list is found", func(t *testing.T) {
		t.Parallel()
		code, out, errs := machine{settings: keyed(), lists: lists, files: planted, env: set}.run(t)
		if code != exit.Failed || strings.Count(out, format.NamesMsg) != 1 {
			t.Errorf("exit %d, out %q, err %q; want the key's one finding", code, out, errs)
		}
		said(t, errs)
	})
	t.Run("no key is not ready, and the variable is no fallback", func(t *testing.T) {
		t.Parallel()
		code, out, errs := machine{settings: "version: 1\n", lists: lists, env: set}.run(t)
		if code != exit.NotReady || out != "" || !strings.Contains(errs, "has no names-file key") {
			t.Errorf("exit %d, out %q, err %q; want not ready naming the key", code, out, errs)
		}
		said(t, errs)
	})
}

// TestNotAskedIsRefusedWhereTheListIsNamed (gragas#1566): -not-asked says the check was not asked on a machine whose
// settings name no list, and is refused, red, on the machine that owes the check: the key names a list, or the
// settings cannot be read. The variable is not read (gragas#1625): set alone it is said and changes nothing, and set
// beside the key it does not lift the refusal. The list holds a planted name the text carries, and it is never opened: a
// read would end in a finding (exit 1, a line on stdout), not the refusal.
func TestNotAskedIsRefusedWhereTheListIsNamed(t *testing.T) {
	t.Parallel()
	planted := replaced(t, fixture(t), "sources.yaml", "A standard of errors", "A standard of "+standIn())
	list := listed(hashOf(standIn()) + "\n")
	notAsked := []string{"-C", repo, "-not-asked"}
	for name, c := range map[string]struct {
		m      machine
		code   int
		out    string
		errHas string
	}{
		"control: no settings file": {machine{files: planted, lists: list, args: notAsked}, exit.Done, "names: not asked: ", ""},
		"control: no key":           {machine{settings: "version: 1\ncolour: auto\n", files: planted, lists: list, args: notAsked}, exit.Done, "names: not asked: ", ""},
		"the key names a list":      {machine{settings: keyed(), files: planted, lists: list, args: notAsked}, exit.Usage, "", "names is asked here"},
		"the variable alone":        {machine{files: planted, lists: list, args: notAsked, env: map[string]string{config.NamesFileVar: listPath()}}, exit.Done, "names: not asked: ", config.NamesFileVar + " is set and not read"},
		"the variable and the key":  {machine{settings: keyed(), files: planted, lists: list, args: notAsked, env: map[string]string{config.NamesFileVar: home("other-names.txt")}}, exit.Usage, "", "names is asked here"},
		"the settings unreadable":   {machine{settings: "version: [\n", files: planted, lists: list, args: notAsked}, exit.Usage, "", "is refused"},
	} {
		code, out, errs := c.m.run(t)
		if code != c.code || !strings.HasPrefix(out, c.out) || (c.out == "" && out != "") || !strings.Contains(errs, c.errHas) {
			t.Errorf("%s: exit %d, out %q, err %q; want exit %d, out %q, err holding %q", name, code, out, errs, c.code, c.out, c.errHas)
		}
	}
}

// TestTheHomeIsTheAccountsAndNeverTheVariables (gragas#1958): the home the
// settings file is found under is the system's record of the account, so a home
// variable a repository's environment sets cannot move the file. Both plants end
// not ready, naming the variable and unset it,
// never a path or a list; the control at the account's own home reads its file.
// The first case plants a list that matches nothing in a repository that holds
// the name: the old code, which followed the variable, read it and ended 0, so
// the case tells old from new by its exit code and not only by the words (gragas#2104).
func TestTheHomeIsTheAccountsAndNeverTheVariables(t *testing.T) {
	t.Parallel()
	other := home("planted")
	plantedFile := config.FromEnv(func(k string) string {
		if k == config.HomeVar {
			return other
		}
		return ""
	}).SettingsFile
	lists := listed(hashOf(standIn()) + "\n")
	planted := replaced(t, fixture(t), "sources.yaml", "A standard of errors", "A standard of "+standIn())
	moved := map[string]string{config.HomeVar: other}
	settingsFiles := map[string]string{plantedFile: "version: 1\nnames-file: " + home("blind.txt") + "\n"}
	cases := map[string]struct {
		m    machine
		code int
		want string
	}{
		"the home variable at a planted home whose file names a list that finds nothing": {machine{settings: keyed(), settingsFiles: settingsFiles, lists: map[string]string{home("blind.txt"): hashOf("unrelated") + "\n"}, files: planted, env: moved}, exit.NotReady, config.HomeVar},
		"the home variable at a planted home with no key, not asked":                     {machine{settingsFiles: map[string]string{plantedFile: "version: 1\n"}, args: []string{"-not-asked"}, env: moved}, exit.NotReady, config.HomeVar},
		"the home variable at a planted home with no settings file at all":               {machine{args: []string{"-not-asked"}, env: moved}, exit.NotReady, config.HomeVar},
		"the system cannot say the account's home":                                       {machine{settings: keyed(), lists: lists, account: func() (string, error) { return "", errors.New("no account") }}, exit.NotReady, "does not say this account's home"},
		"the control: the variable at the account's own home":                            {machine{settings: keyed(), lists: lists, files: planted, env: map[string]string{config.HomeVar: home()}}, exit.Failed, ""},
	}
	for name, c := range cases {
		code, out, errs := c.m.run(t)
		if code != c.code || (c.want != "" && !strings.Contains(errs, c.want)) {
			t.Errorf("%s: exit %d, out %q, err %q; want exit %d naming %q", name, code, out, errs, c.code, c.want)
		}
		if c.code == exit.NotReady && (strings.Contains(out+errs, other) || strings.Contains(out+errs, standIn())) {
			t.Errorf("%s: the refusal names a path or a list: %q", name, out+errs)
		}
	}
}

// TestMainWiresTheSystemsHomeAndTheOwnedRead (gragas#2104): wiring, which main
// hands the run, holds the system's record of the home and the owned read, each
// compared by the function it is. Planted failure: either field set to another
// function or to nil.
func TestMainWiresTheSystemsHomeAndTheOwnedRead(t *testing.T) {
	t.Parallel()
	env := wiring()
	if reflect.ValueOf(env.AccountHome).Pointer() != reflect.ValueOf(config.SystemHome).Pointer() {
		t.Error("AccountHome is not config.SystemHome")
	}
	if reflect.ValueOf(env.Read).Pointer() != reflect.ValueOf(hosttrust.ReadOwned).Pointer() {
		t.Error("Read is not hosttrust.ReadOwned")
	}
	if reflect.ValueOf(env.World).Pointer() != reflect.ValueOf(world).Pointer() {
		t.Error("World is not world")
	}
}
