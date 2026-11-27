package porttest_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// control is the label of the run that must pass; every other label is a plant, which must fail.
const control = "control"

// wants is, by plant label, a line the suite's failure must hold: the reason the plant fails, so a plant that
// fails for another reason (a broken fake, a suite that no longer reaches the fault) is red and named.
func wants() map[string]string {
	return map[string]string{
		"a suite that passes":                                 "unused",
		"a suite that fails":                                  "the reason it fails",
		"frozen":                                              "Now is 1970-01-01",
		"ahead":                                               "Now is 2100-01-01",
		"a relative path":                                     "the relative path \"",
		"the refused file read":                               "ReadOwned of the refused file = \"outside\\n\", <nil>",
		"an empty identity and no error":                      "Who with none configured = {Name: Email:} and no error",
		"one fixed identity":                                  "Who with none configured = {Name:Ada Example",
		"the index as the branch's tree":                      "Files(index)",
		"an empty file as nil":                                "want an empty slice",
		"a missing path as another error":                     "no such path; want fs.ErrNotExist",
		"a link as a file":                                    "{Path:ln Size:6 Dir:false Exec:false Link:false Special:false Object:}",
		"one object id for every content":                     "want three object ids for its three contents",
		"an entry with no object id":                          "README has no object id; want the name of its content",
		"the index's object ids as the work tree's":           "rules/a.yaml edited at the index's size and not staged",
		"a work-tree link hashed as its target":               "WorkObjects: \"wl\" = \"",
		"the messages backwards":                              "\"join\\n\"} {\"67298d66",
		"each message trimmed":                                "\"two\"}], <nil>; want",
		"a merge's first parent alone":                        "\"join\\n\"}], <nil>; want [{\"67298d66",
		"a range with none as nil":                            "= []model.Message(nil), <nil>; want an empty list",
		"an end that is no full name as an empty range":       "= [], <nil>; want refused, and no list",
		"the index's link read as a file":                     "Blob(index, ln) = \"README\", <nil>; want fs.ErrNotExist",
		"a ref named MERGE_HEAD as the merge":                 "Merging = []string{\"67298d66",
		"the commits with no parents":                         "Commits(main, side) = [{ID:67298d661cbab5b2640eac24b984b999593da1c9 Parents:[]}]",
		"the commits newest first":                            "Commits(lone, main) = [{ID:fc5fc79b053c8cef2900e0677ef63e1ad42740ad Parents:[bf9414a9",
		"the commits of to whole, from not taken off":         "Commits(main, side) = [{ID:bf9414a92dbd8ac3abfc53ede9b2141c7d12b671 Parents:[]}",
		"a message cut at its NUL":                            "\"hidden: a subject\\n\"}], <nil>; want refused, and no list",
		"a message under a header whose encoding is not read": "ISO-8859-1\\n\"}], <nil>; want ref",
		"the tags backwards":                                  "Tags = [{Name:v2",
		"follows a link":                                      "Read(\"state/HEAD\") = <nil>, want ErrOutside",
		"reads any name":                                      "ReadSoleWithin(\"rules/twin.yaml\") = \"a\\n\", <nil>, want no bytes and ErrManyNames",
		"reads past its bound":                                "ReadSoleWithin(rules/b.yaml, 1) = \"b\\n\", <nil>, want no bytes and ErrOverBound",
		"places a link":                                       "Within(\"/work/tree/history.jsonl\") = \"history.jsonl\", <nil>, want ErrLinked and no path",
	}
}

