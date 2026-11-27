package fs_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"

	testenv "github.com/birdtable/go-testkit"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// standin is the approval of the planted plugin, standin.
func standin(i int) model.Approval {
	return model.Approval{Digest: contracttest.DigestOf(i), Ref: "standin@v1", Grants: contracttest.DigestOf(1000 + i)}
}

// machine is one machine's state folder and a project beside it, as a run of
// gragas from the project sees them. The state folder is short
// (tests.isolation.5) and is not made: the first Approve makes it.
type machine struct {
	dir, project string
}

// newMachine is a machine whose state folder lies outside the project.
func newMachine(t *testing.T) machine {
	t.Helper()
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	return machine{dir: filepath.Join(t.TempDir(), "state"), project: project}
}

// trust is a run's Trust over m, its roots the project alone.
func (m machine) trust(t *testing.T) *hostfs.Approvals {
	t.Helper()
	return hostfs.NewApprovals(hostfs.Writer{Tree: newTree(t)}, lockInstant(), m.dir, nil, func() ([]string, error) { return []string{m.project}, nil })
}

// record is the path of m's record.
func (m machine) record() string { return filepath.Join(m.dir, "approvals.json") }

// plant writes the record of m as content, in a state folder private to this
// account.
func (m machine) plant(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.record(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTheApprovalsAdapterPassesTheTrustSuite(t *testing.T) {
	t.Parallel()
	contracttest.Trust(t, func(t *testing.T) contracttest.TrustSetup {
		t.Helper()
		m := newMachine(t)
		inside := machine{dir: filepath.Join(m.project, ".gragas", "state"), project: m.project}
		return contracttest.TrustSetup{A: m.trust(t), B: m.trust(t), Carried: inside.trust(t), Refuse: func(t *testing.T) error {
			t.Helper()
			m.plant(t, `{"version":1,"approvals":[]}`)
			trusttest.Loosen(t, m.record(), trusttest.GroupWrites)
			return port.ErrOwnedWritable
		}}
	})
}

// TestTheRecordIsTheSameBytesOnEveryRun: the record is canon's document of
// the approvals sorted by digest, the time of each from the Clock port, so the
// same approvals made in any order are the same bytes. These hold no grants by
// id, so the record is version 1.
func TestTheRecordIsTheSameBytesOnEveryRun(t *testing.T) {
	t.Parallel()
	read := func(order ...int) string {
		m := newMachine(t)
		x := m.trust(t)
		for _, i := range order {
			if err := x.Approve(t.Context(), standin(i)); err != nil {
				t.Fatal(err)
			}
		}
		b, err := os.ReadFile(m.record())
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	when := strconv.FormatInt(lockInstant().Now().Unix(), 10)
	want := `{
  "approvals": [
    {
      "at": ` + when + `,
      "digest": "` + string(contracttest.DigestOf(1)) + `",
      "grants": "` + string(contracttest.DigestOf(1001)) + `",
      "ref": "standin@v1"
    },
    {
      "at": ` + when + `,
      "digest": "` + string(contracttest.DigestOf(2)) + `",
      "grants": "` + string(contracttest.DigestOf(1002)) + `",
      "ref": "standin@v1"
    }
  ],
  "version": 1
}
`
	if got := read(2, 1); got != want {
		t.Errorf("the record is\n%s\nwant\n%s", got, want)
	}
	if a, b := read(1, 2), read(2, 1); a != b {
		t.Errorf("two orders give two records:\n%s\n%s", a, b)
	}
}

// TestARecordThatIsNotWholeIsRefusedByEachMethod: each record below is one the
// owned read refuses or one that is not whole JSON of its form. Approved
// answers false with the refusal, never true, and Approve and Revoke refuse
// it too and leave its bytes as they were. The control is the record whole,
// which answers yes (item 2).
func TestARecordThatIsNotWholeIsRefusedByEachMethod(t *testing.T) {
	t.Parallel()
	d := string(contracttest.DigestOf(1))
	g := string(contracttest.DigestOf(2))
	entry := `{"at":1,"digest":"` + d + `","grants":"` + g + `","ref":"standin"}`
	whole := `{"version":1,"approvals":[` + entry + `]}`
	t.Run("the control: the record whole is a yes", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		m.plant(t, whole)
		if ok, err := m.trust(t).Approved(t.Context(), contracttest.DigestOf(1)); !ok || err != nil {
			t.Errorf("Approved = %v, %v, want true", ok, err)
		}
	})
	for name, c := range map[string]struct {
		content string
		want    error
	}{
		"not JSON":                                {"standin", hostfs.ErrRecord},
		"a byte order mark":                       {"\xef\xbb\xbf" + whole, hostfs.ErrRecord},
		"data after the record":                   {whole + "{}", hostfs.ErrRecord},
		"a key named twice":                       {`{"version":1,"version":1,"approvals":[` + entry + `]}`, hostfs.ErrRecord},
		"a list, not an object":                   {`[` + entry + `]`, hostfs.ErrRecord},
		"no approvals key":                        {`{"version":1}`, hostfs.ErrRecord},
		"no version":                              {`{"approvals":[` + entry + `]}`, hostfs.ErrRecord},
		"a member it does not know":               {`{"version":1,"approvals":[` + entry + `],"all":true}`, hostfs.ErrRecord},
		"a newer version":                         {`{"version":3,"approvals":[` + entry + `]}`, hostfs.ErrRecord},
		"version 0":                               {`{"version":0,"approvals":[` + entry + `]}`, hostfs.ErrRecord},
		"a version as text":                       {`{"version":"1","approvals":[` + entry + `]}`, hostfs.ErrRecord},
		"approvals not a list":                    {`{"version":1,"approvals":{}}`, hostfs.ErrRecord},
		"an entry with a member it does not know": {`{"version":1,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","ref":"standin","yes":true}]}`, hostfs.ErrRecord},
		"an entry with no grants":                 {`{"version":1,"approvals":[{"at":1,"digest":"` + d + `","ref":"standin"}]}`, hostfs.ErrRecord},
		"a digest in capitals":                    {strings.Replace(whole, d, strings.ToUpper(d), 1), hostfs.ErrRecord},
		"a digest cut short":                      {strings.Replace(whole, d, d[:len(d)-1], 1), hostfs.ErrRecord},
		"an empty reference":                      {strings.Replace(whole, `"ref":"standin"`, `"ref":""`, 1), hostfs.ErrRecord},
		"a time before 0":                         {strings.Replace(whole, `"at":1`, `"at":-1`, 1), hostfs.ErrRecord},
		"a time that is a fraction":               {strings.Replace(whole, `"at":1`, `"at":1.5`, 1), hostfs.ErrRecord},
		"a digest approved twice":                 {`{"version":1,"approvals":[` + entry + `,` + entry + `]}`, hostfs.ErrRecord},
		"grants by id in version 1":               {`{"version":1,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","held":[],"ref":"standin"}]}`, hostfs.ErrRecord},
		"grants by id that are not a list":        {`{"version":2,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","held":{},"ref":"standin"}]}`, hostfs.ErrRecord},
		"grants by id out of order":               {`{"version":2,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","held":[{"id":"b","digest":"` + g + `"},{"id":"a","digest":"` + g + `"}],"ref":"standin"}]}`, hostfs.ErrRecord},
		"a grant by id twice":                     {`{"version":2,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","held":[{"id":"a","digest":"` + g + `"},{"id":"a","digest":"` + g + `"}],"ref":"standin"}]}`, hostfs.ErrRecord},
		"a grant by id with a bad digest":         {`{"version":2,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","held":[{"id":"a","digest":"sha256:x"}],"ref":"standin"}]}`, hostfs.ErrRecord},
		"a grant by id with a member unknown":     {`{"version":2,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + g + `","held":[{"id":"a","digest":"` + g + `","x":1}],"ref":"standin"}]}`, hostfs.ErrRecord},
		"past the owned read's size":              {whole + strings.Repeat(" ", 1<<20), port.ErrOwnedTooLong},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			m.plant(t, c.content)
			refusedByEach(t, m, c.want)
		})
	}
	t.Run("a record its group can write", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		m.plant(t, whole)
		trusttest.Loosen(t, m.record(), trusttest.GroupWrites)
		refusedByEach(t, m, port.ErrNotOwned)
	})
	t.Run("a folder above the record its group can write", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		m.plant(t, whole)
		trusttest.Loosen(t, m.dir, trusttest.GroupWrites)
		refusedByEach(t, m, port.ErrNotOwned)
	})
	t.Run("a record this account cannot read", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		m.plant(t, whole)
		trusttest.Deny(t, m.record())
		refusedByEach(t, m, nil)
	})
	t.Run("a link at the record", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		elsewhere := filepath.Join(t.TempDir(), "approvals.json")
		if err := os.WriteFile(elsewhere, []byte(whole), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(m.dir, 0o700); err != nil {
			t.Fatal(err)
		}
		testenv.Link(t, elsewhere, m.record())
		refusedByEach(t, m, port.ErrOwnedLink)
	})
	// The probe: the record a
	// second name of the project's own approvals.json, which holds a yes, so
	// what writes that file in the clone would write this machine's record.
	t.Run("a hard link to a file of the project", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		carried := filepath.Join(m.project, "approvals.json")
		if err := os.WriteFile(carried, []byte(whole), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(m.dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(carried, m.record()); err != nil {
			t.Fatal(err)
		}
		refusedByEach(t, m, port.ErrManyNames)
	})
	t.Run("a folder at the record", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		if err := os.MkdirAll(m.record(), 0o700); err != nil {
			t.Fatal(err)
		}
		refusedByEach(t, m, port.ErrNotOwned)
	})
}

