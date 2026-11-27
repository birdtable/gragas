package journeys

import (
	"maps"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// The checker code a rulebook ships, declared by name and publisher (#193,
// S23, S3): validate turns a planted undeclared artifact into a finding and
// leaves the clean control done, and tools lists the rulebook's own artifact
// under its own name and an import's by reference under the import's. An
// import's checker code is read once a fetch hands the import in; a
// declaration by reference is the importer's own file, so it lists now.

// checkersImport is the import the clean rulebook pins, never fetched here.
const checkersImport = `imports:
  - rulebook: shared-go
    version: 2.0.0
    parts: [ui]
`

// checkersClean is a rulebook with its own artifact, its tests in its folder,
// and an import's declared by reference.
func checkersClean() map[string]string {
	return map[string]string{
		"rulebook.yaml":                          vIdentity + checkersImport,
		"checkers/errwrap-plugin/checker.yaml":   "format: 3\nname: errwrap-plugin\ntool: golangci-lint\npublisher: team-a\ntests: testdata\nlicence: Apache-2.0\n",
		"checkers/errwrap-plugin/testdata/a.txt": "a case the plugin's own tests read\n",
		"checkers/shared-analyzer/checker.yaml":  "format: 3\nname: shared-analyzer\ntool: golangci-lint\npublisher: sam\nfrom: shared-go\n",
	}
}

// checkersBound is the clean rulebook with rules bound to tools, so the
// listing shows the tools and then the checker code.
func checkersBound() map[string]string {
	files := checkersClean()
	files["language/go/rules/errors.yaml"] = boundRules
	return files
}

// checkersPlanted is the clean rulebook with checker code no declaration names.
func checkersPlanted() map[string]string {
	files := checkersClean()
	maps.Copy(files, map[string]string{"checkers/stray-plugin/plugin.wasm": "code a consumer's tool would load\n"})
	return files
}

// TestTheCheckerCodeGoldens: validate over the planted rulebook ends
// on findings, naming the undeclared folder, and over the clean control ends
// done; tools over the clean control lists both artifacts, the import's by
// reference, and ends not ready, since no plugin serves its bound tools.
// Each at 80 columns against its golden.
// Risk: checker code a consumer's tools load that the rulebook never
// declared, or an import's listed as the importer's own.
func TestTheCheckerCodeGoldens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, command string
		files         map[string]string
		code          int
		wants         string
	}{
		{"checkers-planted", "validate", checkersPlanted(), 1, "checker code in checkers/stray-plugin/ has no"},
		{"checkers-clean", "validate", checkersClean(), 0, "0 findings"},
		{"tools-checkers", "tools", checkersBound(), 4, "shared-go, by reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := rulebook(t, tc.files)
			r := validateRun(t, dir, "80")
			if tc.command == "tools" {
				r = toolsRun(t, dir, "80")
			}
			if r.code != tc.code || !strings.Contains(r.stdout+r.stderr, tc.wants) {
				t.Errorf("exit %d, want %d and %q:\n%s%s", r.code, tc.code, tc.wants, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name+"-80", transcript([]string{tc.command}, r))
		})
	}
}
