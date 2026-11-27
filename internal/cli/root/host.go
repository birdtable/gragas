package root

import (
	"github.com/birdtable/gragas/internal/cli/legacy"
	"github.com/birdtable/gragas/internal/model"
	oldport "github.com/birdtable/gragas/internal/port/legacy"
)

// host is the session as the old commands see it: the screen they end on, the
// lines they say and the record around the run (cli/legacy, M0a-20).
type host struct{ s *session }

var _ legacy.Host = host{}

func (h host) Say(line string)                    { h.s.say(line) }
func (h host) Warn(format string, a ...any)       { h.s.warn(format, a...) }
func (h host) Data(v any)                         { h.s.data = v }
func (h host) Applied(on bool)                    { h.s.applied = on }
func (h host) Change(verb, path string) string    { return h.s.change(verb, path) }
func (h host) WayBack(u oldport.Undo)             { h.s.rec.wayBack(u) }
func (h host) WithRun(what string) string         { return h.s.withRun(what) }
func (h host) Exit(code int) error                { return exitErr(code) }
func (h host) Line(argv []string) (string, error) { return h.s.line(argv) }

func (h host) End(e legacy.Ending, lead ...model.Block) int {
	return h.s.end(ending{word: e.Word, what: e.What, next: e.Next, said: e.Said}, lead...)
}
