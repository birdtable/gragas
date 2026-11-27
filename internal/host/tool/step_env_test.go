package tool_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	"github.com/birdtable/go-proc/proc"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// stepHomed is a Tool whose Home is home, with a PATH of one folder outside
// every project and the settings' paths, and no adapter.
func stepHomed(home string, paths map[string]map[string]string) *tool.Tool {
	return tool.New(tool.Config{Paths: paths, Path: pathFolder(), Lang: "C", Home: home})
}

// stepEnvNames is the names of the variables a step run printed with EnvOut,
// less the system's own and the shell's.
func stepEnvNames(out string, system map[string]bool) []string {
	var names []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		name, _, _ := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if name != "PWD" && name != "SHLVL" && name != "_" && name != "OLDPWD" && !system[strings.ToUpper(name)] {
			names = append(names, name)
		}
	}
	return names
}

// TestAStepsEnvironmentIsPathLangAndTheHomeAlone (replaces
// TestTheToolsEnvironmentIsPathAndLangAlone, TestTheEnvironmentOfARunIsThisSystemsTable's
// planted half and TestAHomeIsTheOnlyVariableACallerCanAdd's variable half): a
// secret, USER, LD_PRELOAD, GOFLAGS and a home variable planted in this
// process's environment never reach a step's program. It has PATH and LANG
// alone, but for what the system's table gives it, and the step's Home with
// the variables that name it and GOENV, GOTOOLCHAIN and GOFLAGS held. A
// Tool with no PATH and no LANG gives the program none, and this process's own
// never stand in for them.
// Planted: env() adding proc.Bare() beside the system's part (PATH from this
// process reaches the program; the PATH, variables and bare rows red).
func TestAStepsEnvironmentIsPathLangAndTheHomeAlone(t *testing.T) {
	// serial: it sets variables of the process, to prove the child is handed none of them
	// The stand-in is built once, by go, which GOFLAGS below would break.
	program(t, private(t)+"/lint-a", answering())
	t.Setenv("GRAGAS_PLANTED_SECRET", "s3cret-value")
	t.Setenv("USER", "planted-user")
	t.Setenv("LD_PRELOAD", "/planted.so")
	t.Setenv("GOFLAGS", "-planted")
	t.Setenv(homeName(), "/planted-home")
	t.Setenv(homeVarAt(1), "/planted")
	tl, root := stepFound(t, answering(configtest.Vars("PATH", "LANG", "GRAGAS_PLANTED_SECRET", "USER", "LD_PRELOAD")))
	res, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 0))
	if got := text(t, res.Stdout); err != nil || got != pathFolder()+"|C|unset|unset|unset" {
		t.Errorf("the environment = %q, %v; want PATH and LANG alone", got, err)
	}

	tl, root = stepFound(t, answering(configtest.EnvOut()))
	res, err = stepRunOf(t, stepBoundTo(t, tl, root, 60, 0))
	if err != nil {
		t.Fatal(err)
	}
	// The system's part is the product's own environment for an empty
	// config, which go-proc's table alone gives it.
	system := map[string]bool{}
	for _, kv := range tool.Env(tool.New(tool.Config{}), "") {
		name, _, _ := strings.Cut(kv, "=")
		system[strings.ToUpper(name)] = true
	}
	want := []string{"GOENV", "GOFLAGS", "GOTOOLCHAIN", "LANG", "PATH"}
	for _, kv := range proc.HomeEnv(filepath.FromSlash("/")) {
		if name, _, _ := strings.Cut(kv, "="); !system[strings.ToUpper(name)] {
			want = append(want, name)
		}
	}
	slices.Sort(want)
	out := text(t, res.Stdout)
	if names := stepEnvNames(out, system); !slices.Equal(names, want) {
		t.Errorf("the variables = %v; want %v", names, want)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, w := range []string{"GOENV=off", "GOTOOLCHAIN=local", "GOFLAGS=-modcacherw", "LANG=C"} {
		if !slices.Contains(lines, w) {
			t.Errorf("the environment lacks %q:\n%s", w, out)
		}
	}
	for _, bad := range []string{"s3cret-value", "planted-user", "/planted.so", "-planted", "/planted-home", "/planted"} {
		if strings.Contains(out, bad) {
			t.Errorf("the environment holds the planted %q:\n%s", bad, out)
		}
	}

	// With no PATH and no LANG to give, the step has neither, and this
	// process's own never stands in for them. A grant's own place takes no
	// PATH to filter, so the program is the settings'.
	t.Setenv("LANG", "planted-lang")
	root = private(t)
	prog := program(t, private(t)+"/lint-a", answering(configtest.Vars("PATH", "LANG", "GRAGAS_PLANTED_SECRET", "USER")))
	bare := tool.New(tool.Config{Paths: map[string]map[string]string{root: {"lint-a@1.2.3": prog}}})
	res, err = stepRunOf(t, bind(t, bare, stepScope(t, root, true)))
	// A system may give a program a PATH of its own when it has none; LANG
	// and the rest it never makes up.
	if got := strings.Split(text(t, res.Stdout), "|"); err != nil || len(got) != 4 || strings.Join(got[1:], "|") != "unset|unset|unset" {
		t.Errorf("the bare environment = %q, %v; want no LANG, no secret, no USER", text(t, res.Stdout), err)
	}
}

