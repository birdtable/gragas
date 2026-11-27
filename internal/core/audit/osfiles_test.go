package audit_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/core/audit"
)

// TestAFileNamesAnOperatingSystemByItsNameOrBuildLineAndNothingElse plants the
// file names and build lines the check tells apart (arch.ports.24), each name
// form go build reads with and without _test.
// Planted failure: the check misses a GOOS, a _GOOS_GOARCH name or a term,
// passes a build line a block comment holding "package" comes before, takes a
// GOARCH alone for a system, or reads a word of the body, a feature tag or a
// build line after the package clause.
func TestAFileNamesAnOperatingSystemByItsNameOrBuildLineAndNothingElse(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, body string
		want       bool
	}{
		{"x_windows.go", "package x\n", true},
		{"x_linux_test.go", "package x\n", true},
		{"x_freebsd.go", "package x\n", true},
		{"x_openbsd.go", "package x\n", true},
		{"x_openbsd_test.go", "package x\n", true},
		{"x_wasip1.go", "package x\n", true},
		{"x_other.go", "package x\n", true},
		{"x_unix_test.go", "package x\n", true},
		{"x_linux_amd64.go", "package x\n", true},
		{"x_linux_amd64_test.go", "package x\n", true},
		{"x_windows_arm64_test.go", "package x\n", true},
		{"x_js_wasm.go", "package x\n", true},
		{"x_linux.pb.go", "package x\n", true},
		{"zz_windows.gen_test.go", "package x\n", true},
		{"x.linux.go", "package x\n", false},
		{"x_wasm.go", "package x\n", true},
		{"x_amd64.go", "package x\n", false},
		{"x_arm64_test.go", "package x\n", false},
		{"x_amd64_linux.go", "package x\n", true},
		{"x.go", "//go:build !windows\n\npackage x\n", true},
		{"x.go", "//go:build cgo && (linux || darwin)\n\npackage x\n", true},
		{"x.go", "// +build openbsd\n\npackage x\n", true},
		{"x.go", "/*\npackage doc\n*/\n//go:build linux\n\npackage x\n", true},
		{"x.go", "// Package x says package.\n//go:build linux\n\npackage x\n", true},
		{"x.go", "//go:build integration\n\npackage x\n", false},
		{"x.go", "package x\n\n// on linux it is quick\n", false},
		{"windows.go", "package x\n", false},
		{"linux_test.go", "package x\n", false},
		{"x.go", "package x\n\n//go:build linux\n", false},
	} {
		got, err := audit.NamesAnOS(c.name, []byte(c.body))
		if err != nil || got != c.want {
			t.Errorf("NamesAnOS(%q, %q) = %v, %v, want %v", c.name, c.body, got, err, c.want)
		}
	}
}

// TestAFileWithNoPackageClauseIsAFindingNotAPass: a file go/parser cannot read
// as far as its package clause, or whose //go:build line go build refuses, is
// an error of NamesAnOS and a finding of the check that names its path; the
// walk goes on, so the other files are still found, and a row waives it.
// Planted failure: such a file names no system, or it ends the walk as a
// not-checked that leaves every other file unread.
func TestAFileWithNoPackageClauseIsAFindingNotAPass(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "//go:build linux\n", "/* package x\n", "//go:build linux &&\n\npackage x\n"} {
		if got, err := audit.NamesAnOS("x.go", []byte(body)); err == nil {
			t.Errorf("NamesAnOS(%q) = %v and no error, want an error", body, got)
		}
	}
	repo := osRepo(osHostConfig)
	repo["internal/tools/junk.go"] = &fstest.MapFile{Data: []byte("//go:build ignore\n\nnot go at all\n")}
	r := audit.Check(t.Context(), osInput(repo, nil))
	if len(r.NotChecked) != 0 || len(r.Findings) != 1 {
		t.Fatalf("a file with no package clause: %+v, want one finding and no not-checked", r)
	}
	for _, want := range []string{"internal/tools/junk.go does not parse", "internal/report/size_linux.go"} {
		if !strings.Contains(r.Findings[0].Missing, want) {
			t.Errorf("the finding does not say %q: %s", want, r.Findings[0].Missing)
		}
	}
	r = audit.Check(t.Context(), osInput(repo, map[string]bool{
		"internal/tools/junk.go": true, "internal/report/size_linux.go": true, "internal/journeys/pty_test.go": true,
	}))
	if len(r.Findings) != 0 {
		t.Errorf("a row waives a file that does not parse, got %+v", r.Findings)
	}
}

