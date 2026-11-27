package internal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Prettier preset is the format a project holds its files to, so the
// pinned Prettier is run over fixtures with it (gragas#1007): a preset whose
// values the tool does not hold, or a project config that does not import it,
// is a broken base. Prettier is a node tool, so it is run by the pinned node
// that task tools places, from a copy of its folder in a root of the test's
// own, and held to the version tools.yaml says. Every run has the same argv:
// the files and the config change, never the command.

const prettierPreset = "presets/prettier/base.json"

// prettierRoot is a new root holding a copy of the pinned Prettier, with the
// pinned node first on PATH and both held to tools.yaml's versions.
func prettierRoot(t *testing.T) string {
	t.Helper()
	node, nodeSays := pinnedProgram(t, "node")
	saysItsPin(t, node, nodeSays, exec.CommandContext(t.Context(), "node", "--version"))
	says, _, err := toolPin(lintDoc(t, "tools.yaml"), "prettier")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	copyTree(t, "tools/.node/node_modules/prettier", dir)
	cmd := exec.CommandContext(t.Context(), "node", "tools/.node/node_modules/prettier/bin/prettier.cjs", "--version")
	out, err := runIn(t, node, dir, cmd)
	if err != nil || !strings.Contains(out, says) {
		t.Fatalf("the copy of Prettier does not say tools.yaml's %q (%v):\n%s", says, err, out)
	}
	return dir
}

// prettierLists runs the check of the member over one file of the root, x.ts
// at its top or explorer/x.ts when inExplorer, with the config files the root
// holds, and reports whether the file is one Prettier would change.
func prettierLists(t *testing.T, dir string, inExplorer bool) (string, bool) {
	t.Helper()
	node, _ := pinnedProgram(t, "node")
	var cmd *exec.Cmd
	if inExplorer {
		cmd = exec.CommandContext(t.Context(), "node", "tools/.node/node_modules/prettier/bin/prettier.cjs", "--no-editorconfig", "--list-different", "explorer/x.ts")
	} else {
		cmd = exec.CommandContext(t.Context(), "node", "tools/.node/node_modules/prettier/bin/prettier.cjs", "--no-editorconfig", "--list-different", "x.ts")
	}
	out, err := runIn(t, node, dir, cmd)
	if _, ok := errors.AsType[*exec.ExitError](err); err != nil && !ok {
		t.Fatalf("prettier did not run: %v\n%s", err, out)
	}
	return out, err == nil
}

func put1(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func presetText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(prettierPreset)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const prettierGood = "const a = \"x\";\nconst total = add(\n  veryLongArgumentNumberOne,\n  veryLongArgumentNumberTwo,\n  veryLongArgumentNumberThree,\n);\n"

// TestThePrettierPresetHoldsEachOfItsKeysUnderPrettier: a file formatted to the
// preset passes, and for each key a file that differs in that key alone is
// listed, so each value is one Prettier reads and holds. The control is the
// semicolon-free plant under the preset with semi turned off, which passes: the
// plant is a file the key decides.
func TestThePrettierPresetHoldsEachOfItsKeysUnderPrettier(t *testing.T) {
	dir := prettierRoot(t)
	put1(t, dir, ".prettierrc.json", presetText(t))
	for name, body := range map[string]string{
		"the formatted file": prettierGood,
		"arrow parens kept":  "const f = (x) => x;\n",
	} {
		put1(t, dir, "x.ts", body)
		if out, ok := prettierLists(t, dir, false); !ok {
			t.Errorf("%s is listed under the preset:\n%s", name, out)
		}
	}
	plants := map[string]string{
		"semi":     strings.ReplaceAll(prettierGood, ";", ""),
		"quotes":   strings.Replace(prettierGood, "\"x\"", "'x'", 1),
		"trailing": strings.Replace(prettierGood, "Three,\n", "Three\n", 1),
		"tabs":     strings.ReplaceAll(prettierGood, "  very", "\tvery"),
		"crlf":     strings.ReplaceAll(prettierGood, "\n", "\r\n"),
		"width":    "const a = add(veryLongArgumentNumberOne, veryLongArgumentNumberTwo, veryLongArgumentNumberThree);\n",
		"arrow":    "const f = x => x;\n",
	}
	for key, body := range plants {
		put1(t, dir, "x.ts", body)
		if out, ok := prettierLists(t, dir, false); ok || !strings.Contains(out, "x.ts") {
			t.Errorf("the %s plant: ok %v, want it listed (a key of the preset is not held):\n%s", key, ok, out)
		}
	}
	// The control: semi turned off takes the semicolon-free plant.
	put1(t, dir, ".prettierrc.json", strings.Replace(presetText(t), "\"semi\": true", "\"semi\": false", 1))
	put1(t, dir, "x.ts", plants["semi"])
	if out, ok := prettierLists(t, dir, false); !ok {
		t.Errorf("semi off does not take the semicolon-free file, so its plant tests nothing:\n%s", out)
	}
}

// TestTheExplorerConfigImportsThePreset: explorer/.prettierrc.yaml is the
// path of the preset and nothing else, and in a copy of the two files a change
// of the preset reaches a file of the explorer, so the config reads the shared
// file rather than holding values of its own.
func TestTheExplorerConfigImportsThePreset(t *testing.T) {
	own, err := os.ReadFile(filepath.Join(root, "explorer", ".prettierrc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(own) != "\"../presets/prettier/base.json\"\n" {
		t.Fatalf("explorer/.prettierrc.yaml is %q, want the path of the preset alone", own)
	}
	dir := prettierRoot(t)
	put1(t, dir, "explorer/.prettierrc.yaml", string(own))
	put1(t, dir, prettierPreset, strings.Replace(presetText(t), "\"semi\": true", "\"semi\": false", 1))
	put1(t, dir, "explorer/x.ts", "const a = 1\n")
	if out, ok := prettierLists(t, dir, true); !ok {
		t.Errorf("the preset with semi off is not read through the explorer config:\n%s", out)
	}
	put1(t, dir, "explorer/x.ts", "const a = 1;\n")
	if out, ok := prettierLists(t, dir, true); ok || !strings.Contains(out, "x.ts") {
		t.Errorf("a semicolon under semi off: ok %v, want it listed:\n%s", ok, out)
	}
}
