package wire

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// TestWhatAReadResultMayHold holds a result of a read in have to its files'
// paths and size: no path outside the project or under .git, and a file of
// at most 1 MiB. Risk: a plugin handed a token from .git/config, or one file
// that takes the whole 8 MiB.
func TestWhatAReadResultMayHold(t *testing.T) {
	t.Parallel()
	c := codec(t)
	file := func(path, text string) string {
		return `{"round":0,"step":0,"kind":"read","files":[{"path":"` + path + `","data":{"text":"` + text + `"}}]}`
	}
	rows := []struct{ name, res string }{
		{"a file under .git", file(".git/config", "")},
		{"a file under a nested .git", file("a/.git/config", "")},
		{"a file that climbs", file("../x", "")},
		{"a rooted file", file("/a/b", "")},
		{"a file of 1 MiB and a byte", file("a", strings.Repeat("a", MaxFile+1))},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			var e *RequestError
			if _, err := c.EncodeRequest(request(haveOf(r.res))); !errors.As(err, &e) {
				t.Fatalf("got %T %v, want a *RequestError", err, err)
			}
		})
	}
	if _, err := c.EncodeRequest(request(haveOf(file(".gitignore", "")))); err != nil {
		t.Errorf("the twin, .gitignore: %v", err)
	}
}

// TestAValueOfARankRequestIsHeldToItsCaps holds the preset and found of a
// pair in a request to 64 KiB and 32 levels, as an answer's value is held.
// Risk: a request the engine writes that no plugin may be asked to read.
func TestAValueOfARankRequestIsHeldToItsCaps(t *testing.T) {
	t.Parallel()
	c := codec(t)
	pair := func(v canon.Value) canon.Object {
		return canon.Object{"pairs": canon.Array{canon.Object{"key": canon.String("k"), "aspect": canon.String("value"), "preset": v, "found": canon.Null{}}}}
	}
	big := canon.String(strings.Repeat("a", MaxValue+10))
	var deep canon.Value = canon.Array{}
	for range MaxValueDepth + 8 {
		deep = canon.Array{deep}
	}
	for name, v := range map[string]canon.Value{"a preset over 64 KiB": big, "a preset 41 levels deep": deep} {
		var e *RequestError
		if _, err := c.EncodeRequest(request(pair(v))); !errors.As(err, &e) {
			t.Errorf("%s: %T %v, want a *RequestError", name, err, err)
		}
	}
}

// TestAPlacedFileIsHeldToItsCap holds the data of a placed file to 1 MiB.
// Risk: a plan that places a file the response cap alone holds.
func TestAPlacedFileIsHeldToItsCap(t *testing.T) {
	t.Parallel()
	c := codec(t)
	place := func(n int) []byte {
		return run(`,"place":[{"path":"a","data":{"text":"` + strings.Repeat("a", n) + `"}}]`)
	}
	if _, err := c.DecodePlan(place(MaxFile), 0, grantsOf()); err != nil {
		t.Errorf("a placed file of 1 MiB: %v", err)
	}
	var a *plugin.AnswerError
	if _, err := c.DecodePlan(place(MaxFile+1), 0, grantsOf()); !errors.As(err, &a) {
		t.Errorf("a placed file over 1 MiB: %T %v, want an *plugin.AnswerError", err, err)
	}
}
