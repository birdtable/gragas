package wire

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func codec(t *testing.T) *Codec {
	t.Helper()
	return New(porttest.NewWorld(t).Ports().FileNames)
}

func grantsOf() []string { return []string{"p"} }

func ok(body string) []byte { return []byte(`{"ok":` + body + `}`) }

// run is a plan answer of one run step with member extra added to it.
func run(extra string) []byte {
	return ok(`{"final":true,"steps":[{"kind":"run","program":"p","argv":["a"]` + extra + `}]}`)
}

// nested is a value that many arrays deep.
func nested(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) }

// entries is an answer of entries whose keys are the ones given, each on.
func entries(keys ...string) []byte {
	var es []string
	for _, k := range keys {
		es = append(es, `{"key":"`+k+`","on":true}`)
	}
	return ok(`{"entries":[` + strings.Join(es, ",") + `]}`)
}

// TestWhatTheSchemasCannotSay plants one answer for each rule of the contract
// that this package holds, and the place it is refused at: red first,
// each by removing its check. Every row also has a twin that passes, in the
// tests below, so a rule is never held by refusing everything.
//
//   - a plan that asks for nothing and is not final
//   - an argv word with a { that is not {scratch} or {null}
//   - a place path that is not a path; a collect path likewise
//   - a glob with a character beyond *, ? and **; ** with others in its part
//   - stdin over 1 MiB; a placed file over 1 MiB (TestAPlacedFileIsHeldToItsCap)
//   - entries out of order or twice; a value 33 levels deep; one over 64 KiB
//   - findings out of order; a finding with a path that is not one
//   - words not as many as pairs
//   - preset files out of order; a file without the mark in its first line
//   - an error code the operation does not answer; a plan's unreadable in round 0
//   - a response over 16 MiB (ErrLimit)
//   - a member the response does not name (ErrAnswer)
func TestWhatTheSchemasCannotSay(t *testing.T) {
	t.Parallel()
	c := codec(t)
	rows := []struct {
		name string
		do   func() error
		at   string
	}{
		{"a plan that asks for nothing and is not final", func() error {
			_, err := c.DecodePlan(ok(`{"steps":[],"final":false}`), 0, grantsOf())
			return err
		}, "/ok"},
		{"an argv word with another {", func() error {
			_, err := c.DecodePlan(run(``), 0, grantsOf())
			_ = err
			_, err = c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"run","program":"p","argv":["a","{home}/x"]}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/argv/1"},
		{"a place path that climbs", func() error {
			_, err := c.DecodePlan(run(`,"place":[{"path":"../x","data":{"text":""}}]`), 0, grantsOf())
			return err
		}, "/ok/steps/0/place/0/path"},
		{"a collect path with an empty part", func() error {
			_, err := c.DecodePlan(run(`,"collect":["a//b"]`), 0, grantsOf())
			return err
		}, "/ok/steps/0/collect/0"},
		{"a glob with a class", func() error {
			_, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"read","glob":"a[b]"}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/glob"},
		{"a glob with ** among others", func() error {
			_, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"read","glob":"a**/b"}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/glob"},
		{"stdin over 1 MiB", func() error {
			_, err := c.DecodePlan(run(`,"stdin":{"text":"`+strings.Repeat("a", MaxStdin+1)+`"}`), 0, grantsOf())
			return err
		}, ""},
		{"a program that is no grant", func() error {
			_, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"run","program":"other","argv":["a"]}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/program"},
		{"a placed path inside .git", func() error {
			_, err := c.DecodePlan(run(`,"place":[{"path":"a/.git/config","data":{"text":""}}]`), 0, grantsOf())
			return err
		}, "/ok/steps/0/place/0/path"},
		{"a placed path that is absolute", func() error {
			_, err := c.DecodePlan(run(`,"place":[{"path":"/a/x","data":{"text":""}}]`), 0, grantsOf())
			return err
		}, "/ok/steps/0/place/0/path"},
		{"a collect path that escapes", func() error {
			_, err := c.DecodePlan(run(`,"collect":["a/../../x"]`), 0, grantsOf())
			return err
		}, "/ok/steps/0/collect/0"},
		{"a glob that climbs out", func() error {
			_, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"read","glob":"../**"}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/glob"},
		{"a glob that is absolute", func() error {
			_, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"read","glob":"/a/*"}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/glob"},
		{"a glob into .git", func() error {
			_, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"read","glob":".git/**"}]}`), 0, grantsOf())
			return err
		}, "/ok/steps/0/glob"},
		{"a preset path that climbs", func() error {
			_, err := c.DecodePreset(ok(`{"files":[{"path":"../a","text":"# m\\n"}]}`), "m")
			return err
		}, "/ok/files/0/path"},
		{"a preset path inside .git", func() error {
			_, err := c.DecodePreset(ok(`{"files":[{"path":".git/hooks/x","text":"# m\\n"}]}`), "m")
			return err
		}, "/ok/files/0/path"},
		{"entries out of order", func() error { _, err := c.DecodeEntries(entries("b", "a")); return err }, "/ok/entries/1/key"},
		{"entries that repeat a key", func() error { _, err := c.DecodeEntries(entries("a", "a")); return err }, "/ok/entries/1/key"},
		{"a value 33 levels deep", func() error {
			_, err := c.DecodeEntries(ok(`{"entries":[{"key":"a","on":true,"value":` + nested(33) + `}]}`))
			return err
		}, "/ok/entries/0/value"},
		{"a value over 64 KiB", func() error {
			_, err := c.DecodeEntries(ok(`{"entries":[{"key":"a","on":true,"options":"` + strings.Repeat("a", MaxValue) + `"}]}`))
			return err
		}, ""},
		{"findings out of order", func() error {
			_, err := c.DecodeFindings(ok(`{"ran":true,"findings":[{"key":"k","message":"m","path":"b"},{"key":"k","message":"m","path":"a"}]}`))
			return err
		}, "/ok/findings/1"},
		{"a finding with a path that climbs", func() error {
			_, err := c.DecodeFindings(ok(`{"ran":true,"findings":[{"key":"k","message":"m","path":"../a"}]}`))
			return err
		}, "/ok/findings/0/path"},
		{"fewer words than pairs", func() error { _, err := c.DecodeRank(ok(`{"words":["same"]}`), 2); return err }, "/ok/words"},
		{"preset files out of order", func() error {
			_, err := c.DecodePreset(ok(`{"files":[{"path":"b","text":"# m\n"},{"path":"a","text":"# m\n"}]}`), "m")
			return err
		}, "/ok/files/1/path"},
		{"a preset file without the mark in its first line", func() error {
			_, err := c.DecodePreset(ok(`{"files":[{"path":"a","text":"x\n# m\n"}]}`), "m")
			return err
		}, "/ok/files/0/text"},
		{"an error code the operation does not answer", func() error {
			_, err := c.DecodeRank([]byte(`{"error":{"code":"refused","message":"m"}}`), 0)
			return err
		}, "/error/code"},
		{"a plan's unreadable in round 0", func() error {
			_, err := c.DecodePlan([]byte(`{"error":{"code":"unreadable","message":"m"}}`), 0, grantsOf())
			return err
		}, "/error/code"},
		{"an error path that is no JSON Pointer", func() error {
			_, err := c.DecodePlan([]byte(`{"error":{"code":"bug","message":"m","path":"a/b"}}`), 0, grantsOf())
			return err
		}, "/error/path"},
		{"a member the response does not name", func() error {
			_, err := c.DecodeRank(ok(`{"words":[],"later":1}`), 0)
			return err
		}, "/ok/later"},
		{"a response that is not JSON", func() error { _, err := c.DecodeRank([]byte(`{"ok":`), 0); return err }, ""},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			err := r.do()
			var a *plugin.AnswerError
			if !errors.As(err, &a) {
				t.Fatalf("refused with %T %v, want an *plugin.AnswerError", err, err)
			}
			// A cap the generated types hold is refused earlier, at its own place,
			// so a row that names no place holds the kind alone.
			if r.at != "" && a.Path != r.at {
				t.Errorf("refused at %q (%s), want %q", a.Path, a.Why, r.at)
			}
		})
	}
}

// TestTheAnswersThatPass is the twin of each row: the same answer as it
// should be, read whole.
func TestTheAnswersThatPass(t *testing.T) {
	t.Parallel()
	c := codec(t)
	if _, err := c.DecodePlan(run(`,"place":[{"path":"a/b.js","data":{"text":""}}],"collect":["out/x"]`), 0, grantsOf()); err != nil {
		t.Errorf("plan: %v", err)
	}
	if _, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"read","glob":"src/**/*.t?"}]}`), 0, grantsOf()); err != nil {
		t.Errorf("glob: %v", err)
	}
	if _, err := c.DecodePlan(ok(`{"final":true,"steps":[{"kind":"run","program":"p","argv":["{scratch}","{scratch}/c.js","{null}"]}]}`), 0, grantsOf()); err != nil {
		t.Errorf("argv: %v", err)
	}
	if _, err := c.DecodeEntries(entries("a", "b", "é")); err != nil {
		t.Errorf("entries: %v", err)
	}
	if _, err := c.DecodeEntries(ok(`{"entries":[{"key":"a","on":true,"value":` + nested(MaxValueDepth-1) + `}]}`)); err != nil {
		t.Errorf("a value of 32 levels: %v", err)
	}
	if _, err := c.DecodeFindings(ok(`{"ran":true,"findings":[{"key":"k","message":"m"},{"key":"k","message":"m","path":"a","line":1},{"key":"k","message":"m","path":"a","line":2}]}`)); err != nil {
		t.Errorf("findings: %v", err)
	}
	if _, err := c.DecodeRank(ok(`{"words":["same","weaker"]}`), 2); err != nil {
		t.Errorf("rank: %v", err)
	}
	if _, err := c.DecodePreset(ok(`{"files":[{"path":"a","text":"# m\nx\n"},{"path":"b","text":"// m\n"}]}`), "m"); err != nil {
		t.Errorf("preset: %v", err)
	}
	if _, err := c.DecodePlan([]byte(`{"error":{"code":"unreadable","message":"m"}}`), 1, grantsOf()); !isRemote(err) {
		t.Errorf("a plan's unreadable after round 0: %v, want a *plugin.CallError", err)
	}
}

func isRemote(err error) bool {
	var r *plugin.CallError
	return errors.As(err, &r)
}

// TestACallErrorIsTyped plants a plugin's own refusal and holds that it is
// not an answer fault: its code and pointer reach the caller whole.
func TestARemoteErrorIsTyped(t *testing.T) {
	t.Parallel()
	_, err := codec(t).DecodeEntries([]byte(`{"error":{"code":"unsupported","message":"no","path":"/a"}}`))
	var r *plugin.CallError
	if !errors.As(err, &r) || r.Code != plugin.CodeUnsupported || r.Path != "/a" {
		t.Fatalf("got %T %v, want a *plugin.CallError of unsupported at /a", err, err)
	}
	if _, isAnswer := errors.AsType[*plugin.AnswerError](err); isAnswer {
		t.Error("a plugin's error is no fault of its answer")
	}
}

// TestACapIsALimitNotAnAnswerFault plants a response over 16 MiB.
func TestACapIsALimitNotAnAnswerFault(t *testing.T) {
	t.Parallel()
	big := append([]byte(`{"ok":{"words":[]},"x":"`), []byte(strings.Repeat("a", MaxResponse))...)
	_, err := codec(t).DecodeRank(big, 0)
	var l *plugin.LimitError
	if !errors.As(err, &l) || l.What != plugin.LimitResponse {
		t.Fatalf("got %T %v, want a *plugin.LimitError of the response cap", err, err)
	}
}

func request(body canon.Object) plugin.Request {
	return NewRequest(plugin.OpEntries, body)
}

// planRequest is a plan's request: the caps of a request that are not the
// entries call's own are held on it, as an entries request has a smaller one.
func planRequest(body canon.Object) plugin.Request {
	return NewRequest(plugin.OpPlan, body)
}

// TestTheRequestIsCanonical plants members in one order and holds the bytes:
// sorted, no spare space, the contract's number first by name's order, and
// the features the engine speaks, cache-vars.
func TestTheRequestIsCanonical(t *testing.T) {
	t.Parallel()
	b, err := codec(t).EncodeRequest(request(canon.Object{"z": canon.Int(1), "a": canon.Int(2)}))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"body":{"a":2,"z":1},"contract":1,"features":["cache-vars"],"op":"entries"}`
	if string(b) != want {
		t.Errorf("got %s\nwant %s", b, want)
	}
}

// TestTheRequestIsHeldToItsCaps plants a request over 32 MiB, and a config
// file with a path that climbs and one with data over 1 MiB.
func TestTheRequestIsHeldToItsCaps(t *testing.T) {
	t.Parallel()
	c := codec(t)
	var l *plugin.LimitError
	if _, err := c.EncodeRequest(planRequest(canon.Object{"x": canon.String(strings.Repeat("a", MaxRequest))})); !errors.As(err, &l) || l.What != plugin.LimitRequest {
		t.Errorf("a request over 32 MiB: %T %v, want a *plugin.LimitError", err, err)
	}
	cfg := func(path, text string) canon.Object {
		return canon.Object{"config": canon.Array{canon.Object{"path": canon.String(path), "data": canon.Object{"text": canon.String(text)}}}}
	}
	var r *RequestError
	if _, err := c.EncodeRequest(planRequest(cfg("../a", ""))); !errors.As(err, &r) || r.Path != "/body/config/0/path" {
		t.Errorf("a config path that climbs: %T %v", err, err)
	}
	if _, err := c.EncodeRequest(planRequest(cfg("a", strings.Repeat("a", MaxFile+1)))); !errors.As(err, &r) {
		t.Errorf("a config file over 1 MiB: %T %v", err, err)
	}
	if _, err := c.EncodeRequest(planRequest(cfg("a", strings.Repeat("a", MaxFile)))); err != nil {
		t.Errorf("a config file of 1 MiB: %v", err)
	}
}

func haveOf(rs ...string) canon.Object {
	var a canon.Array
	for _, r := range rs {
		v, err := canon.Decode([]byte(r))
		if err != nil {
			panic(err)
		}
		a = append(a, v)
	}
	return canon.Object{"have": a}
}

// TestTheResultsOfARequestAreHeldToTheContract plants each rule of have: the
// order by round then step, the stream cap, the form of bytes, and the files'
// number and total size. The last case is the twin that passes.
func TestTheResultsOfARequestAreHeldToTheContract(t *testing.T) {
	t.Parallel()
	c := codec(t)
	res := func(round, step int, extra string) string {
		files := `,"files":[]`
		if strings.HasPrefix(extra, `,"files"`) {
			files, extra = extra, ""
		}
		return `{"round":` + strconv.Itoa(round) + `,"step":` + strconv.Itoa(step) + `,"kind":"read"` + extra + files + `}`
	}
	file := func(n int, text string) string {
		return `{"path":"f` + strconv.Itoa(n) + `","data":{"text":"` + text + `"}}`
	}
	var many, eight []string
	for i := 0; i <= MaxReadFiles; i++ {
		many = append(many, file(i, ""))
	}
	for i := range 9 {
		eight = append(eight, file(i, strings.Repeat("a", MaxFile)))
	}
	bad := []struct {
		name string
		body canon.Object
	}{
		{"a round before the one before", haveOf(res(1, 0, ""), res(0, 0, ""))},
		{"a step repeated", haveOf(res(0, 0, ""), res(0, 0, ""))},
		{"a stream over 8 MiB", haveOf(res(0, 0, `,"stdout":{"text":"`+strings.Repeat("a", MaxStream+1)+`"}`))},
		{"base64 of valid UTF-8", haveOf(res(0, 0, `,"stdout":{"base64":"YQ=="}`))},
		{"more than 256 files", haveOf(res(0, 0, `,"files":[`+strings.Join(many, ",")+`]`))},
		{"files over 8 MiB in all", haveOf(res(0, 0, `,"files":[`+strings.Join(eight, ",")+`]`))},
	}
	for _, r := range bad {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			var e *RequestError
			if _, err := c.EncodeRequest(request(r.body)); !errors.As(err, &e) {
				t.Fatalf("got %T %v, want a *RequestError", err, err)
			}
		})
	}
	ok := haveOf(res(0, 0, ""), res(0, 1, `,"stdout":{"base64":"/w=="}`), res(1, 0, ""))
	if _, err := c.EncodeRequest(request(ok)); err != nil {
		t.Errorf("results in order: %v", err)
	}
}

// TestAnEntriesRequestIsCappedAt64KiBBeforeThePluginIsReached: an entries request that encodes to exactly 65536 bytes
// is taken, one byte over is refused with a *plugin.LimitError of entries that
// names the cap and the size it was given, and a plan's request of that size is
// not held to the entries cap. Planted red: with the check taken out, the byte
// over is taken.
// boundary: MaxEntriesInput
// Risk: a plugin is asked to read a megabyte of entries and runs out its
// call's five seconds, where it should have been refused at once.
func TestAnEntriesRequestIsCappedAt64KiBBeforeThePluginIsReached(t *testing.T) {
	t.Parallel()
	c := codec(t)
	sized := func(op plugin.Op, n int) plugin.Request {
		base, err := c.EncodeRequest(NewRequest(op, canon.Object{"x": canon.String("")}))
		if err != nil {
			t.Fatal(err)
		}
		return NewRequest(op, canon.Object{"x": canon.String(strings.Repeat("a", n-len(base)))})
	}
	b, err := c.EncodeRequest(sized(plugin.OpEntries, MaxEntriesInput))
	if err != nil || len(b) != MaxEntriesInput {
		t.Fatalf("an entries request of 64 KiB: %d bytes, %v; want it taken", len(b), err)
	}
	_, err = c.EncodeRequest(sized(plugin.OpEntries, MaxEntriesInput+1))
	l, ok := errors.AsType[*plugin.LimitError](err)
	if !ok || l.What != plugin.LimitEntries || l.Size != MaxEntriesInput+1 || l.Cap != MaxEntriesInput {
		t.Fatalf("an entries request one byte over: %T %v; want a *plugin.LimitError of entries, 65537 over 65536", err, err)
	}
	if want := "over its limit of entries: 65537 bytes, over 65536"; !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "make the config and its files smaller") {
		t.Errorf("the refusal says %q, want it to say %q", err, want)
	}
	if _, err := c.EncodeRequest(sized(plugin.OpPlan, MaxEntriesInput+1)); err != nil {
		t.Errorf("a plan's request of 64 KiB and a byte: %v; want it taken, the cap is the entries call's", err)
	}
}

// TestAnUnknownMembersKeyIsPrintedShown plants a member the response does not
// name whose key holds an ESC sequence, a control character and a bidi
// override: the answer is refused at that key, as the plugin
// sent it, and its error prints the key escaped, so none of it reaches a
// terminal or a log line.
func TestAnUnknownMembersKeyIsPrintedShown(t *testing.T) {
	t.Parallel()
	const paint = "\x1b]0;owned\x07\x1b[2J\u009b31m\u202efdp"
	key, err := json.Marshal(paint)
	if err != nil {
		t.Fatal(err)
	}
	_, err = codec(t).DecodeRank(ok(`{"words":[],`+string(key)+`:1}`), 0)
	a, isAnswer := errors.AsType[*plugin.AnswerError](err)
	if !isAnswer {
		t.Fatalf("refused with %T %v, want an *plugin.AnswerError", err, err)
	}
	if a.Path != "/ok/"+paint {
		t.Fatalf("refused at %q, want the planted key as sent", a.Path)
	}
	got := a.Error()
	for _, r := range []rune{0x1b, 0x07, 0x9b, 0x202e} {
		if strings.ContainsRune(got, r) {
			t.Fatalf("the error prints %U raw: %q", r, got)
		}
	}
	if !strings.Contains(got, plugin.Shown(a.Path)) {
		t.Fatalf("the error does not say the key shown: %q", got)
	}
}

// TestAProgramNoGrantNamesIsSaidAsSent: the why names the plugin's program as
// it sent it, and AnswerError.Error escapes it once, so a backslash in it is
// printed doubled once, not twice.
func TestAProgramNoGrantNamesIsSaidAsSent(t *testing.T) {
	t.Parallel()
	_, err := codec(t).DecodePlan(ok(`{"final":true,"steps":[{"kind":"run","program":"a \\x","argv":["a"]}]}`), 0, grantsOf())
	a, isAnswer := errors.AsType[*plugin.AnswerError](err)
	if !isAnswer || a.Path != "/ok/steps/0/program" {
		t.Fatalf("refused with %T %v, want an *plugin.AnswerError at /ok/steps/0/program", err, err)
	}
	want := `the plugin's answer is not one at /ok/steps/0/program: names "a \\x", which is not a grant of the manifest`
	if got := a.Error(); got != want {
		t.Fatalf("the error is %q, want %q", got, want)
	}
}

// TestAnUngrantedProgramMatchesTheGrantSentinelAndStaysAnAnswerError: a step
// naming a program the manifest does not grant is port.ErrGrant to errors.Is,
// as the port's doc says, and is still the AnswerError at its path with the
// same text; a program that is no program word at all is an AnswerError and
// not a grant matter. Risk: the host adapter telling this
// refusal from the others by a string.
func TestAnUngrantedProgramMatchesTheGrantSentinelAndStaysAnAnswerError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		program   string
		wantGrant bool
	}{
		{"a program no grant names", "q", true},
		{"a program word that is none", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := codec(t).DecodePlan(ok(`{"final":true,"steps":[{"kind":"run","program":"`+tc.program+`","argv":["a"]}]}`), 0, grantsOf())
			if got := errors.Is(err, port.ErrGrant); got != tc.wantGrant {
				t.Errorf("errors.Is(err, port.ErrGrant) = %v for %T %v, want %v", got, err, err, tc.wantGrant)
			}
			a, isAnswer := errors.AsType[*plugin.AnswerError](err)
			if !isAnswer {
				t.Fatalf("refused with %T %v, want an *plugin.AnswerError", err, err)
			}
			if tc.wantGrant && (a.Path != "/ok/steps/0/program" || err.Error() != a.Error()) {
				t.Errorf("path %q and text %q, want /ok/steps/0/program and the AnswerError's own text %q", a.Path, err.Error(), a.Error())
			}
		})
	}
}