// osRepo is a repository whose golangci config spares internal/host in its
// os-host row, and one file alone, with an operating-system file inside the
// layer, one outside it, one the row spares alone, one in testdata, and two
// go build never reads (a name that starts with _ or a dot, the first broken).
func osRepo(config string) fstest.MapFS {
	repo := fstest.MapFS{
		"internal/host/link/link_windows.go":   {Data: []byte("package link\n")},
		"internal/report/size_linux.go":        {Data: []byte("package report\n")},
		"internal/report/size.go":              {Data: []byte("package report\n")},
		"internal/journeys/pty_test.go":        {Data: []byte("//go:build linux || darwin\n\npackage journeys\n")},
		"internal/report/testdata/x_darwin.go": {Data: []byte("package x\n")},
		"internal/report/_scratch_linux.go":    {Data: []byte("/* not built\n")},
		".hidden_windows.go":                   {Data: []byte("package x\n")},
	}
	if config != "" {
		repo[".golangci.yml"] = &fstest.MapFile{Data: []byte(config)}
	}
	return repo
}

const osHostConfig = "version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n        os-host:\n" +
	"          files: [\"$all\", \"!**/internal/host/**\", \"!**/internal/journeys/pty_test.go\"]\n"

func osInput(repo fstest.MapFS, exempt map[string]bool) audit.Input {
	return audit.Input{
		Repo: repo,
		Sources: []audit.Source{{
			Tool: "gragas", Files: []string{".golangci.yml"}, Kind: audit.KindOSFiles,
			Host: "linters.settings.depguard.rules.os-host.files",
		}},
		Bindings: []audit.Binding{{Rule: "arch.ports.24", Tool: "gragas", Key: audit.KindOSFiles}},
		// A row for the rule waives no file it does not name.
		Exempt: func(string) (string, bool) { return "2099-01-01", true },
		ExemptAt: func(_, file string) (string, bool) {
			return "2099-01-01", exempt[file]
		},
	}
}

// TestTheOsFilesOutsideTheHostLayerAreAFindingAndARowWaivesItsOwn: the files
// outside the folders the os-host row spares are one finding that names
// them; one the row spares alone is outside the layer; a row of the
// exemptions file waives the file it names and no other; inside the layer
// and in testdata nothing is read; and with no config every such file is a
// finding.
// Planted failure: the check reads the layer from a file the row spares, a
// rule-wide row waives a file it does not name, or testdata or a file go build skips is read.
func TestTheOsFilesOutsideTheHostLayerAreAFindingAndARowWaivesItsOwn(t *testing.T) {
	t.Parallel()
	res := audit.Check(t.Context(), osInput(osRepo(osHostConfig), nil))
	if len(res.Findings) != 1 || len(res.Exempt) != 0 {
		t.Fatalf("want one finding and no exempt, got %+v", res)
	}
	got := res.Findings[0].String()
	for _, want := range []string{"arch.ports.24", "internal/journeys/pty_test.go, internal/report/size_linux.go", "the host layer is internal/host"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not say %q: %s", want, got)
		}
	}
	for _, not := range []string{"link_windows.go", "x_darwin.go", "size.go,"} {
		if strings.Contains(got, not) {
			t.Errorf("the finding names %q: %s", not, got)
		}
	}

	res = audit.Check(t.Context(), osInput(osRepo(osHostConfig), map[string]bool{"internal/journeys/pty_test.go": true}))
	if len(res.Findings) != 1 || strings.Contains(res.Findings[0].Missing, "pty_test.go") || !strings.Contains(res.Findings[0].Missing, "size_linux.go") {
		t.Errorf("a row waives its own file and no other, got %+v", res.Findings)
	}

	res = audit.Check(t.Context(), osInput(osRepo(osHostConfig), map[string]bool{"internal/journeys/pty_test.go": true, "internal/report/size_linux.go": true}))
	if len(res.Findings) != 0 {
		t.Errorf("every file has its row, got %+v", res.Findings)
	}

	res = audit.Check(t.Context(), osInput(osRepo(""), nil))
	if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Missing, "link_windows.go") || !strings.Contains(res.Findings[0].Missing, "names no host layer") {
		t.Errorf("with no config there is no host layer, got %+v", res.Findings)
	}
}
