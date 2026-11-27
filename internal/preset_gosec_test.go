package internal

import (
	"cmp"
	"debug/buildinfo"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// gosecPlant is one gosec rule's proof for the shared preset: source that
// breaks the rule, and the same code made right, each the body of one file of
// a package named for the rule (gosecPlants holds one for each rule).
type gosecPlant struct {
	planted, clean string
	// goVersion is the go line of the module the plant is linted in, when it
	// is not plantsGo: a rule that applies only to an older Go.
	goVersion string
	// silent is why the preset's settings leave the plant unreported, for a
	// rule the preset turns on that cannot fire at them. The test fails if
	// the plant is reported, so the reason is looked at again.
	silent string
}

// plantsGo is the go line of the modules the plants are linted in.
const plantsGo = "1.23.0"

// presetGosecRules are the gosec rules presets/golangci-lint/.golangci.yml turns on: its
// includes, or every rule of gosecRules when it lists none, less its excludes.
func presetGosecRules(t *testing.T) []string {
	t.Helper()
	settings := lintDoc(t, "presets/golangci-lint/.golangci.yml").Sub("linters").Sub("settings")
	if settings == nil || settings.Sub("gosec") == nil {
		t.Fatal("presets/golangci-lint/.golangci.yml has no linters.settings.gosec")
	}
	gosec := settings.Sub("gosec")
	listed := func(key string) []string {
		var rules []string
		for _, r := range gosec.List(key) {
			s, ok := r.(string)
			if !ok {
				t.Fatalf("gosec %s holds %v, not a rule id", key, r)
			}
			rules = append(rules, s)
		}
		return rules
	}
	rules := listed("includes")
	if len(rules) == 0 {
		rules = gosecRules()
	}
	excluded := listed("excludes")
	return slices.DeleteFunc(rules, func(r string) bool { return slices.Contains(excluded, r) })
}

// carriedGosec is the version of gosec the golangci-lint at bin was built
// with, from the binary's own build info.
func carriedGosec(t *testing.T, bin string) string {
	t.Helper()
	info, err := buildinfo.ReadFile(bin)
	if err != nil {
		t.Fatalf("%s: its build info cannot be read, so the gosec it carries is not known: %v", bin, err)
	}
	for _, d := range info.Deps {
		if d.Path == "github.com/securego/gosec/v2" {
			if d.Replace != nil {
				t.Fatalf("%s carries gosec replaced by %s %s, not a release", bin, d.Replace.Path, d.Replace.Version)
			}
			return d.Version
		}
	}
	t.Fatalf("%s carries no github.com/securego/gosec/v2", bin)
	return ""
}

// gosecModules are the files of the modules the plants are linted in, one for
// each go line a plant asks for, and of the one module of every rule's clean
// control. Each holds the preset as its .golangci.yml and the stand-in for
// golang.org/x/crypto.
func gosecModules(preset string, rules []string, plants map[string]gosecPlant) (planted map[string]map[string]string, clean map[string]string) {
	module := func(goVersion string) map[string]string {
		files := map[string]string{
			".golangci.yml": preset,
			"go.mod":        "module example.com/planted\n\ngo " + goVersion + "\n\nrequire golang.org/x/crypto v0.0.0\n\nreplace golang.org/x/crypto => ./xcrypto\n",
		}
		for name, body := range gosecStandIn() {
			files["xcrypto/"+name] = body
		}
		return files
	}
	file := func(rule, body string) (string, string) {
		pkg := strings.ToLower(rule)
		return pkg + "/x.go", "// Package " + pkg + " is the plant of gosec " + rule + ".\npackage " + pkg + "\n\n" + body
	}
	planted, clean = map[string]map[string]string{}, module(plantsGo)
	for _, rule := range rules {
		p, ok := plants[rule]
		if !ok {
			continue
		}
		goVersion := cmp.Or(p.goVersion, plantsGo)
		if planted[goVersion] == nil {
			planted[goVersion] = module(goVersion)
		}
		name, body := file(rule, p.planted)
		planted[goVersion][name] = body
		name, body = file(rule, p.clean)
		clean[name] = body
	}
	return planted, clean
}

// TestEachGosecRuleThePresetTurnsOnFindsItsPlantAndSparesItsControl runs the
// pinned golangci-lint with presets/golangci-lint/.golangci.yml, as a project that
// imports it would, over modules that break each gosec rule the preset
// turns on, one for each go line a plant needs, and over one that does not: each plant is reported under its
// rule's id in its own package, and the controls are reported by nothing. A
// rule the preset turns on with no plant fails, and so does a pinned
// golangci-lint that carries a gosec other than the one gosecRules lists
// (gates.plants.1). Each module is linted in one run that keeps every finding
// of a line, so a second rule on a plant's line cannot hide it, and that runs
// beside any other golangci-lint on this host, as make lint does. A preset that
// narrows the rules is refused by TestNoExclusionDropsANamedGosecFamily, over
// the .golangci.yml that the generated preset renders.
// Planted failure: a rule with no plant, a plant that no longer breaks its
// rule, a rule held silent whose plant is reported, or a golangci-lint bump
// that brings a gosec whose rules nobody has listed.
func TestEachGosecRuleThePresetTurnsOnFindsItsPlantAndSparesItsControl(t *testing.T) {
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	bin := pinnedLint(t)
	if v := carriedGosec(t, bin); v != gosecVersion {
		t.Fatalf("the pinned golangci-lint carries gosec %s, and gosecRules and gosecPlants are written for %s: list the rules of %s there, and plant each new one", v, gosecVersion, v)
	}
	plants, all := gosecPlants(), gosecRules()
	rules := presetGosecRules(t)
	for _, rule := range rules {
		if !slices.Contains(all, rule) {
			t.Errorf("the preset turns on gosec %s, which gosec %s does not have", rule, gosecVersion)
		} else if _, ok := plants[rule]; !ok {
			t.Errorf("the preset turns on gosec %s, and no plant proves it: add one to gosecPlants", rule)
		}
	}
	for rule := range plants {
		if !slices.Contains(all, rule) {
			t.Errorf("gosecPlants holds %s, which gosec %s does not have", rule, gosecVersion)
		}
	}
	run := func(files map[string]string) (string, bool) {
		return lint(t, bin, writeModule(t, files), exec.CommandContext(t.Context(), "golangci-lint", "run",
			"--enable-only=gosec", "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./..."))
	}
	planted, clean := gosecModules(string(preset), rules, plants)
	if out, ok := run(clean); !ok {
		t.Errorf("the clean controls are reported:\n%s", out)
	}
	found, outs := map[string]bool{}, ""
	finding := regexp.MustCompile(`(?m)^(g\d{3})/x\.go:\d+:\d+: (G\d{3}):`)
	for _, goVersion := range slices.Sorted(maps.Keys(planted)) {
		out, _ := run(planted[goVersion])
		outs += out
		for _, m := range finding.FindAllStringSubmatch(out, -1) {
			found[m[1]+" "+m[2]] = true
		}
	}
	for _, rule := range rules {
		p, ok := plants[rule]
		if !ok {
			continue
		}
		switch hit := found[strings.ToLower(rule)+" "+rule]; {
		case p.silent == "" && !hit:
			t.Errorf("%s: its plant is not reported under %s", rule, rule)
		case p.silent != "" && hit:
			t.Errorf("%s: its plant is reported, and it is held silent because %s: drop the reason", rule, p.silent)
		}
	}
	if t.Failed() {
		t.Logf("the plants' lint said:\n%s", outs)
	}
}