// passes is whether the suite run by f passed, and, for a plant, whether the planted fault is the reason it
// failed: the suite runs as a test of its own, its report is held back from this run's (a count that is never
// zero on green once hid a real failure), and the report is the answer. A plant must fail,
// and the report must hold want, a line of the failure the suite is expected to give; the control (label
// control, want empty) passes. A suite that failed for another reason fails the asking test, naming what
// it wanted and what the suite said.
//
// The suite runs under the asking test's own name and is matched by the real
// matcher, so a -run that selected the asking test selects the suite and a
// -skip that let the asking test run lets the suite run. (A matcher that said
// true to every pattern made a non-empty -skip skip the suite itself: it ran
// nothing, RunTests reported ok and the planted fault "passed".)
// A suite that ran no test fails the asking test as an error naming it; it is
// never a pass.
func passes(t *testing.T, label, want string, f func(t *testing.T)) bool {
	t.Helper()
	if label != control && want == "" {
		t.Fatalf("%s: the plant %q names no expected failure", t.Name(), label)
	}
	ran := false
	ok, report := runQuietly(t, func() bool {
		return testing.RunTests(regexp.MatchString, []testing.InternalTest{{Name: t.Name(), F: func(t *testing.T) {
			t.Helper()
			ran = true
			f(t)
		}}})
	})
	// The report of a planted fault is expected output: on the test stream
	// its "--- FAIL" lines were counted as 66 failures of a passing package.
	// It is shown only when the asking test fails.
	t.Cleanup(func() {
		if t.Failed() && report() != "" {
			t.Logf("the suite's own report:\n%s", report())
		}
	})
	if !ran {
		t.Fatalf("%s: the suite ran no test, so its verdict on the planted fault is no verdict", t.Name())
	}
	if !ok && label != control && !strings.Contains(report(), want) {
		t.Errorf("%s %q: the suite failed, but not as expected: want %q in its report", t.Name(), label, want)
	}
	return ok
}

// quietMu serialises runQuietly: it swaps the process's os.Stdout, which
// testing.RunTests reads when it starts. The tests already running keep the
// file they began with, so only a nested suite's report is held back.
var quietMu sync.Mutex

// runQuietly calls run with os.Stdout pointing at a file of its own, and
// answers run's result and, as a func, what the nested suite printed. A
// failure to make the file fails t.
func runQuietly(t *testing.T, run func() bool) (bool, func() string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "report")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.Close() })
	quietMu.Lock()
	saved := os.Stdout
	os.Stdout = out
	ok := func() bool {
		defer func() {
			os.Stdout = saved
			quietMu.Unlock()
		}()
		return run()
	}()
	return ok, func() string {
		b, err := os.ReadFile(out.Name())
		if err != nil {
			return err.Error()
		}
		return string(b)
	}
}

// frozen is a clock stopped at the epoch.
type frozen struct{}

func (frozen) Now() time.Time { return time.Unix(0, 0).UTC() }

// ahead is a clock stopped in 2100.
type ahead struct{}

func (ahead) Now() time.Time { return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC) }

// expected is the instant the suite is told to expect, and the control clock
// is at it.
func atExpected() time.Time { return time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC) }

type onTime struct{}

func (onTime) Now() time.Time { return atExpected() }

// TestAClockFrozenAtTheEpochFailsTheClockSuite: a planted adapter never goes
// back, is in UTC and is no zero time, and is 56 years behind the expected
// instant; another is 74 years ahead of it (the upper bound).
// The control is a clock at the expected instant.
func TestAClockFrozenAtTheEpochFailsTheClockSuite(t *testing.T) {
	t.Parallel()
	if passes(t, "frozen", wants()["frozen"], func(t *testing.T) {
		t.Helper()
		contracttest.Clock(t, func(*testing.T) port.Clock { return frozen{} }, atExpected)
	}) {
		t.Error("a clock frozen at Unix(0) passed the suite")
	}
	if passes(t, "ahead", wants()["ahead"], func(t *testing.T) {
		t.Helper()
		contracttest.Clock(t, func(*testing.T) port.Clock { return ahead{} }, atExpected)
	}) {
		t.Error("a clock stopped in 2100 passed the suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Clock(t, func(*testing.T) port.Clock { return onTime{} }, atExpected)
	}) {
		t.Error("the control, a clock at the expected instant, failed the suite")
	}
}

// readsAnyPath is the fake owned port with ReadOwned taking a relative path
// too, and reading it from the host's disk.
type readsAnyPath struct{ port.Owned }

