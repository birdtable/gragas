package census_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/census"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// The fake go tool's ports: two systems on the arch x, and one of them on
// y too, which the other lacks, so y is left out.
const dist = `[
 {"GOOS": "aos", "GOARCH": "x", "FirstClass": true},
 {"GOOS": "bos", "GOARCH": "x", "FirstClass": true},
 {"GOOS": "aos", "GOARCH": "y", "FirstClass": true},
 {"GOOS": "cos", "GOARCH": "x", "FirstClass": false}
]`

// fakeGo is a go tool that answers the dist list with dist and go list std
// with listing[GOOS]; it starts no program.
func fakeGo(dist string, listing map[string]string) census.Go {
	return func(_ context.Context, env []string, args ...string) ([]byte, error) {
		if slices.Equal(args, census.DistArgs()) {
			return []byte(dist), nil
		}
		if !slices.Equal(args, census.ListArgs()) {
			return nil, errors.New("the fake go tool was asked " + strings.Join(args, " "))
		}
		for _, e := range env {
			if goos, ok := strings.CutPrefix(e, "GOOS="); ok {
				return []byte(listing[goos]), nil
			}
		}
		return nil, errors.New("the fake go tool was given no GOOS")
	}
}

// goroot is a GOROOT that plants one identifier of each reason in package
// p, one that differs in nothing, a package of internal no program may
// import, and runtime. Each reach plant is red when its rule goes: TwoHop
// without the fixed point (it sorts before mid), Inner and Other without
// internal packages or a value's names, CallsMethod without the dot of a
// method's name, ViaMethod without exported methods, and runtime.Caller
// without the stop at runtime's helpers.
func goroot() fstest.MapFS {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return fstest.MapFS{
		"src/p/common.go": file(`package p

import (
	"internal/q"
	"syscall"
)

// Same is the same everywhere.
func Same() {}

// Helped calls a helper each system has its own of.
func Helped() { helper() }

// TwoHop calls a helper that calls one each system has its own of.
func TwoHop() { mid() }

func mid() { helper() }

// Inner calls an internal package's helper each system has its own of.
func Inner() { q.PerSys() }

// Other is another package's constant, which each system has its own of.
const Other = q.Sep

// T holds methods.
type T struct{}

// helper is a method named as the per-system func is.
func (T) helper() {}

// CallsMethod calls the method, which is the same everywhere.
func CallsMethod() { T{}.helper() }

// Exported calls the per-system helper.
func (T) Exported() { helper() }

// ViaMethod reaches the helper through the exported method.
func ViaMethod() { T{}.Exported() }

// Sys reaches syscall.
func Sys() error { return syscall.EINVAL }

// Wraps reaches Sys through settle, a root.
func Wraps() error { return settle() }

func settle() error { return Sys() }

// Doc says one thing. It works on aos alone.
func Doc() {}
`),
		"src/p/p_a.go":             file("package p\n\nfunc helper() {}\n\n// Only is one system's.\nfunc Only() {}\n\nconst Sep = '/'\n"),
		"src/p/p_b.go":             file("package p\n\nfunc helper() {}\n\nconst Sep = '\\\\'\n"),
		"src/internal/q/q.go":      file("package q\n\nfunc Hidden() {}\n"),
		"src/internal/q/q_a.go":    file("package q\n\nfunc PerSys() {}\n\nconst Sep = 1\n"),
		"src/internal/q/q_b.go":    file("package q\n\nfunc PerSys() {}\n\nconst Sep = 2\n"),
		"src/runtime/common.go":    file("package runtime\n\n// Caller is judged on its own declaration.\nfunc Caller() { callers() }\n"),
		"src/runtime/runtime_a.go": file("package runtime\n\nfunc callers() {}\n"),
		"src/runtime/runtime_b.go": file("package runtime\n\nfunc callers() {}\n"),
	}
}

func listing() map[string]string {
	return map[string]string{
		"aos": "p common.go p_a.go\ninternal/q q.go q_a.go\nruntime common.go runtime_a.go\n",
		"bos": "p common.go p_b.go\ninternal/q q.go q_b.go\nruntime common.go runtime_b.go\n",
	}
}

