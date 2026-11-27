package contract

import (
	"bytes"
	"os"
	"path"
	"strings"
	"testing"
)

// findingsTool is the tool every EncodeFindings test writes for: one check,
// as the validate command lists it.
func findingsTool() Tool {
	return Tool{Name: "gragas", Version: "1.2.3", Checks: []Check{{ID: "gragas.validate.1", Text: "the rulebook validates"}}}
}

// findingsResults is one result of each shape SARIF writes: a finding at a
// line with a fix, one about a whole file, one about no file, and a notice.
func findingsResults() []Result {
	return []Result{
		{Check: "gragas.validate.1", File: "rulebook/parts.yaml", Line: 12, Msg: "the part has no layer", Fix: "give it a layer"},
		{Check: "gragas.validate.1", File: "rulebook/rules.yaml", Msg: "the file is empty"},
		{Check: "gragas.validate.1", Msg: "the rulebook has no rules"},
		{Check: "gragas.validate.1", File: "rulebook/rules.yaml", Line: 3, Msg: "the rule is deprecated", Notice: true},
	}
}

// TestEncodeFindingsIsItsGoldenBytes: the results come out as one run of the
// tool, in the order given, with a line only where there is one, a fix in the
// gragas properties, and a notice at level note; the golden is a SARIF
// document the real schema takes (cmd/gragas checks it).
func TestEncodeFindingsIsItsGoldenBytes(t *testing.T) {
	t.Parallel()
	got, err := EncodeFindings(findingsTool(), findingsResults())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path.Join("testdata", "findings.sarif.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeFindings =\n%s\nwant\n%s", got, want)
	}
	_, s := world(t, schemaFiles(t))
	v, err := Validate(SARIF, got, s)
	if err != nil || len(v.Findings) != 0 {
		t.Errorf("Validate(EncodeFindings) = %v, %v, want no findings", v.Findings, err)
	}
}

// TestEncodeFindingsWritesNoResultsAsAnEmptyList: a run with nothing to say
// still lists its results, as [], and its check.
func TestEncodeFindingsWritesNoResultsAsAnEmptyList(t *testing.T) {
	t.Parallel()
	got, err := EncodeFindings(findingsTool(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"results": []`)) || !bytes.Contains(got, []byte(`"id": "gragas.validate.1"`)) {
		t.Errorf("EncodeFindings(none) =\n%s\nwant an empty results list and the check", got)
	}
}

// TestEncodeFindingsWritesAVersionOnlyWhenItIsSemantic: a development build's
// version is left out, not written as text the schema refuses.
func TestEncodeFindingsWritesAVersionOnlyWhenItIsSemantic(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		version string
		want    bool
	}{
		"a release is written":                 {"1.2.3", true},
		"a zero release is written":            {"0.0.0", true},
		"a development build is left out":      {"dev", false},
		"a version of two numbers is left out": {"1.2", false},
		"a version with its v is left out":     {"v1.2.3", false},
		"a leading zero is left out":           {"01.2.3", false},
		"no version is left out":               {"", false},
	} {
		version, want := c.version, c.want
		tool := findingsTool()
		tool.Version = version
		got, err := EncodeFindings(tool, nil)
		if err != nil {
			t.Fatal(err)
		}
		if has := bytes.Contains(got, []byte(`"version": "`+version+`"`)) && version != ""; has != want {
			t.Errorf("%s: EncodeFindings(version %q) writes it = %v, want %v:\n%s", name, version, has, want, got)
		}
	}
}

// TestEncodeFindingsRefusesWhatTheSchemaWouldRefuse: each input the SARIF
// schema of the contract refuses is refused before it is written, and nothing
// is written.
func TestEncodeFindingsRefusesWhatTheSchemaWouldRefuse(t *testing.T) {
	t.Parallel()
	ok := findingsResults()[0]
	cases := map[string]struct {
		tool    func(*Tool)
		result  func(*Result)
		wantErr string
	}{
		"a tool with no name":             {tool: func(x *Tool) { x.Name = "" }, wantErr: "the tool has no name"},
		"a check id not part.concern.n":   {tool: func(x *Tool) { x.Checks[0].ID = "Gragas.validate.1" }, result: func(x *Result) { x.Check = "Gragas.validate.1" }, wantErr: "needs an id"},
		"a check id numbered 0":           {tool: func(x *Tool) { x.Checks[0].ID = "gragas.validate.0" }, result: func(x *Result) { x.Check = "gragas.validate.0" }, wantErr: "needs an id"},
		"a check with no sentence":        {tool: func(x *Tool) { x.Checks[0].Text = "" }, wantErr: "needs an id"},
		"a check named twice":             {tool: func(x *Tool) { x.Checks = append(x.Checks, x.Checks[0]) }, wantErr: "needs an id"},
		"a result naming no listed check": {result: func(x *Result) { x.Check = "gragas.validate.2" }, wantErr: "which the tool does not list"},
		"a result with no message":        {result: func(x *Result) { x.Msg = "" }, wantErr: "it has no message"},
	}
	for name, c := range cases {
		tool, r := findingsTool(), ok
		if c.tool != nil {
			c.tool(&tool)
		}
		if c.result != nil {
			c.result(&r)
		}
		got, err := EncodeFindings(tool, []Result{r})
		if err == nil || !strings.Contains(err.Error(), c.wantErr) || got != nil {
			t.Errorf("%s: EncodeFindings = %q, %v, want nothing and an error naming %q", name, got, err, c.wantErr)
		}
	}
}
