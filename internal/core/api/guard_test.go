package coreapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The guard's book: one rule with a scope and reasoning prose, and a bindings/
// record, so each kind of lifecycle field S59 names has a line to edit.
const guardRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
    scope:
      paths: ["cmd/**"]
    reasoning:
      definition: An error says what failed.
`

const (
	guardRoot    = "/repo"
	guardRuleAt  = "/repo/book/language/go/rules/errors.yaml"
	guardBinding = "/repo/book/bindings/go-errors-1.yaml"
)

func guardWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	return guardWorldOf(t, guardRules)
}

// guardWorldOf is guardWorld with rules as the rule file's text.
func guardWorldOf(t *testing.T, rules string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC))
	root := w.Seed(guardRoot, map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure,
		"book/language/go/rules/errors.yaml": rules,
		"book/bindings/go-errors-1.yaml":     "subject: go.errors.1\nvalue: 1\n",
	})
	return w, root
}

// hook is the hook's input for one tool call, as the agent's hook sends it.
func hook(t *testing.T, tool string, input map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"session_id": "s", "hook_event_name": "PreToolUse", "tool_name": tool, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func edit(t *testing.T, file, old, new string) []byte {
	t.Helper()
	return hook(t, "Edit", map[string]any{"file_path": file, "old_string": old, "new_string": new})
}

// repeated is an Edit's hook input whose tool_input holds the keys and
// values in order as given, a key repeated as often as it is named: what
// json.Marshal of a map cannot write. Each planted input of it is one that a
// reader of the last value would allow.
func repeated(t *testing.T, kv ...string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{`)
	for i := 0; i+1 < len(kv); i += 2 {
		k, kerr := json.Marshal(kv[i])
		v, verr := json.Marshal(kv[i+1])
		if kerr != nil || verr != nil {
			t.Fatal(kerr, verr)
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s:%s", k, v)
	}
	b.WriteString("}}")
	return []byte(b.String())
}

func judge(t *testing.T, p port.Ports, root model.Root, input []byte) (model.Guard, *model.Refusal) {
	t.Helper()
	g, ref, err := coreapi.GuardEdit(t.Context(), p, coreapi.GuardAsk{Root: root, Input: input, Run: "guard-run", Engine: "v0.0.0-test"})
	if err != nil {
		t.Fatalf("GuardEdit: %v", err)
	}
	return g, ref
}

// refusals are the journal's records of guard edit's refusals at root.
func refusals(t *testing.T, w *porttest.World, root model.Root) []map[string]any {
	t.Helper()
	recs, _, err := w.Ports().Journal.Records(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, r := range recs {
		if r.Command != coreapi.GuardCommand {
			continue
		}
		if r.Kind != model.KindClose {
			t.Errorf("the guard wrote a %s record; a refusal is one close", r.Kind)
		}
		var d map[string]any
		if err := json.Unmarshal(r.Detail, &d); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

// TestAHandEditOfALifecycleFieldIsRefusedNamingTheFieldAndTheCommand: the
// planted edits, each a hand change of one kind of lifecycle field,
// ends refused, names the field and a command, and is journalled once.
// Risk: an agent edits a status, an id, a scope or a binding by hand and the
// change lands without its history entry.
func TestAHandEditOfALifecycleFieldIsRefusedNamingTheFieldAndTheCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, field, command string
		input                []byte
	}{
		{"status", "status", "gragas rule promote go.errors.1", edit(t, guardRuleAt, "status: proposed", "status: active")},
		{"retire by hand", "status", "gragas rule retire go.errors.1", edit(t, guardRuleAt, "status: proposed", "status: retired")},
		{"id", "id", "gragas rule", edit(t, guardRuleAt, "id: go.errors.1", "id: go.errors.7")},
		{"scope", "scope", "gragas rule edit go.errors.1 --scope", edit(t, guardRuleAt, `paths: ["cmd/**"]`, `paths: ["internal/**"]`)},
		{"binding record", "records", "gragas bind", edit(t, guardBinding, "value: 1", "value: 2")},
		{"a new binding record", "records", "gragas bind", hook(t, "Write", map[string]any{"file_path": "/repo/book/bindings/go-errors-1b.yaml", "content": "subject: go.errors.1\nvalue: 3\n"})},
		{"strength by MultiEdit", "strength", "gragas rule replace go.errors.1", hook(t, "MultiEdit", map[string]any{"file_path": guardRuleAt, "edits": []map[string]any{
			{"old_string": "An error says what failed.", "new_string": "An error says what failed, and why."},
			{"old_string": "strength: must", "new_string": "strength: may"},
		}})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, root := guardWorld(t)
			g, ref := judge(t, w.Ports(), root, c.input)
			if ref == nil {
				t.Fatalf("the edit was allowed: %+v", g)
			}
			if ref.Word() != model.Refused {
				t.Fatalf("the word is %s", ref.Word())
			}
			i := slices.IndexFunc(g.Changes, func(x model.Guarded) bool { return x.Field == c.field })
			if i < 0 {
				t.Fatalf("no change of %s among %+v: %s", c.field, g.Changes, ref.Observed())
			}
			if !strings.Contains(g.Changes[i].Command, c.command) || !strings.Contains(ref.Observed(), c.field) || ref.Hint() == "" {
				t.Fatalf("change %+v, why %q, hint %q", g.Changes[i], ref.Observed(), ref.Hint())
			}
			if got := refusals(t, w, root); len(got) != 1 || got[0]["outcome"] != "refused" || got[0]["path"] != g.Path {
				t.Fatalf("the journal holds %+v", got)
			}
			if after, _ := w.File(root, strings.TrimPrefix(guardRuleAt, guardRoot+"/")); after != guardRules {
				t.Fatal("the guard wrote the file it judged")
			}
		})
	}
}

