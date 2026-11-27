package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheFormatMemberFailsClosedOnAWalkThatReadNothing holds gragas#1228: gofmt -l prints
// only what is misformatted, so a clean tree and a walk of nothing print the same; the member
// therefore requires gofmt to list a committed canary and is red without it, on every OS.
func TestTheFormatMemberFailsClosedOnAWalkThatReadNothing(t *testing.T) {
	canary, err := os.ReadFile(filepath.Join("..", "testdata", "gofmt-canary.go"))
	if err != nil {
		t.Fatal(err)
	}
	const bad = "package p\n\nfunc  F( ){ }\n"
	plant := func(t *testing.T, files map[string]string) string {
		t.Helper()
		dir := t.TempDir()
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	// The member names the canary by its exact path, so each plant says where its own is.
	run := func(dir string) (string, bool) {
		return runTask(t, "format", "GOFMT_DIRS="+filepath.ToSlash(dir), "GOFMT_CANARY="+filepath.ToSlash(filepath.Join(dir, "gofmt-canary.go")))
	}
	if out, ok := run(plant(t, map[string]string{"gofmt-canary.go": string(canary)})); !ok {
		t.Errorf("the canary alone: want a pass\n%s", out)
	}
	if out, ok := run(plant(t, map[string]string{"gofmt-canary.go": string(canary), "bad.go": bad})); ok || !strings.Contains(out, "bad.go") {
		t.Errorf("a misformatted file beside the canary: ok %v, want red naming bad.go\n%s", ok, out)
	}
	if out, ok := run(plant(t, map[string]string{"ok.go": "package p\n"})); ok || !strings.Contains(out, "did not list its canary") {
		t.Errorf("a folder without the canary: ok %v, want red saying the canary was not listed\n%s", ok, out)
	}
	if out, ok := run(plant(t, nil)); ok || !strings.Contains(out, "did not list its canary") {
		t.Errorf("an empty folder: ok %v, want red\n%s", ok, out)
	}
	if out, ok := run(plant(t, map[string]string{"gofmt-canary.go": string(canary), "broken.go": "package p\nfunc ("})); ok {
		t.Errorf("a file gofmt cannot parse: want red\n%s", out)
	}
	// One listed path is the canary: a second line that reads as its name is red, as a file
	// named with a \ that folds to the canary's name would be (gragas#1849). A file of that
	// name cannot be made on a system whose separator is \, so the plant lists the canary's
	// folder twice, and gofmt names the canary twice.
	twice := filepath.ToSlash(plant(t, map[string]string{"gofmt-canary.go": string(canary)}))
	if out, ok := runTask(t, "format", "GOFMT_DIRS="+twice+" "+twice, "GOFMT_CANARY="+twice+"/gofmt-canary.go"); ok || !strings.Contains(out, "gofmt-canary.go") {
		t.Errorf("two listed paths that read as the canary's name: ok %v, want red naming the second\n%s", ok, out)
	}
	// A misformatted file of the canary's name in another folder is not the canary.
	dir := plant(t, map[string]string{"gofmt-canary.go": string(canary)})
	if err := os.Mkdir(filepath.Join(dir, "zz"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zz", "gofmt-canary.go"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, ok := run(dir); ok || !strings.Contains(out, "zz") {
		t.Errorf("a misformatted file named like the canary in another folder: ok %v, want red naming it\n%s", ok, out)
	}
}
