package plugin

import (
	"errors"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// fixtureDir is the contract's valid and invalid documents, read as files.
const fixtureDir = "../../testdata/plugin-v1"

// codec is a generated type seen from outside: it reads a Value and writes
// one back.
type codec interface {
	FromCanon(canon.Value) error
	ToCanon() canon.Value
}

// kinds is each fixture file, by name, and a fresh value of the type that
// reads one document of it.
func kinds() map[string]func() codec {
	return map[string]func() codec{
		"request":          func() codec { return &Request{} },
		"response":         func() codec { return &Response{} },
		"error":            func() codec { return &Error{} },
		"plan.request":     func() codec { return &PlanRequest{} },
		"plan.response":    func() codec { return &PlanResponse{} },
		"entries.request":  func() codec { return &EntriesRequest{} },
		"entries.response": func() codec { return &EntriesResponse{} },
		"findings.request": func() codec { return &FindingsRequest{} },
		"findings.response": func() codec {
			return &FindingsResponse{}
		},
		"rank.request":    func() codec { return &RankRequest{} },
		"rank.response":   func() codec { return &RankResponse{} },
		"preset.request":  func() codec { return &PresetRequest{} },
		"preset.response": func() codec { return &PresetResponse{} },
		"manifest":        func() codec { return &Manifest{} },
	}
}

func docs(t *testing.T, name string) canon.Array {
	t.Helper()
	b, err := os.ReadFile(path.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	v, err := canon.Decode(b)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	a, ok := v.(canon.Array)
	if !ok {
		t.Fatalf("%s is not a list of documents", name)
	}
	return a
}

// TestEveryValidDocumentWritesBackItself plants a valid document of each kind:
// read into its type and written back, it is the same bytes in canonical form.
// A field the type drops, a default it adds and an order it changes each fail
// here, red first.
func TestEveryValidDocumentWritesBackItself(t *testing.T) {
	t.Parallel()
	for name, fresh := range kinds() {
		for i, d := range docs(t, name+".valid.json") {
			x := fresh()
			if err := x.FromCanon(d); err != nil {
				t.Errorf("%s[%d]: a valid document is refused: %v", name, i, err)
				continue
			}
			want, err := canon.Encode(d, canon.Canonical)
			if err != nil {
				t.Fatal(err)
			}
			got, err := canon.Encode(x.ToCanon(), canon.Canonical)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(name, "request") {
				// A request is an open object: a member the type does not
				// name is ignored, so the proof is that what is written
				// reads back and writes the same.
				y := fresh()
				if err := y.FromCanon(x.ToCanon()); err != nil {
					t.Errorf("%s[%d]: what it wrote is refused: %v", name, i, err)
					continue
				}
				want, _ = canon.Encode(y.ToCanon(), canon.Canonical)
			}
			if string(got) != string(want) {
				t.Errorf("%s[%d]: written back\n got %s\nwant %s", name, i, got, want)
			}
		}
	}
}

// refusals is a document of a type that its FromCanon must refuse, with the
// JSON Pointer it refuses at: one row a rule the types hold.
type refusal struct {
	name string
	of   func() codec
	json string
	at   string
}

func refusals() []refusal {
	return []refusal{
		{"null outside a value", func() codec { return &Error{} }, `{"code":"bug","message":null}`, "/message"},
		{"an unknown member of a closed object", func() codec { return &Error{} }, `{"code":"bug","message":"m","x":1}`, "/x"},
		{"a missing required member", func() codec { return &Error{} }, `{"code":"bug"}`, ""},
		{"a code outside its enum", func() codec { return &Error{} }, `{"code":"later","message":"m"}`, "/code"},
		{"a string where a list is", func() codec { return &RankResponse{} }, `{"words":"a"}`, "/words"},
		{"a float where a whole number is", func() codec { return &Finding{} }, `{"key":"k","message":"m","line":1.5}`, "/line"},
		{"a line below its range", func() codec { return &Finding{} }, `{"key":"k","message":"m","line":0}`, "/line"},
		{"an empty key", func() codec { return &Entry{} }, `{"key":"","on":true}`, "/key"},
	}
}

// TestWhatTheTypesHoldIsRefused plants each refusal and holds the pointer it
// is refused at. A refusal that moves is a fault the plugin's author reads, so
// the place is part of the row.
func TestWhatTheTypesHoldIsRefused(t *testing.T) {
	t.Parallel()
	for _, r := range refusals() {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			v, err := canon.Decode([]byte(r.json))
			if err != nil {
				t.Fatal(err)
			}
			err = r.of().FromCanon(v)
			var f *Fault
			if !errors.As(err, &f) {
				t.Fatalf("refused with %v, want a *Fault at %q", err, r.at)
			}
			if f.At != r.at {
				t.Errorf("refused at %q (%s), want %q", f.At, f.Msg, r.at)
			}
		})
	}
}

