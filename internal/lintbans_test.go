package internal

// The lint config's bans, held to the exclusions that switch them off
// (gragas#1719 C3, gragas#1750). An exclusion for forbidigo, depguard or
// every linter is held to a rule's id and colon by the shared registry's
// check (TestAnExclusionThatCouldVoidABanIsRefused), so its text reaches every
// ban whose message opens with that id: a ban added later under the id, or
// a ban whose message moves to it, is switched off where the exclusion's
// paths reach, with nothing in the diff to say so. This check reads the
// config at the merge base with origin/main and refuses that.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/ordered"
)

// configBan is a forbidigo row or a depguard deny of a lint config: its key
// (the linter and the fields it is held by, not its message), its linter and
// the message a finding carries.
type configBan struct{ key, linter, msg string }

// mapAt is the mapping at the dotted path in doc, or nil.
func mapAt(doc *ordered.Map, path string) *ordered.Map {
	v, _ := lookup(doc, path)
	m, _ := v.(*ordered.Map)
	return m
}

// configBans is every forbidigo row and depguard deny of doc.
func configBans(doc *ordered.Map) []configBan {
	var out []configBan
	if f := mapAt(doc, "linters.settings.forbidigo"); f != nil {
		for _, r := range ordered.Maps(f.List("forbid")) {
			out = append(out, configBan{"forbidigo " + r.Str("pattern") + " pkg " + r.Str("pkg"), "forbidigo", r.Str("msg")})
		}
	}
	if rules := mapAt(doc, "linters.settings.depguard.rules"); rules != nil {
		for _, name := range rules.Keys() {
			for _, d := range ordered.Maps(rules.Sub(name).List("deny")) {
				out = append(out, configBan{"depguard " + name + " " + d.Str("pkg"), "depguard", d.Str("desc")})
			}
		}
	}
	return out
}

// voidingTexts is the text of each exclusion rule of doc that switches off
// ban somewhere, keyed by the text and whom it is for ("every linter" or the
// ban's linter), to the files it names. The files do not decide whether it
// reaches the ban, only where.
func voidingTexts(doc *ordered.Map, ban configBan) map[string]string {
	out := map[string]string{}
	ex := mapAt(doc, "linters.exclusions")
	if ex == nil {
		return out
	}
	for _, r := range ordered.Maps(ex.List("rules")) {
		k, ok := exclusionFor(r, ban.linter)
		if !ok {
			continue
		}
		if re, err := regexp.Compile(r.Str("text")); err == nil && !re.MatchString(ban.msg) {
			continue
		}
		if out[k] != "" {
			out[k] += ", "
		}
		out[k] += excludedFiles(r)
	}
	return out
}

// exclusionFor is the key of exclusion rule r, its text and whom it is for,
// and whether it is for linter at all.
func exclusionFor(r *ordered.Map, linter string) (string, bool) {
	linters := ordered.Strings(r.List("linters"))
	if len(linters) > 0 && !slices.Contains(linters, linter) {
		return "", false
	}
	who := "every linter"
	if len(linters) > 0 {
		who = linter
	}
	return who + " text " + strings.TrimSpace(r.Str("text")), true
}

// exclusionTexts is the key of every exclusion rule of doc for linter, as
// exclusionFor gives it, whatever ban it reaches.
func exclusionTexts(doc *ordered.Map, linter string) map[string]bool {
	out := map[string]bool{}
	if ex := mapAt(doc, "linters.exclusions"); ex != nil {
		for _, r := range ordered.Maps(ex.List("rules")) {
			if k, ok := exclusionFor(r, linter); ok {
				out[k] = true
			}
		}
	}
	return out
}

// excludedFiles names the files an exclusion rule is for.
func excludedFiles(r *ordered.Map) string {
	switch {
	case r.Str("path") != "":
		return r.Str("path")
	case r.Str("path-except") != "":
		return "every file but " + r.Str("path-except")
	}
	return "every file"
}

// silentVoids is each ban of now that an exclusion switches off and that the
// same text did not switch off at base, unless the ban and the text are both
// new: a ban that comes with its own exclusion stands in the diff for review
// to read (L11a of gragas#1750). Refused: a ban new since base, or one whose
// message changed, reached by a text that was already there; a ban that was
// there, reached by a new text. A shared row's ban (arch.shared.8) is spared:
// its owner's exclusion is the row's, held by the registry's own check. A
// change to an exclusion's paths alone is read by review in lint/.
func silentVoids(now, base *ordered.Map) []string {
	baseBans := map[string]configBan{}
	for _, b := range configBans(base) {
		baseBans[b.key] = b
	}
	var out []string
	for _, b := range configBans(now) {
		if strings.HasPrefix(b.msg, layers.SharedRule+":") {
			continue
		}
		then, had := baseBans[b.key]
		var before map[string]string
		if had {
			before = voidingTexts(base, then)
		}
		baseTexts := exclusionTexts(base, b.linter)
		for k, files := range voidingTexts(now, b) {
			if _, ok := before[k]; ok {
				continue
			}
			if !had && !baseTexts[k] {
				continue
			}
			why := "the ban is new since the merge base with origin/main, and a new ban takes no exclusion that was already there"
			if had {
				why = "the ban was there at the merge base with origin/main, and the exclusion did not reach it; a ban gains no exclusion by a change to its message or to the exclusion's text"
			}
			out = append(out, "the "+b.key+" ("+b.msg+") is switched off at "+files+" by the exclusion for "+k+": "+why+"; give the ban a message of its own id, or its own exclusion beside it (gragas#1750)")
		}
	}
	slices.Sort(out)
	return out
}