func TestTheCensusFindsEachReason(t *testing.T) {
	t.Parallel()
	c, err := census.Take(t.Context(), fakeGo(dist, listing()), goroot())
	if err != nil {
		t.Fatal(err)
	}
	if want := []census.Port{{GOOS: "aos", GOARCH: "x"}, {GOOS: "bos", GOARCH: "x"}}; !slices.Equal(c.Ports, want) {
		t.Errorf("the ports are %v, want %v: an arch some first-class system lacks is left out, and a port not first class too", c.Ports, want)
	}
	if c.Packages != 2 || c.Read != 15 {
		t.Errorf("read %d packages and %d identifiers, want 2 and 15: internal/q is no program's to import", c.Packages, c.Read)
	}
	reach := []string{census.WhyReach}
	want := []census.Ident{
		{Name: "p.Doc", Why: []string{census.WhyDoc}, Doc: "It works on aos alone."},
		{Name: "p.Helped", Why: reach, Via: "p.helper"},
		{Name: "p.Inner", Why: reach, Via: "internal/q.PerSys"},
		{Name: "p.Only", Why: []string{census.WhyPartial, census.WhyFile}},
		{Name: "p.Other", Why: reach, Via: "internal/q.Sep"},
		{Name: "p.Sep", Why: []string{census.WhyDefined, census.WhyFile}},
		{Name: "p.Sys", Why: []string{census.WhySyscall}},
		{Name: "p.T.Exported", Why: reach, Via: "p.helper"},
		{Name: "p.TwoHop", Why: reach, Via: "p.mid"},
		{Name: "p.ViaMethod", Why: reach, Via: "p..Exported"},
		{Name: "p.Wraps", Why: reach, Via: "p.settle"},
	}
	if !slices.EqualFunc(c.Differ, want, func(a, b census.Ident) bool {
		return a.Name == b.Name && a.Doc == b.Doc && a.Via == b.Via && slices.Equal(a.Why, b.Why)
	}) {
		t.Errorf("the census found\n%v\nwant\n%v", c.Differ, want)
	}
	if got := c.TSV(); !strings.Contains(got, "p.Doc\t"+census.WhyDoc+"\tIt works on aos alone.\t\n") {
		t.Errorf("the TSV does not carry the doc sentence that matched:\n%s", got)
	}
}

func TestACensusWithNothingToReadIsRefused(t *testing.T) {
	t.Parallel()
	noFirst := `[{"GOOS": "aos", "GOARCH": "x", "FirstClass": false}]`
	if _, err := census.Take(t.Context(), fakeGo(noFirst, listing()), goroot()); !errors.Is(err, census.ErrNoPort) {
		t.Errorf("a dist list with no first-class port gave %v, want %v", err, census.ErrNoPort)
	}
	empty := map[string]string{"aos": "p\n", "bos": "p\n"}
	if _, err := census.Take(t.Context(), fakeGo(dist, empty), goroot()); !errors.Is(err, census.ErrNoIdent) {
		t.Errorf("a listing whose packages build no file gave %v, want %v", err, census.ErrNoIdent)
	}
	none := map[string]string{"aos": "", "bos": ""}
	if _, err := census.Take(t.Context(), fakeGo(dist, none), goroot()); err == nil {
		t.Error("a listing that names no package was taken as a census")
	}
	if _, err := census.Take(t.Context(), fakeGo(dist, listing()), fstest.MapFS{}); err == nil {
		t.Error("a GOROOT that holds none of the files the listing names was taken as a census")
	}
}

// theCensus is a census of three identifiers, two of one package.
func theCensus() *census.Census {
	return &census.Census{Differ: []census.Ident{
		{Name: "os.Exit", Why: []string{census.WhySyscall}},
		{Name: "os/exec.Cmd.Run", Why: []string{census.WhyDoc}},
		{Name: "syscall.Kill", Why: []string{census.WhyPartial}},
	}}
}

// theLint holds a row for each ban of theList.
func theLint() census.Lint {
	return census.Lint{
		Forbid: []census.Forbid{{Pattern: `^os\.Exit$`, Msg: "go.errors.4: only main exits"}},
		Deny:   []census.Deny{{Pkg: "syscall", Desc: "arch.ports.24: no syscall", Files: []string{"$all"}}},
	}
}

