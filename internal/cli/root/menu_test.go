package root

import (
	"context"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// places are the four places the working folder can be, in the order the
// transcripts are kept.
func places() []Place { return []Place{PlaceRulebook, PlaceProject, PlaceBoth, PlaceNeither} }

// menuText is the menu as a person reads its rows at cols columns: the label,
// then the blurb wrapped under its own column, a blank line after a row that
// ends a group. The kit draws the list; this is what it is handed.
func menuText(choices []model.Choice, cols int) string {
	width := 0
	for _, c := range choices {
		width = max(width, len([]rune(c.Label)))
	}
	indent := 2 + width + 2
	var b strings.Builder
	for _, c := range choices {
		line := "  " + kit.Column(c.Label, width+2)
		for w := range strings.FieldsSeq(c.Blurb) {
			if len([]rune(line))+len([]rune(w))+1 > cols && strings.TrimSpace(line) != "" && len([]rune(line)) > indent {
				b.WriteString(strings.TrimRight(line, " ") + "\n")
				line = strings.Repeat(" ", indent)
			}
			if len([]rune(line)) > indent {
				line += " "
			}
			line += w
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
		if c.Gap {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// TestTheMenuOfEachPlaceIsAGoldenAtThreeWidths: what the bare menu offers in
// a rulebook, a project, both and neither, at 80, 60 and 40 columns, each
// compared with its golden (tests.generated.5, cli.commands.16).
func TestTheMenuOfEachPlaceIsAGoldenAtThreeWidths(t *testing.T) {
	t.Parallel()
	reg := commands()
	for _, p := range places() {
		for _, cols := range []int{80, 60, 40} {
			name := "menu/" + string(p) + "-" + string(rune('0'+cols/10)) + string(rune('0'+cols%10)) + ".golden"
			configtest.Golden(t, name, []byte(menuText(menuChoices(reg, p), cols)))
		}
	}
}

// Only commands in the tree are offered: today the old check and gen and
// version, never a verb decided and not built, and never the group a place
// leads with while its commands are not built.
func TestTheMenuOffersOnlyWhatIsInTheTree(t *testing.T) {
	t.Parallel()
	reg := commands()
	for _, p := range places() {
		for _, c := range menuChoices(reg, p) {
			row, ok := reg.Find(c.Value)
			if !ok || row.State == kit.NotBuilt {
				t.Errorf("%s: the menu offers %s, which is not in the tree", p, c.Value)
			}
		}
	}
	got := func(p Place) []string {
		var out []string
		for _, c := range menuChoices(reg, p) {
			out = append(out, c.Value)
		}
		return out
	}
	if want := []string{"check", "help", "version", "gen"}; !slices.Equal(got(PlaceProject), want) {
		t.Errorf("project menu = %v, want %v", got(PlaceProject), want)
	}
	if want := []string{"help", "version", "gen"}; !slices.Equal(got(PlaceNeither), want) {
		t.Errorf("neither menu = %v, want %v", got(PlaceNeither), want)
	}
}

// The menu leads with the group of the place: a command of the group is first
// when it is built, and "both" keeps the two groups apart, the rulebook's
// first, with upgrade and propose in each.
func TestTheMenuLeadsWithTheGroupOfThePlace(t *testing.T) {
	t.Parallel()
	rows := commands().Rows()
	for i := range rows {
		switch rows[i].Name {
		case "status", "validate", "upgrade", "propose", "handbook":
			rows[i].State, rows[i].Menu = kit.Built, kit.Menu{Offered: true}
		}
	}
	reg, err := kit.NewRegistry(rows)
	if err != nil {
		t.Fatal(err)
	}
	values := func(p Place) []string {
		var out []string
		for _, c := range menuChoices(reg, p) {
			out = append(out, c.Value)
		}
		return out
	}
	if got, want := values(PlaceProject), []string{"status", "check", "upgrade", "propose", "handbook", "help", "version", "gen"}; !slices.Equal(got, want) {
		t.Errorf("project = %v, want %v", got, want)
	}
	if got := values(PlaceRulebook); slices.Index(got, "validate") != 0 {
		t.Errorf("rulebook = %v, want validate first", got)
	}
	if got, want := values(PlaceBoth), []string{"validate", "upgrade", "propose", "status", "check", "upgrade", "propose"}; !slices.Equal(got[:7], want) {
		t.Errorf("both = %v, want the rulebook group, then the project's, as %v", got, want)
	}
	if got := values(PlaceNeither); got[0] != "handbook" {
		t.Errorf("neither = %v, want the handbook first (init refuses, so it is not offered)", got)
	}
}

// The place is read from the working folder through the Tree port.
func TestThePlaceIsReadFromTheWorkingFolder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  Place
	}{
		{"nothing", map[string]string{"README.md": "x"}, PlaceNeither},
		{"identity file", map[string]string{"rulebook.yaml": "x"}, PlaceRulebook},
		{"schema v2 rulebook", map[string]string{"rulebook/parts.yaml": "x"}, PlaceRulebook},
		{"pin", map[string]string{".gragas/pin.yaml": "x"}, PlaceProject},
		{"both", map[string]string{"rulebook.yaml": "x", ".gragas/pin.yaml": "x"}, PlaceBoth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.Seed(".", tc.files)
			if got := placeOf(w.Ports().Tree); got != tc.want {
				t.Errorf("place = %q, want %q", got, tc.want)
			}
		})
	}
}

// Off a terminal the bare command prints help and ends usage, naming the
// reason on standard error; nothing is asked (cli.commands.12,
// cli.environment.5, cli.environment.8).
func TestTheBareCommandOffATerminalPrintsHelpAndEndsUsage(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f)
	code, sc := execute(context.Background(), t, p)
	if code != exit.Usage {
		t.Fatalf("exit %d, want %d (usage)", code, exit.Usage)
	}
	if got := strings.Join(sc.diag.Said, "\n"); !strings.Contains(got, "no command given") || !strings.Contains(got, "gragas --help") {
		t.Errorf("standard error = %q, want the reason and the line to type", got)
	}
	if len(sc.out.Asked) > 0 || len(sc.out.Picked) > 0 {
		t.Errorf("a question was put to nobody: %v %v", sc.out.Asked, sc.out.Picked)
	}
}

// A run given no argument list is the bare command, whatever os.Args holds:
// cobra reads os.Args for a nil list, and in a test that is the test binary's
// own flags, -update among them. Risk: a run that takes its arguments
// from the process and not from main.
//
//nolint:paralleltest // serial: it sets os.Args, a variable of the whole process
func TestANilArgumentListIsTheBareCommandAndNeverOsArgs(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{saved[0], "--no-such-flag", "check"}
	p, _ := process(t, world(t))
	p, sc := attach(t, p)
	p.Args = nil
	if code := Execute(context.Background(), p); code != exit.Usage {
		t.Fatalf("exit %d, want %d (usage, the bare command off a terminal)", code, exit.Usage)
	}
	if got := strings.Join(sc.diag.Said, "\n"); !strings.Contains(got, "no command given") {
		t.Errorf("standard error = %q, want the bare command's reason", got)
	}
}

// At a terminal the menu is offered through the Terminal port, titled, with
// the line that answers it, and the command picked runs.
func TestTheBareCommandAtATerminalOffersTheMenuAndRunsThePick(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f)
	p.Config = config.Config{StateDir: t.TempDir()}
	p, sc := attach(t, p)
	sc.out.SeedTerminal(porttest.Term{Asking: true, Picks: []string{"version"}})
	sc.out.Seed(".", map[string]string{".gragas/pin.yaml": "x"})
	p.World = sc.out.Ports()
	if code := Execute(context.Background(), p); code != exit.Done {
		t.Fatalf("exit %d, want %d", code, exit.Done)
	}
	if len(sc.out.Picked) != 1 || sc.out.Picked[0] != menuTitle+" ["+menuHint+"]" {
		t.Errorf("picked = %v, want the title and the answering line", sc.out.Picked)
	}
}