// TestAnEditOfProseIsAllowedAndJournalsNothing is the clean control: the
// reasoning is prose (S59), and an edit of it ends done.
func TestAnEditOfProseIsAllowedAndJournalsNothing(t *testing.T) {
	t.Parallel()
	w, root := guardWorld(t)
	for _, in := range [][]byte{
		edit(t, guardRuleAt, "An error says what failed.", "An error says what failed and what was being done."),
		edit(t, guardRuleAt, "statement: Wrap an error.", "statement: Wrap an error with what was being done."),
		hook(t, "Write", map[string]any{"file_path": "/repo/notes.md", "content": "a note\n"}),
	} {
		g, ref := judge(t, w.Ports(), root, in)
		if ref != nil || len(g.Changes) != 0 {
			t.Fatalf("%s: refused %v, changes %+v", in, ref, g.Changes)
		}
	}
	if got := refusals(t, w, root); len(got) != 0 {
		t.Fatalf("an allowed edit was journalled: %+v", got)
	}
}

// TestWhatTheGuardCannotReadOrPlaceIsRefusedNeverAllowed: an input that is
// empty, not JSON, of another tool, for a file outside the root, or an edit
// the tool itself would refuse is refused and journalled.
// Risk: a guard that cannot read the edit lets it through.
func TestWhatTheGuardCannotReadOrPlaceIsRefusedNeverAllowed(t *testing.T) {
	t.Parallel()
	cases := map[string][]byte{
		"empty":                               nil,
		"not JSON":                            []byte("status: active"),
		"two documents":                       append(edit(t, guardRuleAt, "a", "b"), edit(t, guardRuleAt, "a", "b")...),
		"an allowed edit, then a second":      append(edit(t, guardRuleAt, "An error says what failed.", "An error tells what failed."), edit(t, guardRuleAt, "status: proposed", "status: active")...),
		"another tool":                        hook(t, "Bash", map[string]any{"file_path": guardRuleAt, "command": "sed -i s/a/b/"}),
		"no tool_input":                       []byte(`{"tool_name":"Edit"}`),
		"no file_path":                        hook(t, "Write", map[string]any{"content": "x"}),
		"a path outside":                      edit(t, "/elsewhere/errors.yaml", "a", "b"),
		"a path that dots":                    edit(t, "/repo/../elsewhere/errors.yaml", "a", "b"),
		"a relative path":                     edit(t, "book/language/go/rules/errors.yaml", "a", "b"),
		"old_string missing":                  edit(t, guardRuleAt, "status: trial", "status: active"),
		"old_string twice":                    edit(t, guardRuleAt, "go.errors", "go.errs"),
		"an edit of a file that is not there": edit(t, "/repo/book/missing.yaml", "a", "b"),
		"an edit that breaks the file":        edit(t, guardRuleAt, "rules:", "rules: ["),
		"a file_path twice":                   repeated(t, "file_path", "/elsewhere/errors.yaml", "file_path", guardRuleAt, "old_string", "An error says what failed.", "new_string", "An error tells what failed."),
		"a new_string twice":                  repeated(t, "file_path", guardRuleAt, "old_string", "status: proposed", "new_string", "status: active", "new_string", "status: proposed"),
		"a key spelled in another case":       repeated(t, "file_path", "/elsewhere/errors.yaml", "File_Path", guardRuleAt, "old_string", "An error says what failed.", "new_string", "An error tells what failed."),
		"a tool_name twice":                   bytes.Replace(edit(t, guardRuleAt, "An error says what failed.", "An error tells what failed."), []byte(`"tool_name":"Edit"`), []byte(`"tool_name":"Bash","tool_name":"Edit"`), 1),
		"an old_string twice in a MultiEdit":  []byte(fmt.Sprintf(`{"tool_name":"MultiEdit","tool_input":{"file_path":%q,"edits":[{"old_string":"status: proposed","old_string":"An error says what failed.","new_string":"An error tells what failed."}]}}`, guardRuleAt)),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := guardWorld(t)
			_, ref := judge(t, w.Ports(), root, input)
			if ref == nil || ref.Word() != model.Refused || ref.Hint() == "" {
				t.Fatalf("not refused with a hint: %v", ref)
			}
			if got := refusals(t, w, root); len(got) != 1 {
				t.Fatalf("the journal holds %d refusals", len(got))
			}
		})
	}
}