// refusedByEach holds that every method of a Trust over m refuses its
// record, matching want when it is set, and that the record's bytes, or the
// folder at its name, stay as they were.
func refusedByEach(t *testing.T, m machine, want error) {
	t.Helper()
	before := atRecord(t, m.record())
	x := m.trust(t)
	refused := func(what string, err error) {
		t.Helper()
		if err == nil || (want != nil && !errors.Is(err, want)) {
			t.Errorf("%s = %v, want a refusal matching %v", what, err, want)
		}
	}
	ok, err := x.Approved(t.Context(), contracttest.DigestOf(1))
	if ok {
		t.Errorf("Approved = true, %v; a refused record is never a yes", err)
	}
	refused("Approved", err)
	refused("Approve", x.Approve(t.Context(), standin(3)))
	refused("Revoke", x.Revoke(t.Context(), contracttest.DigestOf(1)))
	if after := atRecord(t, m.record()); after != before {
		t.Errorf("the record changed under the refusals:\n%q\nwas\n%q", after, before)
	}
}

// atRecord is what is at path, read without following a link: a file's
// bytes, a link's target, or the word folder.
func atRecord(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		return err.Error()
	}
	if to, err := os.Readlink(path); err == nil {
		return "a link to " + to
	}
	if info.IsDir() {
		return "a folder"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "a file that could not be read: " + err.Error()
	}
	return string(b)
}

