package contract_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/contract"
)

const flat = `{"version":1,"tool":"lint","sets":[{"preset":"recommended","entries":["a","b"]}],"observed":["a","b","c"]}`

// TestTheUpstreamReaderReadsOneFileAndRefusesTheRestWhole: a good file is
// read; each wrong one refuses the whole file, and a newer version is
// UpstreamNotReady naming both.
func TestTheUpstreamReaderReadsOneFileAndRefusesTheRestWhole(t *testing.T) {
	t.Parallel()
	u, err := contract.ReadUpstream("lint", []byte(flat))
	if err != nil || len(u.Sets) != 1 || !slices.Equal(u.Observed, []string{"a", "b", "c"}) {
		t.Fatalf("got %+v, %v", u, err)
	}
	for name, doc := range map[string]string{
		"another tool":                 `{"version":1,"tool":"other","sets":[],"observed":[]}`,
		"a member of its own":          `{"version":1,"tool":"lint","sets":[],"observed":[],"extra":1}`,
		"a repeated key":               `{"version":1,"version":1,"tool":"lint","sets":[],"observed":[]}`,
		"a preset twice":               `{"version":1,"tool":"lint","sets":[{"preset":"p","entries":[]},{"preset":"p","entries":[]}],"observed":[]}`,
		"an entry twice":               `{"version":1,"tool":"lint","sets":[],"observed":["a","a"]}`,
		"version zero":                 `{"version":0,"tool":"lint","sets":[],"observed":[]}`,
		"not json":                     `version: 1`,
		"no version":                   `{"tool":"lint","sets":[],"observed":[]}`,
		"no sets or observed":          `{"version":1,"tool":"lint"}`,
		"null members":                 `{"version":1,"tool":"lint","sets":null,"observed":null}`,
		"null items":                   `{"version":1,"tool":"lint","sets":[null],"observed":[null]}`,
		"empty names":                  `{"version":1,"tool":"lint","sets":[{"preset":"","entries":[]}],"observed":[""]}`,
		"a set with no entries member": `{"version":1,"tool":"lint","sets":[{"preset":"p"}],"observed":[]}`,
		"an entry twice in a set":      `{"version":1,"tool":"lint","sets":[{"preset":"p","entries":["a","a"]}],"observed":[]}`,
		"not a slug":                   `{"version":1,"tool":"Lint","sets":[],"observed":[]}`,
	} {
		if _, err := contract.ReadUpstream("lint", []byte(doc)); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	_, err = contract.ReadUpstream("lint", []byte(`{"version":2,"tool":"lint","sets":[],"observed":[]}`))
	var newer *contract.UpstreamNotReady
	if !errors.As(err, &newer) || newer.Found != 2 || newer.Reads != 1 {
		t.Fatalf("got %v", err)
	}
}

// TestANewerVersionIsNotReadyWhateverElseTheFileHolds: the version is read
// before the members, so a file a later version shapes differently ends not
// ready naming both versions, not refused for a member this engine lacks.
func TestANewerVersionIsNotReadyWhateverElseTheFileHolds(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{
		`{"version":2,"tool":"lint","sets":[],"observed":[],"pins":{}}`,
		`{"version":2,"tool":"lint","sets":[],"observed":[{"key":"a"}]}`,
		`{"version":3}`,
	} {
		_, err := contract.ReadUpstream("lint", []byte(doc))
		var newer *contract.UpstreamNotReady
		if !errors.As(err, &newer) || newer.Reads != 1 || newer.Found < 2 {
			t.Errorf("%s: got %v", doc, err)
		}
	}
}

// FuzzReadUpstream: no input panics, and a file that is read is one the schema
// accepts: an object of version 1 for the tool asked for, with all four members
// present, and no preset and no observed entry twice.
func FuzzReadUpstream(f *testing.F) {
	f.Add([]byte(flat))
	f.Fuzz(func(t *testing.T, data []byte) {
		u, err := contract.ReadUpstream("lint", data)
		if err != nil {
			return
		}
		v, derr := canon.Decode(data)
		obj, ok := v.(canon.Object)
		if derr != nil || !ok {
			t.Fatalf("read a file canon does not read as an object: %v", derr)
		}
		_, hasSets := obj["sets"].(canon.Array)
		_, hasObserved := obj["observed"].(canon.Array)
		if obj["version"] != canon.Int(1) || obj["tool"] != canon.String("lint") || !hasSets || !hasObserved {
			t.Fatalf("read a file with version %v, tool %v, sets %v, observed %v", obj["version"], obj["tool"], hasSets, hasObserved)
		}
		presets := map[string]bool{}
		for _, s := range u.Sets {
			if s.Preset == "" || presets[s.Preset] {
				t.Fatalf("a preset named %q, or twice", s.Preset)
			}
			presets[s.Preset] = true
		}
		if slices.Contains(u.Observed, "") || len(slices.Compact(slices.Sorted(slices.Values(u.Observed)))) != len(u.Observed) {
			t.Fatalf("an empty or repeated observed entry: %q", u.Observed)
		}
	})
}

// TestTheUpstreamCapIsHeldAtItsBoundary: a file of exactly MaxUpstream bytes is
// read past the cap, and one byte more is refused before it is parsed.
//
// boundary: MaxUpstream
func TestTheUpstreamCapIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	head := `{"version":1,"tool":"lint","sets":[],"observed":[],"x":"`
	pad := func(n int) []byte {
		return []byte(head + strings.Repeat("a", n-len(head)-2) + `"}`)
	}
	at, over := pad(contract.MaxUpstream), pad(contract.MaxUpstream+1)
	_, err := contract.ReadUpstream("lint", at)
	if err == nil || strings.Contains(err.Error(), "bytes") || strings.Contains(err.Error(), "over") {
		t.Errorf("a file of %d bytes was held by the cap: %v", len(at), err)
	}
	if _, err := contract.ReadUpstream("lint", over); err == nil || !strings.Contains(err.Error(), "over") {
		t.Errorf("a file of %d bytes: got %v, want the cap's refusal", len(over), err)
	}
}