// TestTheGuardRefusesAPathThatIsNotClean.
// Planted: with book/l a folder link, an allowed edit at book/l/../ and the
// rule file's path below it is refused, the path named and the hint saying to
// write it clean, as are a . part and a doubled separator. With main's world
// and guard (f12c8fa4d), the world cleaned the path past l and all three
// edits were allowed; with the guard's ErrNotClean case taken out, they are
// refused with the wrong words, saying neither cause nor cure. Controls: the same edit by the clean path is allowed, as today,
// and a clean path through the same link is refused as a link, as today.
// Risk: Within follows l before the .., as the kernel does, while the host's
// own tool applies the .. to the spelling, so the guard judges one file and
// the host writes another.
func TestTheGuardRefusesAPathThatIsNotClean(t *testing.T) {
	t.Parallel()
	const prose, more = "An error says what failed.", "An error tells what failed."
	for name, at := range map[string]string{
		"a .. after a folder link": "/repo/book/l/../language/go/rules/errors.yaml",
		"a . part":                 "/repo/book/./language/go/rules/errors.yaml",
		"a doubled separator":      "/repo/book//language/go/rules/errors.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := guardWorld(t)
			w.Link(root, "book/l", "language/go")
			_, ref := judge(t, w.Ports(), root, edit(t, at, prose, more))
			if ref == nil || ref.Word() != model.Refused {
				t.Fatalf("%s: not refused: %v", at, ref)
			}
			if !strings.Contains(ref.Observed(), "not clean") || !strings.Contains(ref.Observed(), at) || !strings.Contains(ref.Hint(), "clean absolute path") {
				t.Errorf("the refusal neither names the path nor says to write it clean: %s / %s", ref.Observed(), ref.Hint())
			}
			if got := refusals(t, w, root); len(got) != 1 {
				t.Fatalf("the journal holds %d refusals", len(got))
			}
		})
	}
	t.Run("the clean path", func(t *testing.T) {
		t.Parallel()
		w, root := guardWorld(t)
		w.Link(root, "book/l", "language/go")
		g, ref := judge(t, w.Ports(), root, edit(t, guardRuleAt, prose, more))
		if ref != nil || g.Path != "book/language/go/rules/errors.yaml" {
			t.Fatalf("a clean path through no link is not judged as before: %v at %q", ref, g.Path)
		}
	})
	t.Run("a clean path through the same link", func(t *testing.T) {
		t.Parallel()
		w, root := guardWorld(t)
		w.Link(root, "book/l", "language/go")
		_, ref := judge(t, w.Ports(), root, edit(t, "/repo/book/l/rules/errors.yaml", prose, more))
		if ref == nil || !strings.Contains(ref.Observed(), "reached through a link") || strings.Contains(ref.Observed(), "not clean") {
			t.Fatalf("a clean path through a link is not judged as before: %v", ref)
		}
	})
}

// TestTheGuardRefusesAnEditWhoseLastPartIsALink. Planted: with book/ln.yaml a link to the rule
// file, an edit of its prose by the link's path, allowed at the rule file, is
// refused, naming the file the link leads to and saying to edit that file by
// its own path. With the guard's ErrLinked case taken out, it is refused with
// the wrong words, saying neither cause nor cure; with host/fs's check taken
// out, the journey TestTheGuardRefusesAnEditThroughAFileLink is allowed.
// Control: the same edit by the rule file's own path is allowed. Risk: the
// guard judges the edit at the file the link leads to, and a host tool that
// writes a new file and renames it over replaces the link with a file at
// book/ln.yaml that the guard never judged there.
func TestTheGuardRefusesAnEditWhoseLastPartIsALink(t *testing.T) {
	t.Parallel()
	const prose, more = "An error says what failed.", "An error tells what failed."
	w, root := guardWorld(t)
	w.Link(root, "book/ln.yaml", "language/go/rules/errors.yaml")
	_, ref := judge(t, w.Ports(), root, edit(t, "/repo/book/ln.yaml", prose, more))
	if ref == nil || ref.Word() != model.Refused {
		t.Fatalf("an edit by a link's path is not refused: %v", ref)
	}
	if !strings.Contains(ref.Observed(), "is a link") || !strings.Contains(ref.Observed(), "book/language/go/rules/errors.yaml") || !strings.Contains(ref.Hint(), "the file the link leads to") {
		t.Errorf("the refusal neither names the file the link leads to nor says to edit it: %s / %s", ref.Observed(), ref.Hint())
	}
	if got := refusals(t, w, root); len(got) != 1 {
		t.Fatalf("the journal holds %d refusals", len(got))
	}
	g, ref := judge(t, w.Ports(), root, edit(t, guardRuleAt, prose, more))
	if ref != nil || g.Path != "book/language/go/rules/errors.yaml" {
		t.Fatalf("control: the edit by the rule file's own path is not allowed: %v at %q", ref, g.Path)
	}
}

// TestTheGuardRefusesAnEditToAFileWithASecondName. Planted: with book/twin.yaml a second name of
// the rule file, by a hard link, an edit of its prose by either name, allowed
// once the file has one name, is refused, naming the path and saying to give
// the file one name. With the guard's read by ReadSoleWithin put back to Read, both
// edits are allowed. Control: the same edit, once the second name is removed,
// is allowed. Risk: the guard judges the edit by the
// one name it is given, and the file changes under the other too, unjudged.
func TestTheGuardRefusesAnEditToAFileWithASecondName(t *testing.T) {
	t.Parallel()
	const prose, more = "An error says what failed.", "An error tells what failed."
	w, root := guardWorld(t)
	w.Name(root, "book/language/go/rules/errors.yaml", "book/twin.yaml")
	for _, at := range []string{guardRuleAt, "/repo/book/twin.yaml"} {
		_, ref := judge(t, w.Ports(), root, edit(t, at, prose, more))
		if ref == nil || ref.Word() != model.Refused {
			t.Fatalf("an edit by %s, a name of a file with two, is not refused: %v", at, ref)
		}
		if !strings.Contains(ref.Observed(), "has more than one name") || !strings.Contains(ref.Hint(), "give the file one name") {
			t.Errorf("the refusal of %s neither says the file has more than one name nor how to give it one: %s / %s", at, ref.Observed(), ref.Hint())
		}
	}
	if got := refusals(t, w, root); len(got) != 2 {
		t.Fatalf("the journal holds %d refusals, want 2", len(got))
	}
	if err := w.Ports().Writer.Remove(root, "book/twin.yaml"); err != nil {
		t.Fatal(err)
	}
	g, ref := judge(t, w.Ports(), root, edit(t, guardRuleAt, prose, more))
	if ref != nil || g.Path != "book/language/go/rules/errors.yaml" {
		t.Fatalf("control: the edit, once the file has one name, is not allowed: %v at %q", ref, g.Path)
	}
}