// TestAnEmptyRecordIsNoYesAndNoError: a record with no approval answers
// false and no error, never true (item 2).
func TestAnEmptyRecordIsNoYesAndNoError(t *testing.T) {
	t.Parallel()
	m := newMachine(t)
	m.plant(t, `{"version":1,"approvals":[]}`)
	if ok, err := m.trust(t).Approved(t.Context(), contracttest.DigestOf(1)); ok || err != nil {
		t.Errorf("Approved of an empty record = %v, %v, want false and no error", ok, err)
	}
}

// TestNoRecordIsNoYesAndNoError: a machine that never approved
// a plugin has no record, and on a fresh one no state folder either. Both
// answer false and no error, as an empty record does, Prior is a first
// approval, none recorded for the name, and Revoke writes nothing. Risk: the
// first state of every machine read as a refusal, so Plugins.Open says the
// record could not be read where it should say not approved. Planted: before
// the fix Approved and Prior each answered the owned read's fs.ErrNotExist,
// naming the file.
func TestNoRecordIsNoYesAndNoError(t *testing.T) {
	t.Parallel()
	for name, prepare := range map[string]func(t *testing.T, m machine){
		"no state folder": func(*testing.T, machine) {},
		"an empty state folder": func(t *testing.T, m machine) {
			t.Helper()
			if err := os.Mkdir(m.dir, 0o700); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			prepare(t, m)
			a := m.trust(t)
			if ok, err := a.Approved(t.Context(), contracttest.DigestOf(1)); ok || err != nil {
				t.Errorf("Approved with no record = %v, %v, want false and no error", ok, err)
			}
			// Prior's first approval reads through the same
			// way in: none recorded for the name, not a missing file.
			if got, err := a.Prior(t.Context(), "standin"); !errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "approvals.json") || got.Digest != "" {
				t.Errorf("Prior with no record = %+v, %v, want no approval, fs.ErrNotExist and no word of the file", got, err)
			}
			if err := a.Revoke(t.Context(), contracttest.DigestOf(1)); err != nil {
				t.Errorf("Revoke with no record = %v, want nil", err)
			}
			if _, err := os.Lstat(m.record()); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a record is at %s after Approved and Revoke: %v", m.record(), err)
			}
		})
	}
}