// TestAListThatHoldsEachOnceRefusesARepeat plants a manifest whose needs name
// one thing twice, and holds the refusal at the second.
func TestAListThatHoldsEachOnceRefusesARepeat(t *testing.T) {
	t.Parallel()
	d := docs(t, "manifest.valid.json")[0].(canon.Object)
	d["needs"] = canon.Array{canon.String("a"), canon.String("a")}
	err := (&Manifest{}).FromCanon(d)
	var f *Fault
	if !errors.As(err, &f) || f.At != "/needs/1" {
		t.Fatalf("refused with %v, want a *Fault at /needs/1", err)
	}
}

// patched is d with one RFC 6902 operation applied: add, remove or replace, at
// a pointer into arrays and objects. It copies what it changes.
func patched(t *testing.T, d canon.Value, op canon.Object) canon.Value {
	t.Helper()
	ptr := string(op["path"].(canon.String))
	parts := strings.Split(strings.TrimPrefix(ptr, "/"), "/")
	var walk func(v canon.Value, parts []string) canon.Value
	walk = func(v canon.Value, parts []string) canon.Value {
		k := strings.NewReplacer("~1", "/", "~0", "~").Replace(parts[0])
		last := len(parts) == 1
		switch x := v.(type) {
		case canon.Object:
			c := maps.Clone(x)
			switch {
			case last && op["op"] == canon.String("remove"):
				delete(c, k)
			case last:
				c[k] = op["value"]
			default:
				c[k] = walk(x[k], parts[1:])
			}
			return c
		case canon.Array:
			i, err := strconv.Atoi(k)
			if err != nil || i < 0 || i > len(x) {
				t.Fatalf("pointer %s: %q is no index", ptr, k)
			}
			c := slices.Clone(x)
			switch {
			case last && op["op"] == canon.String("remove"):
				c = slices.Delete(c, i, i+1)
			case last && op["op"] == canon.String("add"):
				c = slices.Insert(c, i, op["value"])
			case last:
				c[i] = op["value"]
			default:
				c[i] = walk(x[i], parts[1:])
			}
			return c
		}
		t.Fatalf("pointer %s: nothing to step into", ptr)
		return nil
	}
	return walk(d, parts)
}

// TestEveryInvalidFixtureIsRefusedByItsType applies each invalid fixture's
// patch to the first valid document of its kind and holds that the type's
// FromCanon refuses the result: every invalid fixture is a rule the types
// hold. A rule dropped from the generator fails here, red first.
func TestEveryInvalidFixtureIsRefusedByItsType(t *testing.T) {
	t.Parallel()
	for name, fresh := range kinds() {
		base := docs(t, name+".valid.json")
		for _, row := range docs(t, name+".invalid.json") {
			r := row.(canon.Object)
			label := name + ": " + string(r["name"].(canon.String))
			var d canon.Value = base
			for _, op := range r["patch"].(canon.Array) {
				d = patched(t, d, op.(canon.Object))
			}
			at := strings.SplitN(string(r["patch"].(canon.Array)[0].(canon.Object)["path"].(canon.String)), "/", 3)[1]
			i, _ := strconv.Atoi(at)
			if err := fresh().FromCanon(d.(canon.Array)[i]); err != nil {
				continue
			}
			t.Errorf("%s: a type reads what the schema refuses (%s)", label, r["problem"])
		}
	}
}