func TestAnInputThatCouldNotBeReadIsRefused(t *testing.T) {
	t.Parallel()
	w, root := guardWorld(t)
	_, ref, err := coreapi.GuardEdit(t.Context(), w.Ports(), coreapi.GuardAsk{Root: root, Unread: errors.New("the pipe broke"), Run: "guard-run"})
	if err != nil || ref == nil || !strings.Contains(ref.Observed(), "the pipe broke") {
		t.Fatalf("ref %v, err %v", ref, err)
	}
}

// unjournalled is a journal that writes nothing.
type unjournalled struct{ port.Journal }

func (unjournalled) Append(context.Context, model.Root, model.Record) error {
	return errors.New("the disk is full")
}

// TestARefusalTheJournalCannotTakeIsStillRefused.
// Risk: a guard that cannot journal its refusal lets the edit through.
func TestARefusalTheJournalCannotTakeIsStillRefused(t *testing.T) {
	t.Parallel()
	w, root := guardWorld(t)
	p := w.Ports()
	p.Journal = unjournalled{p.Journal}
	_, ref := judge(t, p, root, edit(t, guardRuleAt, "status: proposed", "status: active"))
	if ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "could not be journalled: the disk is full") {
		t.Fatalf("ref %v", ref)
	}
}

// TestARunOfItsOwnIsNamedByTheInstantAndTheInput: with no run of the root's,
// the refusal is a run of one close whose id the journal takes.
func TestARunOfItsOwnIsNamedByTheInstantAndTheInput(t *testing.T) {
	t.Parallel()
	w, root := guardWorld(t)
	_, ref, err := coreapi.GuardEdit(t.Context(), w.Ports(), coreapi.GuardAsk{Root: root, Input: []byte("{")})
	if err != nil || ref == nil {
		t.Fatalf("ref %v, err %v", ref, err)
	}
	recs, _, _ := w.Ports().Journal.Records(t.Context(), root)
	if len(recs) != 1 || !strings.HasPrefix(recs[0].Run, "guard-") {
		t.Fatalf("records %+v", recs)
	}
}

// TestOutsideARulebookTheGuardIsNotReady: the guard is not for a project (S1).
func TestOutsideARulebookTheGuardIsNotReady(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	root := w.Seed("/proj", map[string]string{"README.md": "a project\n"})
	_, ref, err := coreapi.GuardEdit(t.Context(), w.Ports(), coreapi.GuardAsk{Root: root, Input: edit(t, "/proj/README.md", "a", "b"), Run: "r"})
	if !errors.Is(err, coreapi.ErrNoBook) || ref != nil {
		t.Fatalf("ref %v, err %v", ref, err)
	}
}

// An input of exactly MaxHookInput bytes is read; one byte more is refused
// whole, never cut and read in part.
// boundary: MaxHookInput
func TestTheHooksInputIsReadUpToItsCapAndNoFurther(t *testing.T) {
	t.Parallel()
	doc := edit(t, guardRuleAt, "a", "b")
	at := append(doc, bytes.Repeat([]byte(" "), coreapi.MaxHookInput-len(doc))...)
	if _, err := coreapi.ParseHookEdit(at); err != nil {
		t.Fatalf("an input of the cap: %v", err)
	}
	if _, err := coreapi.ParseHookEdit(append(at, ' ')); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("an input a byte over the cap: %v", err)
	}
}

func TestTheDocumentSaysDoneOrRefused(t *testing.T) {
	t.Parallel()
	g := model.Guard{Path: "book/x.yaml"}
	if d := coreapi.GuardDoc(g, "", ""); d.Word != "done" || d.Changes == nil || d.Schema != coreapi.GuardSchema {
		t.Fatalf("done: %+v", d)
	}
	if d := coreapi.GuardDoc(g, "why", "next"); d.Word != "refused" || d.Why != "why" || d.Next != "next" {
		t.Fatalf("refused: %+v", d)
	}
}