// TestNoExclusionSwitchesOffABanSilently holds .golangci.yml to its merge
// base with origin/main (gragas#1719 C3, gragas#1750). With no merge base it
// is red and says why.
func TestNoExclusionSwitchesOffABanSilently(t *testing.T) {
	t.Parallel()
	text, why, _ := osListBase(t, root, ".golangci.yml")
	if why != "" {
		t.Fatal(why)
	}
	v, err := ordered.ParseYAML([]byte(text))
	base, ok := v.(*ordered.Map)
	if err != nil || !ok {
		t.Fatalf("the lint config at the merge base does not read as a mapping: %v", err)
	}
	now := lintDoc(t, ".golangci.yml")
	if len(configBans(now)) == 0 {
		t.Fatal("no ban read from the lint config")
	}
	for _, p := range silentVoids(now, base) {
		t.Error(p)
	}
}

// TestASilentExclusionIsRefused: planted configs, each against one base.
// Refused: a new ban under an excluded id, a ban whose message moves to one,
// a new text that reaches a ban that was there, and a text for every linter.
// Not refused: the base itself, a new ban under an id of its own, a new ban
// with a new text of its own, a path widened or narrowed, a text gone, and
// a shared row's ban under an owner's text.
func TestASilentExclusionIsRefused(t *testing.T) {
	t.Parallel()
	const head = "linters:\n  settings:\n    forbidigo:\n      forbid:\n"
	const env = "        - pattern: ^os\\.Getenv$\n          msg: \"cli.config.3: read the environment in the config package\"\n"
	const dep = "    depguard:\n      rules:\n        no-os-packages:\n          deny:\n            - pkg: syscall\n              desc: \"arch.ports.24: ban-120: no syscall\"\n"
	const rules = "  exclusions:\n    rules:\n"
	const envEx = "      - path: ^internal/config/\n        linters: [forbidigo]\n        text: \"cli\\\\.config\\\\.3:\"\n"
	const depEx = "      - path: ^internal/host/\n        linters: [depguard]\n        text: \"arch\\\\.ports\\\\.24: ban-120:\"\n"
	const exit = "        - pattern: ^os\\.Exit$\n          msg: \"go.exits.1: only main exits\"\n"
	base := head + env + exit + dep + rules + envEx + depEx
	doc := func(t *testing.T, y string) *ordered.Map {
		t.Helper()
		v, err := ordered.ParseYAML([]byte(y))
		m, ok := v.(*ordered.Map)
		if err != nil || !ok {
			t.Fatalf("a plant does not read: %v\n%s", err, y)
		}
		return m
	}
	for _, tc := range []struct {
		name, config, says string
	}{
		{"the base", base, ""},
		{"a new ban under an excluded id", head + env + "        - pattern: ^os\\.LookupEnv$\n          msg: \"cli.config.3: read the environment in the config package\"\n" + dep + rules + envEx + depEx, "the forbidigo ^os\\.LookupEnv$ pkg  (cli.config.3:"},
		{"a new import ban under an excluded id", head + env + strings.Replace(dep, "            - pkg: syscall\n", "            - pkg: os/signal\n              desc: \"arch.ports.24: ban-120: use ui.Main\"\n            - pkg: syscall\n", 1) + rules + envEx + depEx, "the depguard no-os-packages os/signal"},
		{"a ban whose message moves to an excluded id", head + env + strings.Replace(exit, "go.exits.1:", "cli.config.3:", 1) + dep + rules + envEx + depEx, "the forbidigo ^os\\.Exit$ pkg  (cli.config.3: only main exits) is switched off at ^internal/config/ by the exclusion for forbidigo text cli\\.config\\.3:: the ban was there"},
		{"a ban gone", head + env + dep + rules + envEx + depEx, ""},
		{"a new text reaching a ban that was there", base + "      - path: ^cmd/\n        linters: [depguard]\n        text: \"arch\\\\.ports\\\\.24:\"\n", "by the exclusion for depguard text arch\\.ports\\.24:: the ban was there"},
		{"a text for every linter reaching a ban that was there", base + "      - path: ^cmd/\n        text: \"cli\\\\.config\\\\.3:\"\n", "by the exclusion for every linter text cli\\.config\\.3:"},
		{"a new ban under an id of its own", head + env + "        - pattern: ^os\\.Chmod$\n          msg: \"arch.ports.25: use write.Mode\"\n" + dep + rules + envEx + depEx, ""},
		{"a new ban with a new text of its own", head + env + "        - pattern: ^path\\.Join$\n          msg: \"arch.ports.26: path is for slash names\"\n" + dep + rules + envEx + depEx + "      - path: ^internal/host/assets/assets\\.go$\n        linters: [forbidigo]\n        text: \"arch\\\\.ports\\\\.26:\"\n", ""},
		{"a path widened", head + env + dep + rules + strings.Replace(envEx, "^internal/config/", "^internal/", 1) + depEx, ""},
		{"a path narrowed", head + env + dep + rules + envEx + strings.Replace(depEx, "^internal/host/", "^internal/host/fs/", 1), ""},
		{"a text gone", head + env + dep + rules + envEx, ""},
		{"a shared row's ban under its owner's text", head + env + "        - pattern: ^syscall\\.Flock$\n          msg: \"arch.shared.8: lock belongs to internal/host/fs; use its Lock.Take\"\n" + dep + rules + envEx + depEx + "      - path: ^internal/host/fs/\n        linters: [forbidigo]\n        text: \"arch\\\\.shared\\\\.8: lock belongs to\"\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := silentVoids(doc(t, tc.config), doc(t, base))
			switch {
			case tc.says == "" && len(got) != 0:
				t.Errorf("refused: %q", got)
			case tc.says != "" && !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.says) }):
				t.Errorf("got %q, want one saying %q", got, tc.says)
			}
		})
	}
}
