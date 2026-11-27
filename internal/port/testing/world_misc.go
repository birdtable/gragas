package porttest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/birdtable/gragas/internal/model"
)

// plant is a finding the Schema answers for one document.
type plant struct {
	name string
	doc  []byte
	out  []model.Problem
}

// SeedSchemas makes each name a schema that was compiled: a document checked
// against it is valid unless a Plant says otherwise.
func (w *World) SeedSchemas(names ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, n := range names {
		w.schemas[n] = true
	}
}

// Plant makes Check of exactly doc against the schema name answer problems.
func (w *World) Plant(name string, doc []byte, problems ...model.Problem) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.plants = append(w.plants, plant{name: name, doc: slices.Clone(doc), out: slices.Clone(problems)})
}

type schemaPort struct{ w *World }

func (x schemaPort) Check(name string, doc []byte) ([]model.Problem, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if !x.w.schemas[name] {
		return []model.Problem{{Msg: fmt.Sprintf("the schema %q was not compiled", name)}}, nil
	}
	if !json.Valid(doc) {
		return nil, fmt.Errorf("the document is not JSON")
	}
	for _, p := range x.w.plants {
		if p.name == name && bytes.Equal(p.doc, doc) {
			out := slices.Clone(p.out)
			sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
			return out[:min(len(out), 10)], nil
		}
	}
	return nil, nil
}

// clock is the instant the Clock answers.
type clock struct{ at time.Time }

// Instant fixes the Clock at at, in UTC.
func (w *World) Instant(at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clock = &clock{at: at.UTC()}
}

// now is the fixed instant, or fails the test for call. The caller holds w.mu.
func (w *World) now(call string) (time.Time, error) {
	if w.clock == nil {
		return time.Time{}, w.unseeded(call, "the instant was never fixed; call Instant")
	}
	return w.clock.at, nil
}

type clockPort struct{ w *World }

func (x clockPort) Now() time.Time {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	at, err := x.w.now("Clock.Now")
	if err != nil {
		return time.Time{} // the test is already failed; Now has no error to answer
	}
	return at
}

// SeedAssets makes the embedded files files, by slash path.
func (w *World) SeedAssets(files map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.assets = maps.Clone(files)
}

type assetsPort struct{ w *World }

func (x assetsPort) Read(p string) ([]byte, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if c, ok := x.w.assets[p]; ok {
		return []byte(c), nil
	}
	return nil, notExist("read", p)
}

func (x assetsPort) List(dir string) ([]string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if err := inside(dir); err != nil {
		return nil, err
	}
	prefix := ""
	if dir != "" && dir != "." {
		prefix = dir + "/"
	}
	out := []string{}
	for p := range x.w.assets {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Term is what the Terminal is scripted with.
type Term struct {
	// Width is the columns, and Colors the bits of colour; 0 for neither.
	Width, Colors int
	// Answers are what Ask reads, one per question, in order.
	Answers []string
	// Input is what Line reads, one line per call, in order.
	Input []string
	// Asking is whether CanAsk is true, and Picks the values Pick answers, one
	// per call, in order; an empty one is backing out.
	Asking bool
	Picks  []string
}

type term struct{ Term }

// SeedTerminal scripts the Terminal. Until a test calls it, every method of
// the Terminal is a call it did not seed.
func (w *World) SeedTerminal(t Term) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.term = &term{Term{Width: t.Width, Colors: t.Colors, Answers: slices.Clone(t.Answers), Input: slices.Clone(t.Input), Asking: t.Asking, Picks: slices.Clone(t.Picks)}}
}

// terminal is the scripted terminal, or nil after failing the test for call.
// The caller holds w.mu.
func (w *World) terminal(call string) (*term, error) {
	if w.term == nil {
		return nil, w.unseeded(call, "no terminal was scripted; call SeedTerminal")
	}
	return w.term, nil
}

type terminalPort struct{ w *World }

func (x terminalPort) Width() int {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.terminal("Terminal.Width")
	if err != nil {
		return 0 // the test is already failed; Width has no error to answer
	}
	return t.Width
}

func (x terminalPort) Colors() int {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.terminal("Terminal.Colors")
	if err != nil {
		return 0 // the test is already failed; Colors has no error to answer
	}
	return t.Colors
}

func (x terminalPort) Ask(ctx context.Context, question string) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	t, err := x.w.terminal("Terminal.Ask")
	if err != nil {
		return "", err
	}
	x.w.Asked = append(x.w.Asked, question)
	if len(t.Answers) == 0 {
		return "", io.EOF
	}
	a := t.Answers[0]
	t.Answers = t.Answers[1:]
	return a, nil
}

func (x terminalPort) Say(text string) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.terminal("Terminal.Say"); err != nil {
		return err
	}
	x.w.Said = append(x.w.Said, strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text))
	return nil
}

func (x terminalPort) Line(ctx context.Context) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	t, err := x.w.terminal("Terminal.Line")
	if err != nil {
		return "", err
	}
	if len(t.Input) == 0 {
		return "", io.EOF
	}
	l := t.Input[0]
	t.Input = t.Input[1:]
	return l, nil
}

func (x terminalPort) Banner(version string) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.terminal("Terminal.Banner"); err != nil {
		return err
	}
	x.w.Banners = append(x.w.Banners, version)
	return nil
}

func (x terminalPort) Show(screen model.Screen) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.terminal("Terminal.Show"); err != nil {
		return err
	}
	if len(screen.Blocks()) == 0 {
		return errors.New("show: the screen has no block; build one with model.NewScreen")
	}
	x.w.Shown = append(x.w.Shown, screen)
	return nil
}

// CanAsk is whether the script lets a question be answered: Term.Asking.
func (x terminalPort) CanAsk() bool {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.terminal("Terminal.CanAsk")
	return err == nil && t.Asking
}

// Pick answers the next of Term.Picks and records its title in World.Picked.
// With no choice, no one to ask or no pick left, nothing is picked.
func (x terminalPort) Pick(title string, choices []model.Choice, flag string) (string, bool) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.terminal("Terminal.Pick")
	if err != nil || !t.Asking || len(choices) == 0 {
		return "", false
	}
	x.w.Picked = append(x.w.Picked, title+" ["+flag+"]")
	if len(t.Picks) == 0 {
		return "", false
	}
	v := t.Picks[0]
	t.Picks = t.Picks[1:]
	return v, v != ""
}