// FuzzParseHookEdit (tests.generated.1): the hook's input is data from
// outside. Whatever it holds, the parser does not panic, and what it answers
// is one of the three tools with a file, a Write with its content and an Edit
// or MultiEdit with its replacements; making the edit on any file never
// panics, and a replacement the file does not hold is an error, not a pass.
func FuzzParseHookEdit(f *testing.F) {
	for _, s := range []string{
		`{"tool_name":"Write","tool_input":{"file_path":"/r/a.yaml","content":"x"}}`,
		`{"tool_name":"Edit","tool_input":{"file_path":"/r/a.yaml","old_string":"a","new_string":"b"}}`,
		`{"tool_name":"Edit","tool_input":{"file_path":"/r/a.yaml","old_string":"a","new_string":"b","replace_all":true}}`,
		`{"tool_name":"MultiEdit","tool_input":{"file_path":"/r/a.yaml","edits":[{"old_string":"a","new_string":"b"},{"old_string":"","new_string":"c"}]}}`,
		`{"tool_name":"Edit","tool_input":{"file_path":"/r/a.yaml","old_string":null,"new_string":"b"}}`,
		`{"tool_name":"Write","tool_input":{"file_path":"/r/a.yaml"}}`,
		`{"tool_name":"Bash","tool_input":{"command":"true"}}`,
		`{"tool_name":"Write","tool_input":"x"}`,
		`{"tool_name":"Write","tool_input":{"file_path":"/r/a","content":"x"}} {}`,
		`{"tool_name":"Write","tool_name":"Edit","tool_input":{"file_path":"/r/a","content":"x"}}`,
		`[]`, `null`, ``, `{`, "\x00", `{"tool_input":{}}`,
	} {
		f.Add([]byte(s), "a\na\nb\n")
	}
	f.Fuzz(func(t *testing.T, input []byte, file string) {
		e, err := coreapi.ParseHookEdit(input)
		if err != nil {
			return
		}
		if e.Path == "" || !slices.Contains([]string{"Write", "Edit", "MultiEdit"}, e.Tool) {
			t.Fatalf("parsed %+v", e)
		}
		if (e.Tool == "Write") != (e.Content != nil) || (e.Tool != "Write" && len(e.Edits) == 0) {
			t.Fatalf("parsed %+v", e)
		}
		got, err := e.Applied([]byte(file), true)
		if err != nil || e.Content != nil {
			return
		}
		for _, r := range e.Edits {
			if r.Old == "" {
				t.Fatalf("an empty old_string was made: %+v into %q", e, got)
			}
		}
		if !strings.Contains(file, e.Edits[0].Old) {
			t.Fatalf("an old_string the file does not hold was made: %q in %q", e.Edits[0].Old, file)
		}
	})
}

// folding is a tree that finds a name in any case, of a file or of a folder,
// as the one of that name in lower case, as a host that folds case does: it
// reads and stats it so, and lists a folder below the name as it was given,
// as a walk from that folder does.
type folding struct{ port.Tree }

func (f folding) Read(root model.Root, rel string) ([]byte, error) {
	return f.Tree.Read(root, strings.ToLower(rel))
}

func (f folding) Stat(root model.Root, rel string) (model.Entry, error) {
	e, err := f.Tree.Stat(root, strings.ToLower(rel))
	e.Path = rel
	return e, err
}

func (f folding) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	entries, err := f.Tree.List(root, strings.ToLower(dir), scope)
	if dir == "" || dir == "." {
		return entries, err
	}
	for i := range entries {
		entries[i].Path = dir + entries[i].Path[len(dir):]
	}
	return entries, err
}

// TestAFileNamedInAnotherCaseIsRefused: on a host that folds case, an edit of
// Errors.yaml, or of errors.yaml below Language/ or Go/, writes the file the
// guard reads as language/go/rules/errors.yaml, so the guard refuses to judge
// it by a name, of the file or of any folder above it, the file does not
// have; the control, the file's own name, is judged, and a new folder is
// placed.
// Risk: the guard judges a file it read under one name while the edit lands
// on another, and a status edit passes as a new file or as no edit at all.
func TestAFileNamedInAnotherCaseIsRefused(t *testing.T) {
	t.Parallel()
	status := func(at string) []byte { return edit(t, at, "status: proposed", "status: active") }
	for name, tc := range otherCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := guardWorld(t)
			p := w.Ports()
			p.Tree = folding{p.Tree}
			_, ref := judge(t, p, root, tc.input)
			if ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), tc.why) {
				t.Fatalf("not refused as %q: %v", tc.why, ref)
			}
			if got := refusals(t, w, root); len(got) != 1 {
				t.Fatalf("the journal holds %d refusals", len(got))
			}
		})
	}
	w, root := guardWorld(t)
	p := w.Ports()
	p.Tree = folding{p.Tree}
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, "An error says", "An error tells")); ref != nil {
		t.Fatalf("the file's own name: %v", ref)
	}
	if _, ref := judge(t, p, root, status(guardRuleAt)); ref == nil || !strings.Contains(ref.Observed(), "status") {
		t.Fatalf("a status edit by the file's own name: %v", ref)
	}
	newAt := strings.Replace(guardRuleAt, "/language/go/rules/errors.yaml", "/notes/new/readme.md", 1)
	if _, ref := judge(t, p, root, hook(t, "Write", map[string]any{"file_path": newAt, "content": "x\n"})); ref != nil {
		t.Fatalf("a new file in a new folder: %v", ref)
	}
}

// diskSpelling is a host that folds case and, as Windows does, answers Within
// in the disk's own spelling, an 8.3 short name expanded: every name of the
// fixture is lower case, and BINDIN~1 is bindings.
type diskSpelling struct{ folding }

func (d diskSpelling) Within(root model.Root, abs string) (string, error) {
	rel, err := d.folding.Within(root, abs)
	return strings.ReplaceAll(strings.ToLower(rel), "bindin~1", "bindings"), err
}