// TestOnlyAMissingRecordIsEmpty: what is at the record's name
// and cannot be read stays an error, never the empty record: a link there
// that leads nowhere, and a state folder that is a file. Neither error
// matches fs.ErrNotExist, which is what lets held read that alone as no
// record. Risk: a record swapped for a dangling link read as a machine that
// approved nothing, and Approve writing over what is there. Planted: held
// read every error of the read as no record.
func TestOnlyAMissingRecordIsEmpty(t *testing.T) {
	t.Parallel()
	for name, plant := range map[string]func(t *testing.T, m machine){
		"a link at the record that leads nowhere": func(t *testing.T, m machine) {
			t.Helper()
			if err := os.Mkdir(m.dir, 0o700); err != nil {
				t.Fatal(err)
			}
			testenv.Link(t, filepath.Join(m.dir, "gone.json"), m.record())
		},
		"a state folder that is a file": func(t *testing.T, m machine) {
			t.Helper()
			if err := os.WriteFile(m.dir, []byte("not a folder"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			plant(t, m)
			ok, err := m.trust(t).Approved(t.Context(), contracttest.DigestOf(1))
			if ok || err == nil || errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Approved = %v, %v, want false and an error that is not fs.ErrNotExist", ok, err)
			}
		})
	}
}

// TestARecordAFolderOfACloneHoldsIsNotRead: a state folder inside the
// project, inside a rulebook, reached through a link into the project, or
// beneath a second spelling of the project's root, is refused by every
// method, matching port.ErrCarried. The record planted there says yes and is
// never read, and Approve writes nothing there (item 3).
func TestARecordAFolderOfACloneHoldsIsNotRead(t *testing.T) {
	t.Parallel()
	d := string(contracttest.DigestOf(1))
	yes := `{"version":1,"approvals":[{"at":1,"digest":"` + d + `","grants":"` + d + `","ref":"standin"}]}`
	for name, place := range map[string]func(t *testing.T, project string) (dir string, roots []string){
		"inside the project": func(_ *testing.T, project string) (string, []string) {
			return filepath.Join(project, ".gragas", "state"), []string{project}
		},
		"the project itself": func(_ *testing.T, project string) (string, []string) {
			return project, []string{project}
		},
		"inside a rulebook": func(t *testing.T, project string) (string, []string) {
			t.Helper()
			book := filepath.Join(t.TempDir(), "book")
			return filepath.Join(book, "state"), []string{project, book}
		},
		"through a link into the project": func(t *testing.T, project string) (string, []string) {
			t.Helper()
			link := filepath.Join(t.TempDir(), "link")
			testenv.Link(t, project, link)
			return filepath.Join(link, "state"), []string{project}
		},
		"beneath a second spelling of the root": func(t *testing.T, project string) (string, []string) {
			t.Helper()
			second := filepath.Join(t.TempDir(), "second")
			testenv.Link(t, project, second)
			return filepath.Join(project, "state"), []string{second}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			project := filepath.Join(t.TempDir(), "project")
			dir, roots := place(t, project)
			for _, r := range append([]string{project}, roots...) {
				if err := os.MkdirAll(r, 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
					t.Fatal(err)
				}
			}
			m := machine{dir: dir, project: project}
			m.plant(t, yes)
			x := hostfs.NewApprovals(hostfs.Writer{Tree: newTree(t)}, lockInstant(), dir, nil, func() ([]string, error) { return roots, nil })
			if ok, err := x.Approved(t.Context(), contracttest.DigestOf(1)); ok || !errors.Is(err, port.ErrCarried) {
				t.Errorf("Approved = %v, %v, want false and port.ErrCarried", ok, err)
			}
			if err := x.Approve(t.Context(), standin(2)); !errors.Is(err, port.ErrCarried) {
				t.Errorf("Approve = %v, want port.ErrCarried", err)
			}
			if err := x.Revoke(t.Context(), contracttest.DigestOf(1)); !errors.Is(err, port.ErrCarried) {
				t.Errorf("Revoke = %v, want port.ErrCarried", err)
			}
			if got := atRecord(t, m.record()); got != yes {
				t.Errorf("the record a clone carries changed: %q", got)
			}
		})
	}
	t.Run("the control: a state folder beside the project is read", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		m.plant(t, yes)
		if ok, err := m.trust(t).Approved(t.Context(), contracttest.DigestOf(1)); !ok || err != nil {
			t.Errorf("Approved = %v, %v, want true", ok, err)
		}
	})
}

