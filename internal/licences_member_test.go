package internal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/digest"
)

// TestTheLicencesMemberIsRedOnAnEmptyListAndOnAListThatFailsTheTree holds gragas#1228: an empty
// allowlist made go-licenses fall back to the weaker check and exit 0, and the pinned binary
// (built -trimpath) took every package for the standard library and checked none. The member
// reads the list with its exit status, sets GOROOT, and is red when the report lists no known
// dependency, so each plant is red and the control, the committed list, is green.
func TestTheLicencesMemberIsRedOnAnEmptyListAndOnAListThatFailsTheTree(t *testing.T) {
	list := func(body string) string {
		p := filepath.Join(t.TempDir(), "licences.txt")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return "LICENCES_FILE=" + filepath.ToSlash(p)
	}
	if out, ok := runTask(t, "licences", list("# nothing\n")); ok || !strings.Contains(out, "names no licence") {
		t.Errorf("an empty list: ok %v, want red saying it names no licence\n%s", ok, out)
	}
	if out, ok := runTask(t, "licences", list("MIT\n")); ok || !strings.Contains(out, "Not allowed") {
		t.Errorf("a list of MIT alone: ok %v, want red naming libraries not allowed\n%s", ok, out)
	}
	if out, ok := runTask(t, "licences"); !ok {
		t.Errorf("the committed list: want a pass\n%s", out)
	}
}

// sourceRows reads gate/licence-sources.txt as the licences member's command does: six fields.
func sourceRows(t *testing.T, path string) [][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	for l := range strings.SplitSeq(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			rows = append(rows, strings.Fields(l))
		}
	}
	return rows
}

// TestEachModuleTheLicencesMemberPassesOverIsSourcedAndStillHasNoLicenceFile holds gragas#1265:
// a row stays only while its module's zip in the module cache holds no LICENSE (the row goes when
// the tag carries one), at the version go.mod pins; its id is on the allowlist; and the licence
// text it names, committed beside the file as gate/<name>-LICENSE, is the one its sha256 says.
// The file may hold no row: every module the build takes carries its LICENSE since #909.
func TestEachModuleTheLicencesMemberPassesOverIsSourcedAndStillHasNoLicenceFile(t *testing.T) {
	rows := sourceRows(t, filepath.Join(root, "gate", "licence-sources.txt"))
	copies, err := filepath.Glob(filepath.Join(root, "gate", "*-LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	sums := map[string]string{}
	for _, c := range copies {
		text, err := os.ReadFile(c)
		if err != nil {
			t.Fatal(err)
		}
		sums[digest.Hex(text)] = c
	}
	named := map[string]bool{}
	for _, r := range rows {
		if len(r) == 6 {
			named[r[4]] = true
		}
	}
	for sum, c := range sums {
		if !named[sum] {
			t.Errorf("%s is a licence copy no row of gate/licence-sources.txt names by its sha256: remove it", filepath.ToSlash(c))
		}
	}
	allowed, err := os.ReadFile(filepath.Join(root, "gate", "licences.txt"))
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(string(allowed))
	modData, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goMod := string(modData)
	for _, r := range rows {
		if len(r) != 6 || strings.ContainsAny(r[0], "*?[") {
			t.Errorf("a row of %d fields or with a wildcard: %v", len(r), r)
			continue
		}
		module, version, id, hash := r[0], r[1], r[3], r[4]
		if _, ok := sums[hash]; !ok {
			t.Errorf("%s: its sha256 %s is the one of no gate/*-LICENSE copy", module, hash)
		}
		if !slices.Contains(ids, id) {
			t.Errorf("%s: %s is not on gate/licences.txt", module, id)
		}
		if !strings.Contains(goMod, "\t"+module+" "+version+"\n") {
			t.Errorf("go.mod does not pin %s at %s, the row's version", module, version)
		}
		out, err := inRoot(nil, "go", "list", "-m", "-f", "{{.Dir}}", module).Output()
		if err != nil {
			t.Errorf("%s: go list -m: %v", module, err)
			continue
		}
		dir := strings.TrimSpace(string(out))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Errorf("%s: %v", module, err)
			continue
		}
		for _, e := range entries {
			if n := strings.ToUpper(e.Name()); strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") || strings.HasPrefix(n, "COPYING") {
				t.Errorf("%s %s now holds %s: drop its row", module, version, e.Name())
			}
		}
	}
}