// TestAStepsHomeIsATrustedFolderOutsideTheProject (replaces
// TestAHomeIsATrustedFolderOutsideTheProject and the odd-Home rows of
// TestAHomeIsTheOnlyVariableACallerCanAdd): a Home that is, or is below, a
// link, that is no folder, that group or others can write, that does not
// exist, that is no absolute clean folder, or that lies at or below the
// project root, refuses Bind before anything starts; a Home equal to the
// scratch folder of a step is refused at its run; the same folder made right
// is accepted, and a step writes only into the folder it was given.
// Planted: checkHome returning nil (every refusal row red).
func TestAStepsHomeIsATrustedFolderOutsideTheProject(t *testing.T) {
	t.Parallel()
	root := private(t)
	outside := private(t)
	prog := program(t, private(t)+"/lint-a", answering(configtest.WriteUnder(homeName(), "marker", "x")))
	paths := map[string]map[string]string{root: {"lint-a@1.2.3": prog}}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(outside+"/real/.cache", 0o700))
	testenv.Link(t, outside+"/real", outside+"/link")
	must(os.WriteFile(outside+"/file", nil, 0o600))
	must(os.MkdirAll(outside+"/open", 0o700))
	trusttest.Loosen(t, outside+"/open", trusttest.GroupWrites)
	must(os.MkdirAll(outside+"/above/inner", 0o700))
	trusttest.Loosen(t, outside+"/above", trusttest.OthersWrite)
	must(os.MkdirAll(root+"/sub/home", 0o700))
	for name, home := range map[string]string{
		"a link":                  outside + "/link",
		"below a link":            outside + "/link/.cache",
		"a file":                  outside + "/file",
		"group writable":          outside + "/open",
		"below a writable":        outside + "/above/inner",
		"missing":                 outside + "/missing",
		"the project root":        root,
		"below the project":       root + "/sub/home",
		"relative":                "relative/home",
		"the system's root":       "/",
		"not clean":               outside + "/real/../real",
		"a trailing slash":        outside + "/real/",
		"a control character":     "/ho\nme",
		"a project sibling":       root + "/hm",
		"beside, and not present": filepath.Dir(root) + "/missing-" + filepath.Base(root),
	} {
		tl := stepHomed(home, paths)
		_, err := tl.Bind(t.Context(), stepScope(t, root, false))
		if !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 0 {
			t.Errorf("%s (%s): %v, ran %d; want a refusal before anything starts", name, home, err, len(tl.Ran()))
		}
	}

	// A Home that is the folder a step runs in is refused at the step.
	tl := stepHomed(outside+"/real", paths)
	s := stepScope(t, root, false)
	s.Scratch.Dir = outside + "/real"
	r := bind(t, tl, s)
	if _, err := stepRunOf(t, r); !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 1 {
		t.Errorf("a Home that is the step's folder: %v, runs %d; want a refusal before the step starts", err, len(tl.Ran()))
	}

	tl = stepHomed(outside+"/real", paths)
	if _, err := stepRunOf(t, bind(t, tl, stepScope(t, root, false))); err != nil {
		t.Errorf("a trusted folder outside the project: %v; want it run", err)
	}
	_, marked := homeVar(outside + "/real")
	if _, err := os.Stat(marked + "/marker"); err != nil {
		t.Errorf("the step left no marker in the folder its home was given: %v", err)
	}
}