// TestANameInAnotherCaseIsRefusedWhereTheHostAnswersTheDisksSpelling: on a
// host whose Within answers the disk's spelling, as Windows does, each edit
// of TestAFileNamedInAnotherCaseIsRefused ends refused for the same reason,
// read from the path as written, and so does a short name the host expands;
// the control, the file's own name, is judged.
// Planted failure: the written parts not
// held to the names.
// Risk: the reason of a refusal changes by host, or a host that folds case
// is judged by a spelling the edit never gave.
func TestANameInAnotherCaseIsRefusedWhereTheHostAnswersTheDisksSpelling(t *testing.T) {
	t.Parallel()
	cases := otherCases(t)
	cases["a short name"] = struct {
		input []byte
		why   string
	}{hook(t, "Write", map[string]any{"file_path": guardRoot + "/book/BINDIN~1/x.yaml", "content": "x: 1\n"}), "8.3 short name"}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := guardWorld(t)
			p := w.Ports()
			p.Tree = diskSpelling{folding{p.Tree}}
			_, ref := judge(t, p, root, tc.input)
			if ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), tc.why) {
				t.Fatalf("not refused as %q: %v", tc.why, ref)
			}
		})
	}
	w, root := guardWorld(t)
	p := w.Ports()
	p.Tree = diskSpelling{folding{p.Tree}}
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, "An error says", "An error tells")); ref != nil {
		t.Fatalf("the file's own name: %v", ref)
	}
}

// otherCases are edits of the rules file, or of a new file beside it, by a
// file or a folder named in another case, each with the reason it is refused.
func otherCases(t *testing.T) map[string]struct {
	input []byte
	why   string
} {
	t.Helper()
	status := func(at string) []byte { return edit(t, at, "status: proposed", "status: active") }
	return map[string]struct {
		input []byte
		why   string
	}{
		"the file":                  {hook(t, "Write", map[string]any{"file_path": strings.Replace(guardRuleAt, "errors.yaml", "Errors.yaml", 1), "content": "x: 1\n"}), "names a file whose own name is spelled otherwise"},
		"the top folder":            {status(strings.Replace(guardRuleAt, "/language/", "/Language/", 1)), "names a folder whose own name is spelled otherwise"},
		"a folder below":            {status(strings.Replace(guardRuleAt, "/go/", "/Go/", 1)), "names a folder whose own name is spelled otherwise"},
		"a new file in such folder": {hook(t, "Write", map[string]any{"file_path": strings.Replace(guardRuleAt, "/go/rules/errors.yaml", "/Go/rules/new.yaml", 1), "content": "x: 1\n"}), "names a folder whose own name is spelled otherwise"},
	}
}

// TestAnEditThatLeavesANewFindingIsRefused: an edit that changes no
// lifecycle field but leaves the rulebook with a finding it did not have is
// refused, naming the finding.
// Risk: an edit that breaks a rule's file reads as one with no rules, so no
// lifecycle field, and passes.
func TestAnEditThatLeavesANewFindingIsRefused(t *testing.T) {
	t.Parallel()
	w, root := guardWorld(t)
	_, ref := judge(t, w.Ports(), root, edit(t, guardRuleAt, "rules:", "rules: ["))
	if ref == nil || !strings.Contains(ref.Observed(), "a finding the rulebook did not have") {
		t.Fatalf("ref %v", ref)
	}
}