// plantedModule is the module of the plant's tree that carries no LICENSE.
const plantedModule = "example.org/nolicence"

// plantTree writes a module tree for the licences member (LICENCES_TREE): a program under this
// repository's LICENSE that takes go.yaml.in/yaml/v3 at this go.mod's version, so the report lists
// its known dependency, and plantedModule, a local module with no LICENSE file.
func plantTree(t *testing.T) string {
	t.Helper()
	modData, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	sumData, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	licence, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	const yaml = "go.yaml.in/yaml/v3"
	var version, sums string
	for l := range strings.SplitSeq(string(modData), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && f[0] == yaml {
			version = f[1]
		}
	}
	for l := range strings.SplitSeq(string(sumData), "\n") {
		if version != "" && strings.HasPrefix(l, yaml+" "+version) {
			sums += l + "\n"
		}
	}
	if version == "" || sums == "" {
		t.Fatalf("go.mod and go.sum do not pin %s", yaml)
	}
	dir := t.TempDir()
	files := map[string]string{
		"nolicence/go.mod": "module " + plantedModule + "\n\ngo 1.27\n",
		"nolicence/n.go":   "package nolicence\n\nfunc N() int { return 1 }\n",
		"app/LICENSE":      string(licence),
		"app/go.mod": "module example.org/app\n\ngo 1.27.1\n\nrequire (\n\t" + plantedModule + " v0.0.0\n\t" + yaml + " " + version + "\n)\n\n" +
			"replace " + plantedModule + " => ../nolicence\n",
		"app/go.sum":  sums,
		"app/main.go": "package main\n\nimport (\n\t\"" + plantedModule + "\"\n\t\"" + yaml + "\"\n)\n\nvar _ = yaml.Marshal\n\nfunc main() { _ = nolicence.N() }\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "app")
}

// TestAModuleWithNoLicenceAndNoRowIsRed plants a module with no LICENSE and no row: the member
// over the plant's tree (plantTree) is red and names it, and the same member over this repository
// is green. Where the sources file has rows, it also plants a missing row: one row is left out,
// and the member is red and names that module.
func TestAModuleWithNoLicenceAndNoRowIsRed(t *testing.T) {
	if out, ok := runTask(t, "licences", "LICENCES_TREE="+filepath.ToSlash(plantTree(t))); ok || !strings.Contains(out, "'"+plantedModule+"'") {
		t.Errorf("a module with no LICENSE and no row: ok %v, want red naming %s\n%s", ok, plantedModule, out)
	}
	if out, ok := runTask(t, "licences"); !ok {
		t.Errorf("this repository's tree and sources file: want a pass\n%s", out)
	}
	rows := sourceRows(t, filepath.Join(root, "gate", "licence-sources.txt"))
	if len(rows) == 0 {
		return
	}
	var kept []string
	for _, r := range rows[1:] {
		kept = append(kept, strings.Join(r, " "))
	}
	p := filepath.Join(t.TempDir(), "sources.txt")
	if err := os.WriteFile(p, []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, ok := runTask(t, "licences", "LICENCES_SOURCES="+filepath.ToSlash(p))
	if ok || !strings.Contains(out, "'"+rows[0][0]+"'") {
		t.Errorf("a module with no row: ok %v, want red naming %s\n%s", ok, rows[0][0], out)
	}
}

// underARowsPath reads a listing of "<package> <module>" lines and returns how many packages start with a
// row's path and each that is of another module: --ignore is a prefix of the import path, so a row for
// github.com/x/m/run would also pass over x/m/runtime, or a package of another module below it.
func underARowsPath(rows [][]string, listing string) (int, []string) {
	var bad []string
	seen := 0
	for l := range strings.SplitSeq(strings.TrimSpace(listing), "\n") {
		pkg, mod, _ := strings.Cut(l, " ")
		for _, r := range rows {
			if !strings.HasPrefix(pkg, r[0]) {
				continue
			}
			seen++
			if mod != r[0] {
				bad = append(bad, pkg+" (module "+mod+") starts with the path of the row for "+r[0])
			}
		}
	}
	return seen, bad
}

// TestNoPackageButTheRowsModuleLiesUnderARowsPath holds the --ignore of the licences member over the
// build's real packages on every system the member checks, and plants a package of another module
// under a row's path. The rows are the sources file's and the allowlist's takes rows (gragas#1455):
// libcap's psx is in the Linux builds alone, so the host's listing would never read it.
func TestNoPackageButTheRowsModuleLiesUnderARowsPath(t *testing.T) {
	rows := sourceRows(t, filepath.Join(root, "gate", "licence-sources.txt"))
	allowed, err := os.ReadFile(filepath.Join(root, "gate", "licences.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range takesRowsOf(string(allowed)) {
		rows = append(rows, []string{r[0]})
	}
	seen := 0
	for _, sys := range []string{"linux/amd64", "linux/arm64", "darwin/arm64", "windows/amd64", "freebsd/amd64"} {
		goos, goarch, _ := strings.Cut(sys, "/")
		out, err := inRoot([]string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0"}, "go", "list", "-deps", "-test", "-f", "{{.ImportPath}} {{if .Module}}{{.Module.Path}}{{end}}", "./...").Output()
		if err != nil {
			t.Fatalf("go list -deps for %s: %v", sys, err)
		}
		n, bad := underARowsPath(rows, string(out))
		seen += n
		for _, b := range bad {
			t.Error(sys + ": " + b)
		}
	}
	if len(rows) != 0 && seen == 0 {
		t.Error("no package of any row's module was listed on any system: the check read nothing")
	}
	// The plant is on a row of its own, so it holds the prefix rule with the file empty too.
	row := "example.org/m/run"
	planted := [][]string{{row, "v0.1.0", "0", "Apache-2.0", "0", "#0"}}
	plant := row + "/sub " + row + "\n" + row + "time example.org/other\n" + row + "/x/y example.org/other\n"
	if n, got := underARowsPath(planted, plant); n != 3 || len(got) != 2 {
		t.Errorf("a planted sibling and a planted child of another module: %d under the path, %v; want 3 and two named", n, got)
	}
	// A takes row read from a planted list, and a module whose path starts with it.
	taken := takesRowsOf("BSD-3-Clause\nexample.org/libcap/psx takes BSD-3-Clause\n")
	if n, got := underARowsPath(taken, "example.org/libcap/psx example.org/libcap/psx\nexample.org/libcap/psxevil example.org/libcap/psxevil\n"); n != 2 || len(got) != 1 || !strings.Contains(got[0], "psxevil") {
		t.Errorf("a planted module under a takes row's path: %d under the path, %v; want 2 and psxevil named", n, got)
	}
}

// takesRowsOf reads the "<module> takes <id>" rows of an allowlist's text as the licences
// member's command does, as {module, id}.
func takesRowsOf(text string) [][]string {
	var rows [][]string
	for l := range strings.SplitSeq(text, "\n") {
		if f := strings.Fields(l); len(f) == 3 && f[1] == "takes" {
			rows = append(rows, []string{f[0], f[2]})
		}
	}
	return rows
}

// offersAsAChoice is whether a licence file's text declares id as one side of an SPDX OR: a takes
// row passes its module over, so the module must still offer the side the row takes. Only a flat
// OR list counts, each side one bare id, in one pair of brackets at most: in "(A OR B) AND C",
// taking A does not meet C, so no expression with AND, WITH or a nested bracket offers a choice.
func offersAsAChoice(text, id string) bool {
	for l := range strings.SplitSeq(text, "\n") {
		_, expr, ok := strings.Cut(l, "SPDX-License-Identifier:")
		if !ok {
			continue
		}
		expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(expr), "*/"))
		if inner, ok := strings.CutPrefix(expr, "("); ok {
			if expr, ok = strings.CutSuffix(inner, ")"); !ok {
				continue
			}
		}
		sides := strings.Split(expr, " OR ")
		for i := range sides {
			sides[i] = strings.TrimSpace(sides[i])
		}
		if len(sides) > 1 && !slices.ContainsFunc(sides, notABareID) && slices.Contains(sides, id) {
			return true
		}
	}
	return false
}

// notABareID is whether an SPDX side is anything but one licence id: letters, digits, '.', '-'
// and a closing '+'.
func notABareID(side string) bool {
	if side == "" {
		return true
	}
	for i, r := range side {
		bare := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+' && i == len(side)-1
		if !bare {
			return true
		}
	}
	return false
}

// TestEachModuleTheListTakesOneSideOfStillOffersThatSide holds gragas#1455: the takes row of
// gate/licences.txt passes its module over whole, so its licence file, at the version go.mod
// pins, must still offer the id taken as one side of an SPDX OR. A tag that drops the choice turns
// this red, and the row goes or changes. Plants: the id alone, an OR without the id, an id taken
// as a word of another id, no SPDX line, and an OR held by AND to another id, inside or outside
// its brackets, with an exception or nested (taking one side there
// does not meet the other id, so passing it would fail open).
func TestEachModuleTheListTakesOneSideOfStillOffersThatSide(t *testing.T) {
	allowed, err := os.ReadFile(filepath.Join(root, "gate", "licences.txt"))
	if err != nil {
		t.Fatal(err)
	}
	modData, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range takesRowsOf(string(allowed)) {
		module, id := r[0], r[1]
		if !strings.Contains(string(modData), "\t"+module+" ") {
			t.Errorf("%s: a takes row for a module go.mod does not require: drop the row", module)
			continue
		}
		out, err := inRoot(nil, "go", "list", "-m", "-f", "{{.Dir}}", module).Output()
		if err != nil {
			t.Errorf("%s: go list -m: %v", module, err)
			continue
		}
		dir := strings.TrimSpace(string(out))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Errorf("%s: %v", module, err)
			continue
		}
		offered := false
		for _, e := range entries {
			if n := strings.ToUpper(e.Name()); !strings.HasPrefix(n, "LICENSE") && !strings.HasPrefix(n, "LICENCE") && !strings.HasPrefix(n, "COPYING") {
				continue
			}
			text, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			offered = offered || offersAsAChoice(string(text), id)
		}
		if !offered {
			t.Errorf("%s: no licence file of it offers %s as one side of an SPDX OR, so its takes row may not pass it over", module, id)
		}
	}
	for claim, text := range map[string]string{
		"the id alone":                                   "/* SPDX-License-Identifier: BSD-3-Clause */\n",
		"a choice without the id":                        "SPDX-License-Identifier: MIT OR GPL-2.0-only\n",
		"a choice held to another id":                    "SPDX-License-Identifier: (BSD-3-Clause OR GPL-2.0-only) AND GPL-3.0-only\n",
		"a choice held to another, outside its brackets": "SPDX-License-Identifier: GPL-3.0-only AND (BSD-3-Clause OR GPL-2.0-only)\n",
		"a side with an exception":                       "SPDX-License-Identifier: BSD-3-Clause OR GPL-2.0-only WITH Linux-syscall-note AND MIT\n",
		"a choice held to another id, each bracketed":    "SPDX-License-Identifier: (BSD-3-Clause OR GPL-2.0-only) AND (GPL-3.0-only)\n",
		"a nested choice":                                "SPDX-License-Identifier: ((BSD-3-Clause OR MIT) OR GPL-2.0-only)\n",
		"the id as a word of another":                    "SPDX-License-Identifier: BSD-3-Clause-Clear OR GPL-2.0-only\n",
		"the id with no SPDX line":                       "BSD-3-Clause OR GPL-2.0-only\n",
	} {
		if offersAsAChoice(text, "BSD-3-Clause") {
			t.Errorf("a planted licence, %s: offers BSD-3-Clause as a choice, want not", claim)
		}
	}
	for claim, text := range map[string]string{
		"psx's own line":       "/* SPDX-License-Identifier: BSD-3-Clause OR GPL-2.0-only */\n",
		"the choice bracketed": "SPDX-License-Identifier: (GPL-2.0-only OR BSD-3-Clause)\n",
		"three sides":          "SPDX-License-Identifier: MIT OR BSD-3-Clause OR GPL-2.0-or-later\n",
	} {
		if !offersAsAChoice(text, "BSD-3-Clause") {
			t.Errorf("%s: want BSD-3-Clause offered as a choice", claim)
		}
	}
}
