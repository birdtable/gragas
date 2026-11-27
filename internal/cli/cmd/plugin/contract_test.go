package plugin_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/plugin"
	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// said is a session that keeps the lines a command said, with the ending.
type said struct {
	kit.Session
	json   bool
	lines  []string
	doc    any
	ending *kit.Ending
}

func (s *said) Say(l string)   { s.lines = append(s.lines, l) }
func (s *said) JSON() bool     { return s.json }
func (s *said) Document(v any) { s.doc = v }
func (s *said) Exit(int) error { return nil }
func (s *said) End(e kit.Ending) int {
	s.ending = &e
	return exit.Done
}

func runContract(t *testing.T, s *said) {
	t.Helper()
	c := plugin.New(s)
	c.SetArgs([]string{"contract"})
	c.SilenceUsage, c.SilenceErrors = true, true
	if err := c.Execute(); err != nil {
		t.Fatalf("plugin contract: %v", err)
	}
}

func (s *said) ApplyFlag() *bool { return new(bool) }

// TestContractIsAWordOfTheRow: // writes: it reads the codec's value and nothing else.
func TestContractIsAWordOfTheRow(t *testing.T) {
	t.Parallel()
	row := plugin.Row()
	if !slices.Contains(row.Then, "contract") || len(row.Writes) != 4 || !strings.Contains(row.Writes[0], "(approve)") || !strings.Contains(row.Writes[1], "(add)") || !strings.Contains(row.Writes[2], "(upgrade)") {
		t.Errorf("row = %+v, want the word contract and the writes of approve, add, upgrade and record", row)
	}
}

// TestPluginContractSaysOneLine: the plain form is one line and one ending.
func TestPluginContractSaysOneLine(t *testing.T) {
	t.Parallel()
	s := &said{}
	runContract(t, s)
	if len(s.lines) != 1 || s.lines[0] != "plugin contract 1, features: cache-vars" {
		t.Errorf("said %q, want the one line of contract 1 with its one feature, cache-vars", s.lines)
	}
	if s.ending == nil || s.ending.Word != model.Done {
		t.Errorf("ending = %+v, want done", s.ending)
	}
}

// TestPluginContractUnderJSONIsOneDocument: the document is the core's value,
// contract 1 and the engine's features, and nothing else is said.
func TestPluginContractUnderJSONIsOneDocument(t *testing.T) {
	t.Parallel()
	s := &said{json: true}
	runContract(t, s)
	d, ok := s.doc.(coreapi.PluginContract)
	if !ok || d.Contract != 1 || !slices.Equal(d.Features, []string{"cache-vars"}) {
		t.Errorf("document = %#v, want contract 1 and the features [cache-vars]", s.doc)
	}
	if len(s.lines) != 0 || s.ending != nil {
		t.Errorf("under --json it said %v and ended %+v", s.lines, s.ending)
	}
}