func (r readsAnyPath) ReadOwned(p string) ([]byte, error) {
	if filepath.IsAbs(p) {
		return r.Owned.ReadOwned(p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// TestReadOwnedAcceptingARelativePathFailsTheOwnedSuite: the file outside the
// roots is on disk, so the relative path to it exists and the planted adapter
// reads it. The control is the fake, which refuses a relative path.
func TestReadOwnedAcceptingARelativePathFailsTheOwnedSuite(t *testing.T) {
	t.Parallel()
	build := func(plant bool) contracttest.OwnedMaker {
		return func(t *testing.T) (port.Owned, contracttest.Outside) {
			t.Helper()
			outside := refusedOutside(t)
			if err := os.WriteFile(outside.File, []byte("outside\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			w := porttest.NewWorld(t)
			w.SeedOutside(outside.File, "outside\n")
			w.SeedRefused(outside.Refused, porttest.Refused{Why: "is writable by its group"})
			if plant {
				return readsAnyPath{w.Ports().Owned}, outside
			}
			return w.Ports().Owned, outside
		}
	}
	if passes(t, "a relative path", wants()["a relative path"], func(t *testing.T) {
		t.Helper()
		contracttest.Owned(t, build(true))
	}) {
		t.Error("a ReadOwned that takes a relative path passed the suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Owned(t, build(false))
	}) {
		t.Error("the control, a ReadOwned that refuses it, failed the suite")
	}
}

// refusedOutside is the two files outside the roots a planted Owned suite
// names, in a temporary folder at this system's absolute path.
func refusedOutside(t *testing.T) contracttest.Outside {
	t.Helper()
	dir := t.TempDir()
	return contracttest.Outside{File: filepath.Join(dir, "names"), Refused: filepath.Join(dir, "loose")}
}

// readsRefused is the fake owned port with ReadOwned reading the refused
// file as it reads any: a file another account can write is read as an owned
// one.
type readsRefused struct {
	port.Owned
	refused string
}

func (r readsRefused) ReadOwned(p string) ([]byte, error) {
	if p == r.refused {
		return []byte("outside\n"), nil
	}
	return r.Owned.ReadOwned(p)
}

// TestAReadOwnedThatReadsAFileAnotherAccountCanWriteFailsTheOwnedSuite: the
// planted adapter reads the refused file as it reads any; the control is the
// fake, which refuses it as planted.
func TestAReadOwnedThatReadsAFileAnotherAccountCanWriteFailsTheOwnedSuite(t *testing.T) {
	t.Parallel()
	build := func(plant bool) contracttest.OwnedMaker {
		return func(t *testing.T) (port.Owned, contracttest.Outside) {
			t.Helper()
			outside := refusedOutside(t)
			w := porttest.NewWorld(t)
			w.SeedOutside(outside.File, "outside\n")
			w.SeedOutside(outside.Refused, "outside\n")
			w.SeedRefused(outside.Refused, porttest.Refused{Why: "is writable by its group"})
			if plant {
				return readsRefused{w.Ports().Owned, outside.Refused}, outside
			}
			return w.Ports().Owned, outside
		}
	}
	if passes(t, "the refused file read", wants()["the refused file read"], func(t *testing.T) {
		t.Helper()
		contracttest.Owned(t, build(true))
	}) {
		t.Error("a ReadOwned that reads a file another account can write passed the suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Owned(t, build(false))
	}) {
		t.Error("the control, a ReadOwned that refuses it, failed the suite")
	}
}

// whoFixed answers one identity whatever is configured, and none for nobody
// when empty is set: two planted adapters of the identity method.
type whoFixed struct {
	id    model.Identity
	empty bool
}

func (w whoFixed) Who(context.Context, model.Root) (model.Identity, error) {
	if w.empty {
		return model.Identity{}, nil
	}
	return w.id, nil
}

// whoSeeded answers what was configured, and an error for none: the control.
type whoSeeded struct{ who *model.Identity }

func (w whoSeeded) Who(context.Context, model.Root) (model.Identity, error) {
	if w.who == nil {
		return model.Identity{}, errors.New("none configured")
	}
	return *w.who, nil
}

// TestAWhoThatAnswersNobodyOrAConstantFailsTheWhoSuite: a planted adapter
// answers an empty identity with no error; another answers one fixed
// identity whatever is configured, so it never says none is. The control
// answers what is configured.
func TestAWhoThatAnswersNobodyOrAConstantFailsTheWhoSuite(t *testing.T) {
	t.Parallel()
	plants := map[string]func(*model.Identity) contracttest.Identifier{
		"an empty identity and no error": func(*model.Identity) contracttest.Identifier { return whoFixed{empty: true} },
		"one fixed identity":             func(*model.Identity) contracttest.Identifier { return whoFixed{id: contracttest.TheIdentity()} },
	}
	for name, plant := range plants {
		if passes(t, name, wants()[name], func(t *testing.T) {
			t.Helper()
			contracttest.Who(t, func(_ *testing.T, who *model.Identity) (contracttest.Identifier, model.Root) {
				return plant(who), "/work/book"
			})
		}) {
			t.Errorf("a Who that answers %s passed the suite", name)
		}
	}
	if len(plants) == 0 {
		t.Fatal("no plants")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Who(t, func(_ *testing.T, who *model.Identity) (contracttest.Identifier, model.Root) {
			return whoSeeded{who: who}, "/work/book"
		})
	}) {
		t.Error("the control, a Who that answers what is configured, failed the suite")
	}
}

// planted is a History that answers as the fake does, but for one method.
type planted struct {
	port.History
	files   func(ctx context.Context, root model.Root, rev string) ([]model.Entry, error)
	blob    func(ctx context.Context, root model.Root, rev, path string) ([]byte, error)
	tags    func(ctx context.Context, root model.Root) ([]model.Tag, error)
	merging func(ctx context.Context, root model.Root) ([]string, error)
	commits func(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error)
	msgs    func(ctx context.Context, root model.Root, base, head string) ([]model.Message, error)
	work    func(ctx context.Context, root model.Root, paths []string) ([]string, error)
}

func (p planted) WorkObjects(ctx context.Context, root model.Root, paths []string) ([]string, error) {
	if p.work != nil {
		return p.work(ctx, root, paths)
	}
	return p.History.WorkObjects(ctx, root, paths)
}

func (p planted) Merging(ctx context.Context, root model.Root) ([]string, error) {
	if p.merging != nil {
		return p.merging(ctx, root)
	}
	return p.History.Merging(ctx, root)
}

func (p planted) Commits(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error) {
	if p.commits != nil {
		return p.commits(ctx, root, from, to)
	}
	return p.History.Commits(ctx, root, from, to)
}

func (p planted) Files(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
	if p.files != nil {
		return p.files(ctx, root, rev)
	}
	return p.History.Files(ctx, root, rev)
}

func (p planted) Blob(ctx context.Context, root model.Root, rev, path string) ([]byte, error) {
	if p.blob != nil {
		return p.blob(ctx, root, rev, path)
	}
	return p.History.Blob(ctx, root, rev, path)
}

func (p planted) Tags(ctx context.Context, root model.Root) ([]model.Tag, error) {
	if p.tags != nil {
		return p.tags(ctx, root)
	}
	return p.History.Tags(ctx, root)
}

func (p planted) Messages(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
	if p.msgs != nil {
		return p.msgs(ctx, root, base, head)
	}
	return p.History.Messages(ctx, root, base, head)
}

// objectPlants are the Histories that answer an object id wrongly: one id for every content, an entry with no id, the index's
// ids as the work tree's, and a work-tree link hashed as its target.
func objectPlants() map[string]func(h port.History) port.History {
	return map[string]func(h port.History) port.History{
		"one object id for every content": func(h port.History) port.History {
			return planted{History: h, files: func(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
				files, err := h.Files(ctx, root, rev)
				for i := range files {
					files[i].Object = "same"
				}
				return files, err
			}}
		},
		"an entry with no object id": func(h port.History) port.History {
			return planted{History: h, files: func(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
				files, err := h.Files(ctx, root, rev)
				for i := range files {
					files[i].Object = ""
				}
				return files, err
			}}
		},
		"the index's object ids as the work tree's": func(h port.History) port.History {
			return planted{History: h, work: func(ctx context.Context, root model.Root, paths []string) ([]string, error) {
				files, err := h.Files(ctx, root, "")
				if err != nil {
					return nil, err
				}
				id := map[string]string{}
				for _, e := range files {
					id[e.Path] = e.Object
				}
				out := make([]string, len(paths))
				for i, p := range paths {
					out[i] = id[p]
				}
				return out, nil
			}}
		},
		"a work-tree link hashed as its target": func(h port.History) port.History {
			return planted{History: h, work: func(ctx context.Context, root model.Root, paths []string) ([]string, error) {
				followed := slices.Clone(paths)
				for i, p := range followed {
					if p == "wl" {
						followed[i] = "README"
					}
				}
				return h.WorkObjects(ctx, root, followed)
			}}
		},
	}
}

// TestAHistoryThatReadsTheWrongTreeFailsTheHistorySuite: planted Histories
// answer the index as the branch's tree, an empty file as nil, a path the
// rev lacks as some other error, the tags backwards, a tag named MERGE_HEAD
// as the merge in progress, the commits between two revs with no parents,
// newest first, or with the commits of from left in, a link as a file, one
// object id for every content, an entry with no object id, the index's ids
// as the work tree's, a work-tree link hashed as its target, the
// messages backwards, trimmed or of a merge's first parent alone, a range
// with none as nil, an end that is no full name as an empty range, and the
// index's link read as a file; and
// fakes seeded to answer a message cut at its NUL, or under a header whose
// encoding is not read; each fails the suite. The control,
// the fake as it is, passes.
func TestAHistoryThatReadsTheWrongTreeFailsTheHistorySuite(t *testing.T) {
	t.Parallel()
	plants := map[string]func(h port.History) port.History{
		"the index as the branch's tree": func(h port.History) port.History {
			return planted{History: h, files: func(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
				if rev == "" {
					rev = "main"
				}
				return h.Files(ctx, root, rev)
			}}
		},
		"an empty file as nil": func(h port.History) port.History {
			return planted{History: h, blob: func(ctx context.Context, root model.Root, rev, path string) ([]byte, error) {
				b, err := h.Blob(ctx, root, rev, path)
				if len(b) == 0 {
					return nil, err
				}
				return b, err
			}}
		},
		"a missing path as another error": func(h port.History) port.History {
			return planted{History: h, blob: func(ctx context.Context, root model.Root, rev, path string) ([]byte, error) {
				b, err := h.Blob(ctx, root, rev, path)
				if errors.Is(err, fs.ErrNotExist) {
					return nil, errors.New("no such path")
				}
				return b, err
			}}
		},
		"a link as a file": func(h port.History) port.History {
			return planted{History: h, files: func(ctx context.Context, root model.Root, rev string) ([]model.Entry, error) {
				files, err := h.Files(ctx, root, rev)
				for i := range files {
					files[i].Link = false
				}
				return files, err
			}}
		},
		"the messages backwards": func(h port.History) port.History {
			return planted{History: h, msgs: func(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
				msgs, err := h.Messages(ctx, root, base, head)
				slices.Reverse(msgs)
				return msgs, err
			}}
		},
		"each message trimmed": func(h port.History) port.History {
			return planted{History: h, msgs: func(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
				msgs, err := h.Messages(ctx, root, base, head)
				for i := range msgs {
					msgs[i].Text = strings.TrimSpace(msgs[i].Text)
				}
				return msgs, err
			}}
		},
		"a merge's first parent alone": func(h port.History) port.History {
			return planted{History: h, msgs: func(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
				// side reaches join only through its second parent's line
				msgs, err := h.Messages(ctx, root, base, head)
				return slices.DeleteFunc(msgs, func(m model.Message) bool { return m.Text == "side\n" && len(msgs) > 1 }), err
			}}
		},
		"a range with none as nil": func(h port.History) port.History {
			return planted{History: h, msgs: func(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
				msgs, err := h.Messages(ctx, root, base, head)
				if len(msgs) == 0 {
					return nil, err
				}
				return msgs, err
			}}
		},
		"an end that is no full name as an empty range": func(h port.History) port.History {
			return planted{History: h, msgs: func(ctx context.Context, root model.Root, base, head string) ([]model.Message, error) {
				msgs, err := h.Messages(ctx, root, base, head)
				if err != nil {
					return []model.Message{}, nil
				}
				return msgs, err
			}}
		},
		"the index's link read as a file": func(h port.History) port.History {
			return planted{History: h, blob: func(ctx context.Context, root model.Root, rev, path string) ([]byte, error) {
				if rev == "" && path == "ln" {
					return []byte("README"), nil
				}
				return h.Blob(ctx, root, rev, path)
			}}
		},
		"the tags backwards": func(h port.History) port.History {
			return planted{History: h, tags: func(ctx context.Context, root model.Root) ([]model.Tag, error) {
				tags, err := h.Tags(ctx, root)
				slices.Reverse(tags)
				return tags, err
			}}
		},
		"a ref named MERGE_HEAD as the merge": func(h port.History) port.History {
			return planted{History: h, merging: func(ctx context.Context, root model.Root) ([]string, error) {
				tags, err := h.Tags(ctx, root)
				out := []string{}
				for _, tag := range tags {
					if tag.Name == "MERGE_HEAD" {
						out = append(out, tag.Commit)
					}
				}
				return out, err
			}}
		},
		"the commits with no parents": func(h port.History) port.History {
			return planted{History: h, commits: func(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error) {
				cs, err := h.Commits(ctx, root, from, to)
				for i := range cs {
					cs[i].Parents = nil
				}
				return cs, err
			}}
		},
		"the commits newest first": func(h port.History) port.History {
			return planted{History: h, commits: func(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error) {
				cs, err := h.Commits(ctx, root, from, to)
				slices.Reverse(cs)
				return cs, err
			}}
		},
		"the commits of to whole, from not taken off": func(h port.History) port.History {
			return planted{History: h, commits: func(ctx context.Context, root model.Root, from, to string) ([]model.Commit, error) {
				if _, err := h.Commits(ctx, root, from, to); err != nil {
					return nil, err
				}
				return h.Commits(ctx, root, "lone", to)
			}}
		},
	}
	maps.Copy(plants, objectPlants())
	seeds := map[string]func(r *porttest.Repo){
		"a message cut at its NUL": func(r *porttest.Repo) {
			for k, msgs := range r.Logs {
				for i := range msgs {
					msgs[i].Text, _, _ = strings.Cut(msgs[i].Text, "\x00")
				}
				r.Logs[k] = msgs
			}
		},
		"a message under a header whose encoding is not read": func(r *porttest.Repo) { r.Encodings = nil },
	}
	seeded := func(seed func(*porttest.Repo), plant func(port.History) port.History) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			contracttest.History(t, func(t *testing.T, f contracttest.HistoryFixture) (port.History, model.Root) {
				t.Helper()
				w := porttest.NewWorld(t)
				r := repoOf(f)
				seed(&r)
				w.SeedRepo("/work/book", r)
				seedWork(w, "/work/book", f)
				return plant(w.Ports().History), "/work/book"
			})
		}
	}
	suite := func(plant func(port.History) port.History) func(t *testing.T) {
		return seeded(func(*porttest.Repo) {}, plant)
	}
	for name, plant := range plants {
		if passes(t, name, wants()[name], suite(plant)) {
			t.Errorf("a History that answers %s passed the suite", name)
		}
	}
	for name, seed := range seeds {
		if passes(t, name, wants()[name], seeded(seed, func(h port.History) port.History { return h })) {
			t.Errorf("a History that answers %s passed the suite", name)
		}
	}
	if len(plants) == 0 {
		t.Fatal("no plants")
	}
	if !passes(t, control, "", suite(func(h port.History) port.History { return h })) {
		t.Error("the control, the fake as it is, failed the suite")
	}
}

// TestThePlantRunnerSeesASuiteThatPassesAndOneThatFailsForItsReason: a suite
// that passes under a plant's label is answered as a pass (so the plant tests
// that expect a failure are red, named by the plant), and a failing one is
// answered as a failure whose report is held back, so this run's report holds
// no inner failure.
func TestThePlantRunnerSeesASuiteThatPassesAndOneThatFailsForItsReason(t *testing.T) {
	t.Parallel()
	if !passes(t, "a suite that passes", wants()["a suite that passes"], func(*testing.T) {}) {
		t.Error("a suite that passes was answered as a failure")
	}
	if passes(t, "a suite that fails", wants()["a suite that fails"], func(t *testing.T) {
		t.Helper()
		t.Error("the reason it fails: planted")
	}) {
		t.Error("a suite that fails was answered as a pass")
	}
}