// TestANewNameThatFoldsToOneTheLoaderReadsIsRefusedOnEveryHost: a new file
// whose name folds to one the loader reads, there or not, or to one the
// repository has, and is spelled otherwise is refused on a host that keeps
// case as on one that folds it; so is a new name some host reads as another.
// The controls, the loader's own spellings and a plain new name, are judged.
// The fake's Within refuses a colon and a
// backslash before the guard reads them; the journey holds the disk's.
// Risk: History.jsonl or Bindings/x.yaml is a new file to the guard and the
// file the loader reads once a host that folds case writes it.
func TestANewNameThatFoldsToOneTheLoaderReadsIsRefusedOnEveryHost(t *testing.T) {
	t.Parallel()
	write := func(rel, content string) []byte {
		return hook(t, "Write", map[string]any{"file_path": guardRoot + "/" + rel, "content": content})
	}
	const entry = `{"at":"2026-11-15","entity":"rule:go.errors.1","field":"status","from":"proposed","to":"active"}` + "\n"
	cases := map[string]struct {
		input []byte
		why   string
	}{
		"history in another case":         {write("book/History.jsonl", entry), "spelled otherwise"},
		"exclusions, not there":           {write("book/Exclusions.yaml", "format: 3\n"), "spelled otherwise"},
		"a binding in a folder so":        {write("book/Bindings/x.yaml", "subject: go.errors.1\nvalue: 2\n"), "spelled otherwise"},
		"the book's folder so":            {write("Book/notes.md", "x\n"), "spelled otherwise"},
		"an end in a dot":                 {write("book/history.jsonl.", entry), "ends in a dot or a space"},
		"an end in a space":               {write("book/bindings/x.yaml ", "x\n"), "ends in a dot or a space"},
		"an alternate stream":             {write("book/history.jsonl:x", entry), ""},
		"a short name":                    {write("book/BINDIN~1/x.yaml", "x\n"), "8.3 short name"},
		"a backslash":                     {write(`book/bindings\x.yaml`, "x\n"), ""},
		"a name that is not ASCII":        {write("book/historÿ.jsonl", entry), "not printable ASCII"},
		"a folded letter that is not one": {write("book/Keep.md", "x\n"), "not printable ASCII"},
		"a device name Windows keeps":     {write("book/CON.md", "x\n"), "Windows keeps it for a device"},
		"a short name with an extension":  {write("book/a~1.txt", "x\n"), "8.3 short name"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := guardWorld(t)
			_, ref := judge(t, w.Ports(), root, tc.input)
			if ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), tc.why) {
				t.Fatalf("not refused as %q: %v", tc.why, ref)
			}
			if got := refusals(t, w, root); len(got) != 1 {
				t.Fatalf("the journal holds %d refusals", len(got))
			}
		})
	}
	w, root := guardWorld(t)
	if _, ref := judge(t, w.Ports(), root, write("book/notes/keep.md", "x\n")); ref != nil {
		t.Fatalf("a plain new name: %v", ref)
	}
	// A ~ and a digit that is not an 8.3 short name is the port's to pass, not the guard's to refuse: the
	// guard's old rule held it anywhere in a name, which was broader than NTFS and a table of its own.
	for _, name := range []string{"a~1b.md", "x~1b.txt", "a.~1", "a~1.b.c"} {
		if _, ref := judge(t, w.Ports(), root, write("book/"+name, "x\n")); ref != nil {
			t.Fatalf("a new name %q that is no short name: %v", name, ref)
		}
	}
	if _, ref := judge(t, w.Ports(), root, write("book/history.jsonl", entry)); ref == nil || strings.Contains(ref.Observed(), "spelled") {
		t.Fatalf("the loader's own history.jsonl is not judged as the history: %v", ref)
	}
	if _, ref := judge(t, w.Ports(), root, write("book/bindings/x.yaml", "subject: go.errors.1\nvalue: 2\n")); ref == nil || strings.Contains(ref.Observed(), "spelled") {
		t.Fatalf("a binding in bindings/ is not judged as one: %v", ref)
	}
}

// TestATwinInAnotherCaseBelowAFolderThePortRefusesIsRefused: the
// repository holds book/<folder>/keep.md and book/<folder>/sub/x.md, <folder> a name the port refuses (aux, old~1,
// v1., a name with U+200D, a dotted capital I), and a write to a twin of either in another case is refused as it
// is on release-1: spelled otherwise, a file or a folder. The names the port passes below it are still folded, so
// the key folds name by name when one name is refused and a refused name stands for itself.
// Risk: below a folder no system can hold, a twin in another case meets no form and passes, so the guard is looser
// than it was, and by a reason that differs by host.
func TestATwinInAnotherCaseBelowAFolderThePortRefusesIsRefused(t *testing.T) {
	t.Parallel()
	for _, folder := range []string{"aux", "old~1", "v1.", "a\u200db", "\u0130"} {
		for name, tc := range map[string]struct{ at, why string }{
			"a file":   {"book/" + folder + "/Keep.md", "names a file whose own name is spelled otherwise"},
			"a folder": {"book/" + folder + "/Sub/new.md", "names a folder whose own name is spelled otherwise"},
		} {
			t.Run(strings.NewReplacer("/", "_", "~", "_").Replace(folder)+" "+name, func(t *testing.T) {
				t.Parallel()
				w := porttest.NewWorld(t)
				w.SeedSchemas(allSchemas()...)
				w.Instant(time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC))
				root := w.Seed(guardRoot, map[string]string{
					"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure,
					"book/language/go/rules/errors.yaml": guardRules,
					"book/bindings/go-errors-1.yaml":     "subject: go.errors.1\nvalue: 1\n",
					"book/" + folder + "/keep.md":        "x\n",
					"book/" + folder + "/sub/x.md":       "x\n",
				})
				in := hook(t, "Write", map[string]any{"file_path": guardRoot + "/" + tc.at, "content": "x\n"})
				_, ref := judge(t, w.Ports(), root, in)
				if ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), tc.why) {
					t.Fatalf("a twin %q below %q not refused as %q: %v", tc.at, folder, tc.why, ref)
				}
			})
		}
	}
}

// TestTwoNamesThatFoldToOneAreNeitherJudged: a repository that holds two
// names a host that folds case reads as one has neither judged, since the
// guard cannot say which the host writes.
// Risk: the guard reads a.md and the host writes A.md, or the reverse.
func TestTwoNamesThatFoldToOneAreNeitherJudged(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{"book/notes/a.md", "book/notes/A.md"} {
		w := porttest.NewWorld(t)
		w.SeedSchemas(allSchemas()...)
		root := w.Seed(guardRoot, map[string]string{
			"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure,
			"book/language/go/rules/errors.yaml": guardRules,
			"book/notes/a.md":                    "a\n", "book/notes/A.md": "A\n",
		})
		_, ref := judge(t, w.Ports(), root, hook(t, "Write", map[string]any{"file_path": guardRoot + "/" + rel, "content": "b\n"}))
		if ref == nil || !strings.Contains(ref.Observed(), "another name of the repository folds to") {
			t.Fatalf("%s: %v", rel, ref)
		}
	}
}