// A mistyped command prints the nearest match and the line to type on
// standard error, with nothing on standard output and the usage code
// (cli.commands.10).
func TestAMistypedCommandNamesTheNearestOneOnStderrAndEndsUsage(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, out := process(t, f, "chek")
	code, sc := execute(context.Background(), t, p)
	if code != exit.Usage {
		t.Fatalf("exit %d, want %d", code, exit.Usage)
	}
	if out.String() != "" || sc.output() != "" {
		t.Errorf("standard output = %q %q, want empty", out.String(), sc.output())
	}
	if got := strings.Join(sc.diag.Said, "\n"); !strings.Contains(got, "did you mean `gragas check`?") {
		t.Errorf("standard error = %q, want the nearest command", got)
	}
}

// Forms: a question a command asks names, in code, the flag or argument that
// answers it (cli.environment.14). No command has a form yet; the menu's own
// question is answered by the command typed.
type form struct{ command, question, answer string }

// forms are the questions the tree asks, command by command.
func forms() []form { return nil }

// walkForms checks every form names its answer, and answers how many it saw.
func walkForms(fs []form) (walked int, bad []string) {
	for _, f := range fs {
		walked++
		if strings.TrimSpace(f.answer) == "" {
			bad = append(bad, f.command+": "+f.question)
		}
	}
	return walked, bad
}

