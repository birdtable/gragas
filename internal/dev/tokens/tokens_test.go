package tokens_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/tokens"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// shipped is the document the repository carries, read from its place.
func shipped(t *testing.T) (tokens.Document, []byte) {
	t.Helper()
	b, err := os.ReadFile("../../../" + tokens.File)
	if err != nil {
		t.Fatal(err)
	}
	d, err := tokens.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return d, b
}

// replace is the document with the first match of the pattern replaced, and it
// fails when the pattern matches nothing, so a plant never plants nothing. The
// document is formatted by Prettier, so a pattern spans its lines with \s*.
func replace(t *testing.T, doc []byte, pattern, to string) []byte {
	t.Helper()
	re := regexp.MustCompile(pattern)
	loc := re.FindSubmatchIndex(doc)
	if loc == nil {
		t.Fatalf("the document holds nothing that matches %q to plant over", pattern)
	}
	out := append([]byte{}, doc[:loc[0]]...)
	out = re.Expand(out, []byte(to), doc, loc)
	return append(out, doc[loc[1]:]...)
}

// TestEveryPairHoldsItsFloorInBothModes: the shipped values pass, light and
// dark. Planted below: the guard TestAPairBelowItsFloorIsNamed.
func TestEveryPairHoldsItsFloorInBothModes(t *testing.T) {
	t.Parallel()
	d, _ := shipped(t)
	for _, l := range d.Low() {
		t.Error(l)
	}
}

// TestAPairBelowItsFloorIsNamed: a text colour lowered to a pale grey fails in
// light and a part lowered below 3:1 fails in dark, each named by role, ground
// and mode (web.tokens.8, web.accessibility.15).
func TestAPairBelowItsFloorIsNamed(t *testing.T) {
	t.Parallel()
	_, doc := shipped(t)
	for _, c := range []struct{ name, from, to, want string }{
		{"text in light", `("muted": \{\s*"light": ")#5a5850`, `${1}#c0c0c0`, "text.muted on page in light"},
		{"text in dark", `("violet": \{\s*"light": "#6d28d9",\s*"dark": ")#c4b5fd`, `${1}#222222`, "standing.violet on page in dark"},
		{"a part in dark", `("focus": \{\s*"light": "#1e3a8a",\s*"dark": ")#a8bbf5`, `${1}#2a2a2a`, "parts.focus on page in dark"},
		{"a part in light", `("control-border": \{\s*"light": ")#7c7a72`, `${1}#e0e0e0`, "parts.control-border on page in light"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			d, err := tokens.ParseBytes(replace(t, doc, c.from, c.to))
			if err != nil {
				t.Fatal(err)
			}
			low := d.Low()
			if len(low) == 0 || !slices.ContainsFunc(low, func(s string) bool { return strings.HasPrefix(s, c.want) }) {
				t.Errorf("Low = %v, want a line that starts %q", low, c.want)
			}
		})
	}
}

// TestAColourWithoutAValueInOneModeIsRefused: light and dark are written once
// each under one name; a role with one missing, or one that is no #rrggbb, is
// an error that names it (web.tokens.11, web.tokens.18).
func TestAColourWithoutAValueInOneModeIsRefused(t *testing.T) {
	t.Parallel()
	_, doc := shipped(t)
	for _, c := range []struct{ name, from, to, want string }{
		{"no dark", `"dark": "#a6a6a6",\s*`, ``, "text.note: no value in dark"},
		{"no light", `"light": "#595959",\s*`, ``, "text.note: no value in light"},
		{"short hex", `"light": "#595959"`, `"light": "#595"`, "not lower-case #rrggbb"},
		{"upper case", `"light": "#595959"`, `"light": "#59595F"`, "not lower-case #rrggbb"},
		{"unknown ground", `("prose": \{[^}]*"on": \["page", ")surface`, `${1}missing`, `sits on "missing"`},
		{"unknown field", `"fonts": \{`, `"colour": {}, "fonts": {`, "unknown field"},
		{"a breakpoint gone", `"compact": "40rem",\s*`, ``, "breakpoints"},
		{"a standing role gone", `"slate": \{`, `"slate2": {`, "standing.slate: the standing role is missing"},
		{"a shadow with no dark", `"dark": "0 12px 32px #00000080"`, `"dark": ""`, "shadow.popup: dark"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := tokens.ParseBytes(replace(t, doc, c.from, c.to))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("ParseBytes = %v, want an error holding %q", err, c.want)
			}
		})
	}
}

// TestThePaletteTheTerminalEmbedsIsWhatTheDocumentGenerates: palette.json is the
// golden of the palette, byte for byte what the document renders, and names
// its generator, so the terminal and the web share role names and values
// (web.tokens.19, cli.look.2). That the terminal loads those roles is the test
// of internal/host/term, which dev may not import.
func TestThePaletteTheTerminalEmbedsIsWhatTheDocumentGenerates(t *testing.T) {
	t.Parallel()
	d, _ := shipped(t)
	want, err := d.Palette()
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile("../../../" + tokens.PaletteFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != string(want) {
		t.Fatal(tokens.PaletteFile + " is not what the document generates; run make gen")
	}
	if !strings.Contains(string(have), `"$generated"`) {
		t.Error("palette.json does not name its generator")
	}
}

// TestEveryGeneratedFileNamesItsGenerator: the CSS and the theme open with the
// header, and the CSS scopes both modes by the one method (web.tokens.15,
// supply.provenance.4).
func TestEveryGeneratedFileNamesItsGenerator(t *testing.T) {
	t.Parallel()
	d, _ := shipped(t)
	for name, b := range map[string][]byte{"css": d.CSS(), "theme": d.Theme()} {
		if !strings.HasPrefix(string(b), "/* Generated by gragas-dev tokens v"+tokens.Version+" from "+tokens.File) {
			t.Errorf("%s does not open with the header", name)
		}
	}
	css := string(d.CSS())
	for _, scope := range []string{":root,\n:host {", `:root:not([data-theme="light"])`, `:root[data-theme="dark"]`, "@media (prefers-color-scheme: dark)"} {
		if !strings.Contains(css, scope) {
			t.Errorf("the CSS has no %q", scope)
		}
	}
}

// TestEveryColourIsInBothModesOfTheCSS: each colour role's two values are in
// the light block and in the dark ones, so a mode is never half written.
func TestEveryColourIsInBothModesOfTheCSS(t *testing.T) {
	t.Parallel()
	d, _ := shipped(t)
	css := string(d.CSS())
	light, rest, _ := strings.Cut(css, "@media")
	dark, _ := strings.CutPrefix(rest, "")
	for _, s := range []struct {
		kind  string
		roles map[string]tokens.Colour
	}{{"ground", d.Grounds}, {"text", d.Text}, {"standing", d.Standing}, {"mark", d.Marks}, {"observation", d.Observations}} {
		for n, c := range s.roles {
			if !strings.Contains(light, "--gr-"+s.kind+"-"+n+": "+c.Light+";") {
				t.Errorf("%s %s: no light value", s.kind, n)
			}
			if strings.Count(dark, "--gr-"+s.kind+"-"+n+": "+c.Dark+";") != 2 {
				t.Errorf("%s %s: the dark value is not in both dark blocks", s.kind, n)
			}
		}
	}
}
