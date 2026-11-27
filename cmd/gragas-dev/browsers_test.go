package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const pinFixture = `tools:
  - id: x
browsers:
  playwright: 1.65.0
  platforms:
    linux/amd64: {tree: browsers2:` + "0000000000000000000000000000000000000000000000000000000000000000" + `}
    darwin/arm64: {tree: browsers2:bad}
    plan9/arm64: {unmeasured: no session of that system has run the install}
    freebsd/amd64: {tree: browsers1:` + "1111111111111111111111111111111111111111111111111111111111111111" + `}

expected:
`

func TestTheBrowsersPinIsReadForItsPlatformAndNoOther(t *testing.T) {
	t.Parallel()
	got, err := readBrowsersPin([]byte(pinFixture), "linux/amd64")
	if err != nil || got != "browsers2:"+strings.Repeat("0", 64) {
		t.Fatalf("got %q, %v", got, err)
	}
	// an older format is read, for treepin.Check to refuse by name
	if got, err := readBrowsersPin([]byte(pinFixture), "freebsd/amd64"); err != nil || got != "browsers1:"+strings.Repeat("1", 64) {
		t.Errorf("a browsers1: pin: %q, %v, want it read", got, err)
	}
	if _, err := readBrowsersPin([]byte(pinFixture), "darwin/arm64"); err == nil {
		t.Error("a malformed digest was taken")
	}
	if _, err := readBrowsersPin([]byte(pinFixture), "plan9/arm64"); err == nil || !strings.Contains(err.Error(), "plan9/arm64 says its tree is not measured: no session of that system has run the install") {
		t.Errorf("an unmeasured row: %v, want a refusal naming the row and its reason", err)
	}
	if _, err := readBrowsersPin([]byte(pinFixture), "windows/amd64"); err == nil {
		t.Error("a platform with no pin was taken")
	}
}

func TestTheBrowsersFolderIsHeldToItsPinAndAPlantedFileIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, ".tools", "playwright")
	if err := os.MkdirAll(filepath.Join(folder, "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "x", "browser"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := run(context.Background(), []string{"browsers", "-C", dir, "--platform", "linux/amd64", "--print"}, &out, &errs); code != exitDone {
		t.Fatalf("print: %d %s", code, errs.String())
	}
	pin := strings.TrimSpace(out.String())
	yaml := "browsers:\n  platforms:\n    linux/amd64: {tree: " + pin + "}\n"
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"browsers", "-C", dir, "--platform", "linux/amd64"}
	out.Reset()
	errs.Reset()
	if code := run(context.Background(), args, &out, &errs); code != exitDone {
		t.Fatalf("the clean control is refused: %d %s", code, errs.String())
	}
	if err := os.WriteFile(filepath.Join(folder, "x", "planted"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs.Reset()
	if code := run(context.Background(), args, &out, &errs); code != exitFailed || !strings.Contains(errs.String(), pin) {
		t.Fatalf("a planted file passed, or the refusal does not name the pin: %d %s", code, errs.String())
	}
	// the pin of the older format, browsers1:, is refused by name (gragas#1451)
	old := "browsers1:" + strings.TrimPrefix(pin, "browsers2:")
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte("browsers:\n  platforms:\n    linux/amd64: {tree: "+old+"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs.Reset()
	if code := run(context.Background(), args, &out, &errs); code != exitFailed || !strings.Contains(errs.String(), "older format than browsers2:") {
		t.Fatalf("a browsers1: pin was not refused as an older format: %d %s", code, errs.String())
	}
}

// TestEverySystemToolsYamlListsHasABrowsersRow: each system a tool of tools.yaml is pinned for has
// a row under browsers.platforms, a digest or an unmeasured row that says why, so the step names
// the row and refuses, never reads "no pin" for a system the file knows (gragas#1828).
func TestEverySystemToolsYamlListsHasABrowsersRow(t *testing.T) {
	t.Parallel()
	yaml, err := os.ReadFile(filepath.Join("..", "..", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	systems := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^      ((?:darwin|linux|windows)/[a-z0-9]+):`).FindAllStringSubmatch(string(yaml), -1) {
		systems[m[1]] = true
	}
	if len(systems) < 5 {
		t.Fatalf("read %d systems from tools.yaml, want at least 5", len(systems))
	}
	for s := range systems {
		if _, err := readBrowsersPin(yaml, s); err == nil {
			continue
		} else if !strings.Contains(err.Error(), "says its tree is not measured") {
			t.Errorf("%s: %v, want a digest or an unmeasured row", s, err)
		}
	}
	// the plant: a system with no row at all is the "pins no browsers" refusal
	if _, err := readBrowsersPin(yaml, "linux/riscv64"); err == nil || !strings.Contains(err.Error(), "pins no browsers for linux/riscv64") {
		t.Errorf("a system with no row: %v", err)
	}
}

// TestTheToolsStepEndsGreenOnAnUnmeasuredSystemAndTheMembersStillRefuseIt (gragas#1984): with --unmeasured-ok, which
// task tools passes, a system whose row says unmeasured ends green on one line naming it and makes no folder; the same
// row without the flag, as a member that runs a browser calls it, is refused by name; a system with no row is red either
// way; a measured system is checked as before, flag or none. Planted failures: a flag that also forgives a missing row
// or a wrong digest, or a refusal that no longer names the row.
func TestTheToolsStepEndsGreenOnAnUnmeasuredSystemAndTheMembersStillRefuseIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yaml := "browsers:\n  platforms:\n    plan9/arm64: {unmeasured: no session of that system has run the install}\n    linux/amd64: {tree: browsers2:" + strings.Repeat("0", 64) + "}\n"
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	try := func(platform string, flag bool) (int, string, string) {
		args := []string{"browsers", "-C", dir, "--platform", platform}
		if flag {
			args = append(args, "--unmeasured-ok")
		}
		var out, errs bytes.Buffer
		return run(context.Background(), args, &out, &errs), out.String(), errs.String()
	}
	if code, out, errs := try("plan9/arm64", true); code != exitDone || strings.Count(out, "\n") != 1 || !strings.Contains(out, "plan9/arm64") || !strings.Contains(out, "not measured") || errs != "" {
		t.Errorf("tools step on an unmeasured system = %d %q %q, want green and one line naming it", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tools")); err == nil {
		t.Error("the unmeasured system's step made a folder")
	}
	if code, _, errs := try("plan9/arm64", false); code != exitFailed || !strings.Contains(errs, "browsers row for plan9/arm64 says its tree is not measured") {
		t.Errorf("a browser member on an unmeasured system = %d %q, want a refusal naming the row", code, errs)
	}
	for _, flag := range []bool{true, false} {
		if code, _, errs := try("linux/riscv64", flag); code != exitFailed || !strings.Contains(errs, "pins no browsers for linux/riscv64") {
			t.Errorf("a system with no row (flag %v) = %d %q, want a refusal", flag, code, errs)
		}
		if code, _, errs := try("linux/amd64", flag); code != exitFailed || !strings.Contains(errs, "treepin:") {
			t.Errorf("a measured system whose folder is missing (flag %v) = %d %q, want the check as before", flag, code, errs)
		}
	}
}