// TestAStepsHomeInsideTheProjectBySpellingOfALinkIsRefused (replaces
// TestAHomeInsideTheProjectBySpellingOfALinkIsRefused and
// TestAHomeInAnotherSpellingOfTheProjectIsRefused): the project spelled
// through a link, or at another case on a volume that does not tell case
// apart, is still the project, so a Home inside it is refused at Bind, and a
// Home that is the scratch folder spelled through a link is refused at the
// step; nothing is left in either.
// Planted: checkHome handing the root "" (the root and other-case rows red), and
// handing the step's folder "" (the scratch folder row red).
func TestAStepsHomeInsideTheProjectBySpellingOfALinkIsRefused(t *testing.T) {
	t.Parallel()
	root := private(t)
	other := private(t)
	prog := program(t, private(t)+"/lint-a", answering(configtest.WriteUnder(homeName(), "marker", "x")))
	if err := os.MkdirAll(root+"/sub/home", 0o700); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, root, other+"/rootlink")
	home := root + "/sub/home"
	spelled := other + "/rootlink"
	tl := stepHomed(home, map[string]map[string]string{spelled: {"lint-a@1.2.3": prog}})
	if _, err := tl.Bind(t.Context(), stepScope(t, spelled, false)); !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 0 {
		t.Errorf("the root spelled through a link: %v, ran %d; want a refusal before anything starts", err, len(tl.Ran()))
	}
	_, marked := homeVar(home)
	if _, err := os.Stat(marked + "/marker"); err == nil {
		t.Error("a step left its marker inside the project")
	}

	// The scratch folder spelled through a link to the Home's own folder.
	real := private(t)
	testenv.Link(t, real, other+"/reallink")
	paths := map[string]map[string]string{root: {"lint-a@1.2.3": prog}}
	tl = stepHomed(real, paths)
	s := stepScope(t, root, false)
	s.Scratch.Dir = other + "/reallink"
	if _, err := stepRunOf(t, bind(t, tl, s)); !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 1 {
		t.Errorf("a step folder spelled through a link to the Home: %v, runs %d; want refused before the step starts", err, len(tl.Ran()))
	}

	base := private(t)
	proj := base + "/proj"
	if err := os.MkdirAll(proj+"/sub/home", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(base + "/PROJ"); err != nil {
		return // this volume tells case apart: there is no other spelling to try
	}
	tl = stepHomed(base+"/PROJ/sub/home", map[string]map[string]string{proj: {"lint-a@1.2.3": prog}})
	if _, err := tl.Bind(t.Context(), stepScope(t, proj, false)); !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 0 {
		t.Errorf("a home below another spelling of the project: %v, ran %d; want a refusal before anything starts", err, len(tl.Ran()))
	}
}

// TestAStepsJudgesItsHomeByTheRootItWasBoundFor (replaces
// TestARunJudgesItsHomeByTheRootItWasFoundFor): a step holds the
// Home to the scope's root, not to none. The project is bound with the Home
// outside it; before the step the project folder is moved onto the Home's way
// and its old name made a link to it, so the root, judged as the folder it
// is, now holds the Home. The step is refused before it starts, and the Home is
// left as it was.
// Planted: checkHome handing the root "" (the step runs and is not refused).
func TestAStepsJudgesItsHomeByTheRootItWasBoundFor(t *testing.T) {
	t.Parallel()
	base := private(t)
	root, home := base+"/proj", base+"/hm"
	for _, d := range []string{root, home} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mine := program(t, base+"/machine/lint-a", answering(configtest.WriteUnder(homeName(), "marker", "x")))
	tl := stepHomed(home, map[string]map[string]string{root: {"lint-a@1.2.3": mine}})
	r := bind(t, tl, stepScope(t, root, false))
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, home); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, home, root)
	_, err := stepRunOf(t, r)
	var ref *tool.Refused
	if !errors.Is(err, port.ErrRefused) || !errors.As(err, &ref) || !strings.Contains(ref.Observed(), "the project folder") {
		t.Errorf("a Home the root now holds: %v; want refused as inside the project folder", err)
	}
	if ran := tl.Ran(); len(ran) != 1 || ran[0].Shape != "version" {
		t.Errorf("runs = %+v; want Bind's version run alone", ran)
	}
	if _, err := os.Stat(home + "/marker"); err == nil {
		t.Error("the step wrote into a Home inside the project")
	}
}

