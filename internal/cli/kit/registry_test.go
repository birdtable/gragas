package kit_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

func row(name string, state kit.State) kit.Command {
	return kit.Command{Name: name, Context: kit.Anywhere, Ends: kit.EndsListing, State: state, Summary: "Say " + name,
		Examples: [2]kit.Example{{Line: "gragas " + name, Does: "say it"}, {Line: "gragas " + name + " --json", Does: "as JSON"}},
		Exits:    []model.Word{model.Done}, JSONKind: name}
}

// TestTreeProblemsSaysWhereTreeAndRegistryDisagree: a name the tree holds and
// the registry lacks or has not built, and a built row the tree lacks; cobra's
// help and completion are no commands of ours.
func TestTreeProblemsSaysWhereTreeAndRegistryDisagree(t *testing.T) {
	t.Parallel()
	reg := kit.Of([]kit.Command{row("check", kit.Built), row("gen", kit.Legacy), row("waive", kit.NotBuilt)})
	if got := reg.TreeProblems([]string{"check", "gen", "help", "completion"}); len(got) != 0 {
		t.Errorf("a tree that agrees is told %v", got)
	}
	want := []string{
		"gen is legacy in the registry and not in the tree",
		"stranger is in the tree and not in the registry",
		"waive is on the not-built list and in the tree",
	}
	if got := reg.TreeProblems([]string{"check", "stranger", "waive"}); !slices.Equal(got, want) {
		t.Errorf("TreeProblems =\n %q\nwant\n %q", got, want)
	}
}

// TestUsageIsHowARowIsTyped: a flat name alone, a group with its words.
func TestUsageIsHowARowIsTyped(t *testing.T) {
	t.Parallel()
	if got := (kit.Command{Name: "check"}).Usage(); got != "check" {
		t.Errorf("Usage = %q", got)
	}
	if got := (kit.Command{Name: "rule", Then: []string{"new", "edit"}}).Usage(); got != "rule (new, edit)" {
		t.Errorf("Usage = %q", got)
	}
}

// TestARowThatBreaksARuleIsRefused: NewRegistry takes a good row and refuses a
// name declared twice.
func TestARowThatBreaksARuleIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := kit.NewRegistry([]kit.Command{row("check", kit.Built)}); err != nil {
		t.Fatal(err)
	}
	if _, err := kit.NewRegistry([]kit.Command{row("check", kit.Built), row("check", kit.Built)}); err == nil {
		t.Error("a name declared twice was taken")
	}
}

// TestARowThatTakesSARIFEndsOnFindings: --sarif is for a command that lists
// findings, so a row that takes it and never ends on findings is refused.
func TestARowThatTakesSARIFEndsOnFindings(t *testing.T) {
	t.Parallel()
	lists := row("check", kit.Built)
	lists.SARIF, lists.Exits = true, []model.Word{model.Done, model.Findings}
	if _, err := kit.NewRegistry([]kit.Command{lists}); err != nil {
		t.Fatal(err)
	}
	never := row("check", kit.Built)
	never.SARIF = true
	_, err := kit.NewRegistry([]kit.Command{never})
	if err == nil || !strings.Contains(err.Error(), "it takes --sarif and never ends on findings") {
		t.Errorf("a row that takes --sarif and never ends on findings: %v, want refused", err)
	}
}

func ordered(c kit.Command, order int) kit.Command {
	c.Order = order
	return c
}

// TestAssembleGivesTheTablesRowsTheFreeSlotsInOrder: a row with no Order takes
// the next free slot of ten, skipping one a package claimed, and the result
// stays in Order, so a package that sits at a slot of the table keeps its place.
func TestAssembleGivesTheTablesRowsTheFreeSlotsInOrder(t *testing.T) {
	t.Parallel()
	reg := kit.Assemble(
		[]kit.Command{row("check", kit.Built), row("gen", kit.Built), row("docs", kit.Built)},
		[]kit.Command{ordered(row("show", kit.Built), 20)})
	var got []string
	for _, c := range reg.Rows() {
		got = append(got, c.Name+":"+string(rune('0'+c.Order/10)))
	}
	if want := []string{"check:1", "show:2", "gen:3", "docs:4"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

// TestTwoCommandsCannotClaimOneSlot, and an order below nothing is refused.
func TestTwoCommandsCannotClaimOneSlot(t *testing.T) {
	t.Parallel()
	_, err := kit.NewRegistry([]kit.Command{ordered(row("check", kit.Built), 10), ordered(row("gen", kit.Built), 10)})
	if err == nil || !strings.Contains(err.Error(), "gen: the order 10 is check's already") {
		t.Errorf("a slot claimed twice: %v", err)
	}
	if _, err := kit.NewRegistry([]kit.Command{ordered(row("check", kit.Built), -1)}); err == nil {
		t.Error("an order below 0 was taken")
	}
	if _, err := kit.NewRegistry([]kit.Command{row("check", kit.Built), row("gen", kit.Built)}); err != nil {
		t.Errorf("two rows that declare no order are refused: %v", err)
	}
}