// TestTheRootsAreNotOptional: an Approvals whose roots are nil, answer none,
// or fail refuses every method, so a forgotten argument never turns the check
// off. Only NewApprovalsOfNoClone, named for it, compares nothing.
func TestTheRootsAreNotOptional(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		roots func() ([]string, error)
		want  error
	}{
		"nil roots":       {nil, hostfs.ErrNoRoots},
		"no root":         {func() ([]string, error) { return nil, nil }, hostfs.ErrNoRoots},
		"roots that fail": {func() ([]string, error) { return nil, fs.ErrPermission }, fs.ErrPermission},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			x := hostfs.NewApprovals(hostfs.Writer{Tree: newTree(t)}, lockInstant(), m.dir, nil, c.roots)
			if ok, err := x.Approved(t.Context(), contracttest.DigestOf(1)); ok || !errors.Is(err, c.want) {
				t.Errorf("Approved = %v, %v, want false and %v", ok, err, c.want)
			}
			if err := x.Approve(t.Context(), standin(1)); !errors.Is(err, c.want) {
				t.Errorf("Approve = %v, want %v", err, c.want)
			}
			if err := x.Revoke(t.Context(), contracttest.DigestOf(1)); !errors.Is(err, c.want) {
				t.Errorf("Revoke = %v, want %v", err, c.want)
			}
			if _, err := os.Lstat(m.dir); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a refused call made the state folder: %v", err)
			}
		})
	}
	t.Run("a zero Approvals refuses", func(t *testing.T) {
		t.Parallel()
		var x hostfs.Approvals
		if ok, err := x.Approved(t.Context(), contracttest.DigestOf(1)); ok || err == nil {
			t.Errorf("Approved = %v, %v, want a refusal", ok, err)
		}
	})
	t.Run("the constructor named for no clone compares nothing", func(t *testing.T) {
		t.Parallel()
		m := newMachine(t)
		x := hostfs.NewApprovalsOfNoClone(hostfs.Writer{Tree: newTree(t)}, lockInstant(), m.dir, nil)
		if err := x.Approve(t.Context(), standin(1)); err != nil {
			t.Fatalf("Approve = %v", err)
		}
		if ok, err := x.Approved(t.Context(), contracttest.DigestOf(1)); !ok || err != nil {
			t.Errorf("Approved = %v, %v, want true", ok, err)
		}
	})
}