// TestAStepsRefusesAHomeWhoseFolderIsALink (replaces
// TestARunRefusesAHomeWhoseFolderIsALink): a link at the folder
// the step's home variable names, below the Home, is refused before anything
// starts, and nothing is made or written through it.
// Planted: makeHome making its folders with os.MkdirAll, which follows a link
// (Bind is no longer refused).
func TestAStepsRefusesAHomeWhoseFolderIsALink(t *testing.T) {
	t.Parallel()
	base := private(t)
	root, home, outside := base+"/proj", base+"/hm", base+"/elsewhere"
	for _, d := range []string{root, home, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	_, folder := homeVar(home)
	testenv.Link(t, outside, folder)
	mine := program(t, base+"/machine/lint-a", answering(configtest.WriteUnder(homeName(), "marker", "x")))
	tl := stepHomed(home, map[string]map[string]string{root: {"lint-a@1.2.3": mine}})
	if _, err := tl.Bind(t.Context(), stepScope(t, root, false)); err == nil {
		t.Error("a Home whose home folder is a link: bound; want it refused")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Errorf("the folder the link leads to holds %v, %v; want nothing made or written there", entries, err)
	}
}

// TestAStepsStoredSlashPathIsRunAsTheSystemsOwnAndRecordedAsStored (replaces
// TestAStoredSlashPathIsRunAsTheSystemsOwnAndRecordedAsStored): the root, the
// program's path, the step's folder and the Home
// are all held in the slash form of the model, and the walk, the home check
// and the start read the system's own, so each is turned to it where it is
// checked or run. One step of a program found by a settings row, with a Home,
// through them all: it is bound, run, and every record keeps the stored slash
// form of the program, and the step's folder is the scratch folder as given.
// Planted: the record keeping the program's path, or the step's folder, in a
// form other than the stored one (both red here; the system's own form is not
// told apart from the slash form on this system).
func TestAStepsStoredSlashPathIsRunAsTheSystemsOwnAndRecordedAsStored(t *testing.T) {
	t.Parallel()
	root, outside := private(t), private(t)
	own := program(t, outside+"/lint-a", marking(outside+"/lint-a"))
	home := outside + "/hm"
	if err := os.MkdirAll(filepath.FromSlash(home), 0o700); err != nil {
		t.Fatal(err)
	}
	tl := stepHomed(home, map[string]map[string]string{root: {"lint-a@1.2.3": own}})
	s := stepScope(t, root, false)
	r := bind(t, tl, s)
	if _, err := stepRunOf(t, r); err != nil {
		t.Fatalf("a step with the home %s: %v", home, err)
	}
	ran := tl.Ran()
	if len(ran) != 2 {
		t.Fatalf("runs = %+v; want the version run and the step", ran)
	}
	for _, rec := range ran {
		if rec.Argv[0] != own {
			t.Errorf("recorded %q; want the stored slash form %q", rec.Argv[0], own)
		}
	}
	if last := ran[1]; last.Shape != "step" || last.Dir != s.Scratch.Dir {
		t.Errorf("the step ran in %q (%s); want the stored slash form %q", last.Dir, last.Shape, s.Scratch.Dir)
	}
}

// TestAStepsPlaceThatIsThereButAnswersNotFoundSaysSoInTheNotFound (replaces
// TestAPlaceThatIsThereButAnswersNotFoundSaysSoInTheNotFound): a link at the
// grant's own place whose target is not there is
// a place that exists, and the walk answers not-there for it. Bind says the
// place is there and what the check answered, in the not-ready it ends in, so
// a find is never read as one of an empty place; or it refuses the link.
// Planted: the walk's absent() answering without the words "the place is there,
// and the check of it answered: ".
func TestAStepsPlaceThatIsThereButAnswersNotFoundSaysSoInTheNotFound(t *testing.T) {
	t.Parallel()
	root := private(t)
	at := root + "/" + binDir + "/" + tool.FileName("lint-a")
	if err := os.MkdirAll(filepath.Dir(at), 0o700); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, root+"/"+binDir+"/gone", at)
	m := atGrant()
	m.Tool = "lint-a"
	s := stepScope(t, root, true)
	s.Manifest = m
	_, err := stepOver(nil, nil).Bind(t.Context(), s)
	if errors.Is(err, port.ErrStepNotReady) {
		if !strings.Contains(err.Error(), "the place is there, and the check of it answered: ") {
			t.Errorf("Bind of a link to nothing = %v, want the not-ready to say the place is there", err)
		}
		return
	}
	if !errors.Is(err, port.ErrRefused) {
		t.Errorf("Bind of a link to nothing = %v, want a not-ready that names the place, or a refusal", err)
	}
}