// TestAnEmptyNewStringBeforeALinesEndIsRefused: an Edit that takes out an
// old_string the file holds before a line's end, with nothing in its place,
// is refused, since the agent's host takes the end out too and joins the
// line below into the line above; the controls, the end named in old_string
// and a new_string that is not empty, are judged.
// Risk: a comment's text is taken out, the host joins the aliases line into
// the comment, and the guard judges an edit that kept it.
func TestAnEmptyNewStringBeforeALinesEndIsRefused(t *testing.T) {
	t.Parallel()
	commented := strings.Replace(guardRules, "    status: proposed\n", "    status: proposed #ZZ\n", 1)
	w, root := guardWorldOf(t, commented)
	p := w.Ports()
	for name, input := range map[string][]byte{
		"an Edit":          edit(t, guardRuleAt, " #ZZ", ""),
		"a MultiEdit's":    []byte(fmt.Sprintf(`{"tool_name":"MultiEdit","tool_input":{"file_path":%q,"edits":[{"old_string":"An error says","new_string":"An error tells"},{"old_string":"ZZ","new_string":""}]}}`, guardRuleAt)),
		"with replace_all": hook(t, "Edit", map[string]any{"file_path": guardRuleAt, "old_string": "#ZZ", "new_string": "", "replace_all": true}),
	} {
		if _, ref := judge(t, p, root, input); ref == nil || !strings.Contains(ref.Observed(), "name the line's end in old_string") {
			t.Fatalf("%s: %v", name, ref)
		}
	}
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, " #ZZ\n", "\n")); ref != nil {
		t.Fatalf("the end named in old_string: %v", ref)
	}
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, "#ZZ", "#")); ref != nil {
		t.Fatalf("a new_string that is not empty: %v", ref)
	}
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, "An error says ", "")); ref != nil {
		t.Fatalf("an old_string not before a line's end: %v", ref)
	}
}

// TestAnEditOfAFileThatDoesNotReadIsJudgedOnlyOnceItReads: an edit inside a
// rules file the loader cannot read, which holds no rules before or after
// it, is refused while the file stays unread; the edit that makes it read
// is judged, and refused for the rule it brings back.
// Risk: an edit inside a broken file passes as no change, whatever it holds.
func TestAnEditOfAFileThatDoesNotReadIsJudgedOnlyOnceItReads(t *testing.T) {
	t.Parallel()
	broken := strings.Replace(guardRules, "rules:\n", "rules: [\n", 1)
	w, root := guardWorldOf(t, broken)
	p := w.Ports()
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, "An error says", "An error tells")); ref == nil || !strings.Contains(ref.Observed(), "a finding before the edit and after it") {
		t.Fatalf("an edit that leaves the file unread: %v", ref)
	}
	if _, ref := judge(t, p, root, edit(t, guardRuleAt, "rules: [\n", "rules:\n")); ref == nil || strings.Contains(ref.Observed(), "a finding before") {
		t.Fatalf("the edit that makes the file read is not judged on its rules: %v", ref)
	}
	if _, ref := judge(t, p, root, hook(t, "Write", map[string]any{"file_path": guardRoot + "/book/notes.md", "content": "x\n"})); ref != nil {
		t.Fatalf("an edit of another file: %v", ref)
	}
}

// TestTheGuardReadsAFileOfAtMostMaxHookInputBytes: the file the guard judges
// is read at coreapi.MaxHookInput and no more.
// Planted: a rule file of MaxHookInput+1 bytes is refused, the read named, and
// one of exactly MaxHookInput is read, so the edit is judged as for any file.
// The refusal names the bound, MaxHookInput (16 MiB), and what to do.
// With the bound in guard_tree.go loosened to 1<<40, the first is read whole
// and allowed. Risk: a guard that reads what it was not bounded to reads a file
// of any size whole into the hook.
func TestTheGuardReadsAFileOfAtMostMaxHookInputBytes(t *testing.T) {
	t.Parallel()
	const prose, more = "An error says what failed.", "An error tells what failed."
	padded := func(size int) string {
		return guardRules + "\n#" + strings.Repeat("x", size-len(guardRules)-2)
	}
	t.Run("a file of exactly the bound is read", func(t *testing.T) {
		t.Parallel()
		w, root := guardWorldOf(t, padded(coreapi.MaxHookInput))
		g, ref := judge(t, w.Ports(), root, edit(t, guardRuleAt, prose, more))
		if ref != nil && strings.Contains(ref.Observed(), "could not be read") {
			t.Fatalf("a file of MaxHookInput bytes was not read: %s", ref.Observed())
		}
		if ref == nil && g.Path != "book/language/go/rules/errors.yaml" {
			t.Errorf("the edit was allowed at %q", g.Path)
		}
	})
	t.Run("a file of one byte more is refused, the read named", func(t *testing.T) {
		t.Parallel()
		w, root := guardWorldOf(t, padded(coreapi.MaxHookInput+1))
		_, ref := judge(t, w.Ports(), root, edit(t, guardRuleAt, prose, more))
		if ref == nil || ref.Word() != model.Refused || !strings.Contains(ref.Observed(), "over the 16777216 bytes (16 MiB) the guard reads") || !strings.Contains(ref.Hint(), "at most 16 MiB") {
			t.Fatalf("a file of MaxHookInput+1 bytes: %v, want the bound named (MaxHookInput, 16 MiB) and what to do", ref)
		}
	})
}