func TestEveryFormShowsTheFlagOrArgumentThatAnswersIt(t *testing.T) {
	t.Parallel()
	walked, bad := walkForms(forms())
	if walked != len(forms()) {
		t.Errorf("the walker saw %d of %d forms", walked, len(forms()))
	}
	if len(bad) > 0 {
		t.Errorf("forms with no answer in code: %v", bad)
	}
	if menuHint != "gragas <command>" {
		t.Errorf("the menu's question is answered by %q, want the command typed", menuHint)
	}
}

func TestTheFormWalkerFailsOnAQuestionWithNoAnswerAndCountsWhatItWalked(t *testing.T) {
	t.Parallel()
	walked, bad := walkForms([]form{{"init", "Project or rulebook?", "--kind"}, {"init", "Name?", " "}})
	if walked != 2 || len(bad) != 1 || !strings.Contains(bad[0], "Name?") {
		t.Errorf("walked %d, bad %v, want 2 walked and the unanswered one", walked, bad)
	}
}

// A command whose own row says a group leads with it is offered in that group,
// by rank, after the commands the group's list names, and a row that names no
// group is offered after the groups as before.
func TestARowCanSayWhichGroupLeadsWithIt(t *testing.T) {
	t.Parallel()
	rows := commands().Rows()
	for i := range rows {
		switch rows[i].Name {
		case "version":
			rows[i].Menu = kit.Menu{Offered: true, Lead: "project", Rank: 1}
		case "help":
			rows[i].Menu = kit.Menu{Offered: true, Lead: "project", Rank: 2}
		}
	}
	var got []string
	for _, c := range menuChoices(kit.Of(rows), PlaceProject) {
		got = append(got, c.Value)
	}
	if want := []string{"check", "version", "help", "gen"}; !slices.Equal(got, want) {
		t.Errorf("project = %v, want %v: version then help by rank, led after check, and gen after the group", got, want)
	}
	var plain []string
	for _, c := range menuChoices(commands(), PlaceProject) {
		plain = append(plain, c.Value)
	}
	if want := []string{"check", "help", "version", "gen"}; !slices.Equal(plain, want) {
		t.Errorf("without a Lead the project menu = %v, want %v", plain, want)
	}
}
