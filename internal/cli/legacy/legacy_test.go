package legacy

import (
	"errors"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
	oldport "github.com/birdtable/gragas/internal/port/legacy"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// recorder is the host a check ends on: it keeps the blocks of the answer.
type recorder struct{ lead []model.Block }

func (r *recorder) Say(string)          {}
func (r *recorder) Warn(string, ...any) {}
func (r *recorder) End(_ Ending, lead ...model.Block) int {
	r.lead = lead
	return 0
}
func (r *recorder) Data(any)                           {}
func (r *recorder) Applied(bool)                       {}
func (r *recorder) Change(verb, path string) string    { return verb + " " + path }
func (r *recorder) WayBack(oldport.Undo)               {}
func (r *recorder) WithRun(what string) string         { return what }
func (r *recorder) Exit(code int) error                { return errors.New("exit " + strconv.Itoa(code)) }
func (r *recorder) Line(argv []string) (string, error) { return strings.Join(argv, " "), nil }

// rulebook is the small valid rulebook of internal/cli/root's testdata, in
// memory, read from there by path.
func rulebook(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for _, p := range []string{"rulebook/parts.yaml", "rulebook/products.yaml", "rulebook/sources.yaml", "rulebook/baseline.json",
		"rulebook/base/arch/rules/layers.yaml", "rulebook/base/arch/tools.yaml", "rulebook/lang/go/rules/errors.yaml", "presets/layers.yml"} {
		data, err := os.ReadFile(path.Join("..", "root", "testdata", p))
		if err != nil {
			t.Fatal(err)
		}
		m[p] = &fstest.MapFile{Data: data}
	}
	return m
}

// checked runs the check command over a rulebook whose every document gives
// n problems, and answers how many findings it printed and its closing note.
func checked(t *testing.T, n int) (printed int, note string) {
	t.Helper()
	probs := make([]oldport.Problem, n)
	for i := range probs {
		probs[i] = oldport.Problem{Line: i + 1, Msg: "planted " + strconv.Itoa(i)}
	}
	f := &porttest.Fake{Repo: rulebook(t), Problems: probs, NamesErr: errors.New("no names file here")}
	h := &recorder{}
	c := &commands{h: h, d: Deps{Ports: f.Ports()}}
	c.check(t.Context(), ".")
	for _, b := range h.lead {
		switch b := b.(type) {
		case model.List:
			printed = len(b.Items)
		case model.Note:
			if strings.HasPrefix(b.Text, "... and ") {
				note = b.Text
			}
		}
	}
	return printed, note
}

// TestCheckPrintsMaxShownFindingsAndCountsTheRest (boundary: maxShown): a run
// with maxShown findings prints them all and no note, and one with a finding
// more prints maxShown and says it left one out, so the cut is the cap's.
func TestCheckPrintsMaxShownFindingsAndCountsTheRest(t *testing.T) {
	t.Parallel()
	printed, note := checked(t, maxShown)
	if printed != maxShown || note != "" {
		t.Errorf("maxShown findings: printed %d with note %q, want all of them and no note", printed, note)
	}
	printed, note = checked(t, maxShown+1)
	if printed != maxShown || note != "... and 1 more" {
		t.Errorf("maxShown+1 findings: printed %d with note %q, want %d and the note of 1 more", printed, note, maxShown)
	}
}