const theList = `# a comment and a blank line say nothing

os.Exit ban go.errors.4
os/exec.Cmd.Run later #1750 L5
syscall.* ban arch.ports.24
`

func TestTheListIsHeldToTheCensusAndTheLint(t *testing.T) {
	t.Parallel()
	ds, bad := census.ParseDecisions(theList)
	if len(bad) != 0 || len(ds) != 3 {
		t.Fatalf("the list read as %v, with %v", ds, bad)
	}
	if got := census.Hold(theCensus(), ds, theLint()); len(got) != 0 {
		t.Errorf("the list with its census and lint is refused: %v", got)
	}
	for _, tc := range []struct {
		name string
		c    *census.Census
		list string
		lint census.Lint
		want string
	}{
		{"an undecided identifier", &census.Census{Differ: append(theCensus().Differ, census.Ident{Name: "os.Getpid", Why: []string{census.WhySyscall}})}, theList, theLint(),
			"os.Getpid differs by system (reaches syscall) and has no line in gate/os-api.txt"},
		{"a line the census does not find", &census.Census{Differ: theCensus().Differ[1:]}, theList, theLint(),
			"os.Exit names nothing the census found"},
		{"a package line the census finds nothing of", &census.Census{Differ: theCensus().Differ[:2]}, theList, theLint(),
			"syscall.* names nothing the census found"},
		{"an identifier decided twice", theCensus(), theList + "syscall.Kill allow a reason\n", theLint(),
			"syscall.Kill is decided by its own line and by line"},
		{"a ban with no forbidigo row", theCensus(), theList, census.Lint{Deny: theLint().Deny},
			"os.Exit is banned under go.errors.4 and no forbidigo row matches os.Exit"},
		{"a ban whose row is another rule's", theCensus(), theList, census.Lint{Forbid: []census.Forbid{{Pattern: `^os\.Exit$`, Msg: "cli.config.3: no"}}, Deny: theLint().Deny},
			"no forbidigo row matches os.Exit with a message that opens with go.errors.4"},
		{"a ban whose row is another package's", theCensus(), theList, census.Lint{Forbid: []census.Forbid{{Pattern: `^os\.Exit$`, Pkg: `^other/os$`, Msg: "go.errors.4: no"}}, Deny: theLint().Deny},
			"no forbidigo row matches os.Exit"},
		{"a package ban with no depguard deny", theCensus(), theList, census.Lint{Forbid: theLint().Forbid},
			"no depguard rule on every file denies syscall"},
		{"a package ban whose deny holds some files only", theCensus(), theList, census.Lint{Forbid: theLint().Forbid, Deny: []census.Deny{{Pkg: "syscall", Desc: "arch.ports.24: no", Files: []string{"**/a/**"}}}},
			"no depguard rule on every file denies syscall"},
		{"a census that read nothing", &census.Census{}, theList, theLint(),
			census.ErrNoIdent.Error()},
		{"no census", nil, theList, theLint(),
			census.ErrNoIdent.Error()},
	} {
		ds, bad := census.ParseDecisions(tc.list)
		if len(bad) != 0 {
			t.Fatalf("%s: the list does not read: %v", tc.name, bad)
		}
		got := census.Hold(tc.c, ds, tc.lint)
		if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.want) }) {
			t.Errorf("%s: want a problem that says %q, got %v", tc.name, tc.want, got)
		}
	}
}

// rootedList decides what differs by its own declarations in goroot, and
// allows the two roots of internal/q and p.helper, which the rest reaches.
const rootedList = `internal/q.PerSys allow a reason
internal/q.Sep allow a reason
p.Doc allow a reason
p.Only later #1750 L14
p.Sep later #1750 L11
p.Sys allow a reason
p.helper allow a reason
`