// TestAWriteThatFailsLeavesTheOldRecordWhole: an Approve whose record cannot
// be written, its reference not UTF-8, which canon refuses, or its state
// folder one this account cannot write, leaves the record byte for byte as it
// was, and the approval made before it still holds (item 5). The record is
// replaced whole by Writer.Replace, go-files' write.FileIn, or not at all.
func TestAWriteThatFailsLeavesTheOldRecordWhole(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (machine, *hostfs.Approvals, []byte) {
		t.Helper()
		m := newMachine(t)
		x := m.trust(t)
		if err := x.Approve(t.Context(), standin(1)); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(m.record())
		if err != nil {
			t.Fatal(err)
		}
		return m, x, before
	}
	held := func(t *testing.T, m machine, x *hostfs.Approvals, before []byte) {
		t.Helper()
		if after, err := os.ReadFile(m.record()); err != nil || !bytes.Equal(after, before) {
			t.Errorf("the record is %q, %v, want it whole as it was: %q", after, err, before)
		}
		if ok, err := x.Approved(t.Context(), contracttest.DigestOf(1)); !ok || err != nil {
			t.Errorf("the approval made before = %v, %v, want true", ok, err)
		}
	}
	t.Run("a reference canon refuses", func(t *testing.T) {
		t.Parallel()
		m, x, before := setup(t)
		bad := standin(2)
		bad.Ref = "standin\xff"
		if err := x.Approve(t.Context(), bad); err == nil {
			t.Error("Approve of a reference that is not UTF-8 is no error")
		}
		held(t, m, x, before)
	})
	t.Run("a state folder this account cannot open", func(t *testing.T) {
		t.Parallel()
		m, x, before := setup(t)
		// The denial lasts until the inner test ends, which puts the
		// folder back, so the record is read after it.
		t.Run("denied", func(t *testing.T) {
			trusttest.Deny(t, m.dir)
			if err := x.Approve(t.Context(), standin(2)); err == nil {
				t.Error("Approve into a folder this account cannot open is no error")
			}
		})
		held(t, m, x, before)
	})
}

// TestApprovalsMadeAtOnceAllHold: eight runs approve at once on a machine
// whose state folder is not made yet, each a Trust of its own over its own
// Tree, as eight processes are. Each holds: the state folder's root lock orders
// the read and the replace, so none writes a record it read before another's
// back over it, and the runs that make the state folder together all take it
// (item 5).
func TestApprovalsMadeAtOnceAllHold(t *testing.T) {
	t.Parallel()
	const runs = 8
	m := newMachine(t)
	errs := make([]error, runs)
	var wg sync.WaitGroup
	for i := range runs {
		x := m.trust(t)
		wg.Go(func() { errs[i] = x.Approve(t.Context(), standin(i)) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("Approve %d = %v", i, err)
		}
	}
	x := m.trust(t)
	for i := range runs {
		if ok, err := x.Approved(t.Context(), contracttest.DigestOf(i)); !ok || err != nil {
			t.Errorf("approval %d, made beside %d others = %v, %v, want true", i, runs-1, ok, err)
		}
	}
}

// TestTheCarriedComparisonIsOneFunction: carried, the one comparison the
// callers share, refuses a state folder in a root and passes one beside it, and
// a root that cannot be resolved is refused, never taken to be clear.
func TestTheCarriedComparisonIsOneFunction(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	project := filepath.Join(base, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := hostfs.Carried(filepath.Join(project, "state"), []string{project}); !errors.Is(err, port.ErrCarried) {
		t.Errorf("a state folder in the project = %v, want port.ErrCarried", err)
	}
	if err := hostfs.Carried(filepath.Join(base, "state"), []string{project}); err != nil {
		t.Errorf("a state folder beside the project = %v, want no error", err)
	}
	loop := filepath.Join(base, "loop")
	testenv.Link(t, loop, loop)
	if err := hostfs.Carried(filepath.Join(base, "state"), []string{loop}); err == nil || errors.Is(err, port.ErrCarried) {
		t.Errorf("a root that is a loop of links = %v, want a refusal that is not ErrCarried", err)
	}
	if err := hostfs.Carried(filepath.Join(loop, "state"), []string{project}); err == nil {
		t.Error("a state folder beneath a loop of links is no error")
	}
	if err := os.Mkdir(filepath.Join(project, "inner"), 0o750); err != nil {
		t.Fatal(err)
	}
	sameSpelling(t, project, filepath.Join(base, "PROJECT"))
}

// sameSpelling is the row of a second spelling of the project that no link
// makes: its name in another case. The
// system says whether that is the same folder, and the row wants carried to
// say what the system says: refused as ErrCarried where the disk folds case,
// clear where it does not. Only the comparison by file identity (confine.Below)
// refuses it, since confine.Resolve keeps the case as written; the row is red
// with that comparison dropped.
func sameSpelling(t *testing.T, project, other string) {
	t.Helper()
	same := false
	if a, err := os.Stat(project); err != nil {
		t.Fatal(err)
	} else if b, err := os.Stat(other); err == nil {
		same = os.SameFile(a, b)
	}
	for _, dir := range []string{
		other,
		filepath.Join(other, "state"),
		filepath.Join(other, "inner"),
		filepath.Join(other, "inner", "state"),
	} {
		err := hostfs.Carried(dir, []string{project})
		switch {
		case same && !errors.Is(err, port.ErrCarried):
			t.Errorf("a state folder %s, the project in another case on a disk that folds it, = %v, want port.ErrCarried", dir, err)
		case !same && err != nil:
			t.Errorf("a state folder %s, another folder on a disk that keeps case, = %v, want no error", dir, err)
		}
	}
}

// TestALinkMadeIntoACloneAfterTheCheckMakesNothingThere: carried resolves the
// nearest folder of the state
// folder that is there and keeps the missing names as written, so a run that
// makes the missing state folder a link into the clone after that check and
// before the make must find the make refusing it, and the clone left as it
// was. Planted: make judging with no folder held apart (it then takes the
// project as the state folder, red here).
func TestALinkMadeIntoACloneAfterTheCheckMakesNothingThere(t *testing.T) {
	t.Parallel()
	m := newMachine(t)
	x := m.trust(t)
	apart, err := x.Where()
	if err != nil {
		t.Fatalf("Where = %v, want a state folder clear of the clone", err)
	}
	testenv.Link(t, m.project, m.dir)
	if err := x.Make(apart); err == nil {
		t.Error("the make through a link into the project is no error")
	}
	if got, err := os.ReadDir(m.project); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Errorf("the project holds %d entries after the refused make", len(got))
	}
	if err := x.Approve(t.Context(), standin(1)); !errors.Is(err, port.ErrCarried) {
		t.Errorf("Approve over the link = %v, want ErrCarried", err)
	}
}

// TestARecordThatGrowsPastMaxOwnedBetweenTheTwoReadsIsRefused: the record is
// judged by the owned read at trust.MaxOwned and then read again by the sole
// read, which holds the same bound.
// Planted: a record that grows to MaxOwned+1 bytes after the owned read is
// refused, matching pluginbundle.ErrOverBound, and not read whole. With the
// sole read's bound at -1, which the adapter reads as none, it is read whole and
// refused as changed, not as over its bound. Risk: a record that grows after it
// was judged is read to its end into memory.
func TestARecordThatGrowsPastMaxOwnedBetweenTheTwoReadsIsRefused(t *testing.T) {
	t.Parallel()
	m := newMachine(t)
	m.plant(t, `{"version":1,"approvals":[]}`)
	tr := newTree(t)
	a := hostfs.NewApprovals(hostfs.Writer{Tree: tr}, lockInstant(), m.dir, nil, func() ([]string, error) { return []string{m.project}, nil })
	grown := false
	hostfs.WithOpening(tr, func(path string) {
		if grown || !strings.HasSuffix(path, "approvals.json") {
			return
		}
		grown = true
		if err := os.WriteFile(m.record(), bytes.Repeat([]byte{' '}, hosttrust.MaxOwned+1), 0o600); err != nil {
			t.Error(err)
		}
	})
	_, err := a.Approved(t.Context(), contracttest.DigestOf(1))
	if !grown {
		t.Fatal("the record was never opened by the sole read")
	}
	if !errors.Is(err, pluginbundle.ErrOverBound) {
		t.Fatalf("a record grown to MaxOwned+1 bytes: %v, want ErrOverBound", err)
	}
}