func TestAnAllowedRootSettlesWhatReachesItAlone(t *testing.T) {
	t.Parallel()
	c, err := census.Take(t.Context(), fakeGo(dist, listing()), goroot())
	if err != nil {
		t.Fatal(err)
	}
	ds, bad := census.ParseDecisions(rootedList)
	if len(bad) != 0 {
		t.Fatalf("the list does not read: %v", bad)
	}
	if got := census.Hold(c, ds, census.Lint{}); len(got) != 0 {
		t.Errorf("an allow on each root settles what reaches it alone, yet the list is refused: %v", got)
	}
	without := strings.Replace(rootedList, "p.helper allow a reason\n", "", 1)
	ds, _ = census.ParseDecisions(without)
	got := census.Hold(c, ds, census.Lint{})
	for _, name := range []string{"p.Helped", "p.T.Exported", "p.TwoHop", "p.ViaMethod"} {
		if !slices.ContainsFunc(got, func(p string) bool { return strings.HasPrefix(p, name+" differs by system") }) {
			t.Errorf("with p.helper undecided, %s reaches it and wants a line; the problems are %v", name, got)
		}
	}
	if slices.ContainsFunc(got, func(p string) bool { return strings.HasPrefix(p, "p.Inner ") }) {
		t.Errorf("p.Inner reaches only internal/q.PerSys, which is decided, yet wants a line: %v", got)
	}
	ds, _ = census.ParseDecisions("internal/q.Hidden allow a reason\n" + rootedList)
	if got := census.Hold(c, ds, census.Lint{}); !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, "internal/q.Hidden names nothing the census found") }) {
		t.Errorf("a root line on a name that differs in nothing is held: %v", got)
	}
	ds, _ = census.ParseDecisions("crypto/internal/entropy/v1.0.0.Seed allow a reason\n")
	if len(ds) != 1 || ds[0].Path() != "crypto/internal/entropy/v1.0.0" {
		t.Errorf("a versioned element read as %v: its dots that a digit follows are the path's", ds)
	}
}

// TestABanSettlesNoWrapperButAnAllowedRootDoes plants the two sides of
// the rule for a root. A ban says the difference is wrong,
// so a name that reaches a banned one is banned too, or allowed with its own
// reason, and with no line it is red. An allow says the difference is fine,
// so the same wrapper, reaching the banned name only through an allowed
// root, is green.
func TestABanSettlesNoWrapperButAnAllowedRootDoes(t *testing.T) {
	t.Parallel()
	c, err := census.Take(t.Context(), fakeGo(dist, listing()), goroot())
	if err != nil {
		t.Fatal(err)
	}
	banned := strings.Replace(rootedList, "p.Sys allow a reason\n", "p.Sys ban go.errors.4\n", 1)
	lint := census.Lint{Forbid: []census.Forbid{{Pattern: `^p\.Sys$`, Msg: "go.errors.4: a stand-in row"}}}
	wraps := func(p string) bool { return strings.HasPrefix(p, "p.Wraps differs by system") }
	for _, tc := range []struct {
		name, list string
		red        bool
	}{
		{"a wrapper of a banned name with no line", banned, true},
		{"the same wrapper through an allowed root", banned + "p.settle allow a reason\n", false},
	} {
		ds, bad := census.ParseDecisions(tc.list)
		if len(bad) != 0 {
			t.Fatalf("%s: the list does not read: %v", tc.name, bad)
		}
		got := census.Hold(c, ds, lint)
		if slices.ContainsFunc(got, wraps) != tc.red {
			t.Errorf("%s: want p.Wraps red %v, got the problems %v", tc.name, tc.red, got)
		}
		if others := slices.DeleteFunc(slices.Clone(got), wraps); len(others) != 0 {
			t.Errorf("%s: want no problem but p.Wraps's, got %v", tc.name, others)
		}
	}
}

func TestAListThatDoesNotReadIsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, list, want string }{
		{"a line with no rest", "os.Exit ban\n", "is not <name> <kind> <rest>"},
		{"an unknown kind", "os.Exit refuse go.errors.4\n", `has the kind "refuse"`},
		{"a ban that names no rule id", "os.Exit ban only main exits\n", "a ban names one rule id"},
		{"a later line with no row", "os.Exit later soon\n", "a later line names the issue and the row"},
		{"an allowed package", "syscall.* allow it is fine\n", "a whole package is banned or later, never allowed"},
		{"a name with no identifier", "os ban go.errors.4\n", "is not <import path>.<Name>"},
		{"a name of no shape", "os.Exit-now ban go.errors.4\n", "is not <import path>.<Name>"},
		{"a banned root", "os.exit ban go.errors.4\n", "is a root, which only allow decides"},
		{"a banned root of internal", "internal/poll.FD ban go.errors.4\n", "is a root, which only allow decides"},
		{"a later root", "net..listenUnix later #1750 L14\n", "is a root, which only allow decides"},
		{"a package of internal", "internal/poll.* later #1750 L1\n", "names a package no program outside std may import"},
		{"a name decided twice", "os.Exit ban go.errors.4\nos.Exit allow a reason\n", "os.Exit is decided twice (line 1 too)"},
		{"a line out of order", "os/exec.Cmd ban cli.process.1\nos.Exit ban go.errors.4\n", "os.Exit comes before os/exec.Cmd (line 1) in byte order"},
	} {
		_, bad := census.ParseDecisions(tc.list)
		if !slices.ContainsFunc(bad, func(p string) bool { return strings.Contains(p, tc.want) }) {
			t.Errorf("%s: want a problem that says %q, got %v", tc.name, tc.want, bad)
		}
	}
}

func TestTheLaterLinesOnlyShrink(t *testing.T) {
	t.Parallel()
	read := func(s string) []census.Decision {
		ds, bad := census.ParseDecisions(s)
		if len(bad) != 0 {
			t.Fatalf("%q does not read: %v", s, bad)
		}
		return ds
	}
	const a, b = "os.Chmod later #1750 L6\n", "os.Chown later #1750 L6\n"
	if got := census.LaterGrown(read(a), read(a+b)); len(got) != 0 {
		t.Errorf("a later line that went is refused: %v", got)
	}
	if got := census.LaterGrown(read("os.Chmod ban cli.process.1\n"+b), read(a+b)); len(got) != 0 {
		t.Errorf("a later line that became a ban is refused: %v", got)
	}
	for _, tc := range []struct{ name, head, base string }{
		{"an added later line", a + b, a},
		{"an allowed identifier made later", a + b, a + "os.Chown allow a reason\n"},
		{"a later line on a base with none", a, "os.Chmod allow a reason\n"},
	} {
		got := census.LaterGrown(read(tc.head), read(tc.base))
		if len(got) != 1 || !strings.Contains(got[0], "was not later at the merge base") {
			t.Errorf("%s: want one problem that says it was not later at the merge base, got %v", tc.name, got)
		}
	}
}

// FuzzFirstClass holds the dist list's reader to what the census leans on: it
// never panics, and a list it reads gives ports whose every GOARCH has a port
// for every GOOS among them, and systems sorted, each once, that hold every
// port's GOOS.
func FuzzFirstClass(f *testing.F) {
	for _, seed := range []string{"", "[", "[]", "null", "{}", dist,
		`[{"GOOS":"a","GOARCH":"x","FirstClass":true}]`,
		`[{"GOOS":"","GOARCH":"x","FirstClass":true}]`,
		`[{"GOOS":"a","GOARCH":"x","FirstClass":false}]`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		ports, systems, err := census.FirstClass(data)
		if err != nil {
			return
		}
		if len(ports) == 0 {
			t.Fatal("a list that reads gave no port and no error")
		}
		if !slices.IsSorted(systems) || len(slices.Compact(slices.Clone(systems))) != len(systems) {
			t.Fatalf("systems %v are not sorted, each once", systems)
		}
		goos, on := map[string]bool{}, map[string]map[string]bool{}
		for _, p := range ports {
			if _, ok := slices.BinarySearch(systems, p.GOOS); !ok {
				t.Fatalf("port %s has a GOOS that systems %v lack", p, systems)
			}
			goos[p.GOOS] = true
			if on[p.GOARCH] == nil {
				on[p.GOARCH] = map[string]bool{}
			}
			on[p.GOARCH][p.GOOS] = true
		}
		for arch, has := range on {
			if len(has) != len(goos) {
				t.Fatalf("GOARCH %s has %d of the %d systems among the ports %v", arch, len(has), len(goos), ports)
			}
		}
	})
}
