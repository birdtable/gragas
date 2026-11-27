package contract

import (
	"fmt"
	"regexp"
	"slices"

	mc "github.com/birdtable/gragas/internal/model/contract"
)

// SARIFVersion is the SARIF version every SARIF document of the contract
// names.
const SARIFVersion = "2.1.0"

// Tool is the program a SARIF document's results are from: its name, its
// version, and the checks the results name, each once.
type Tool struct {
	// Name is the program's name, such as gragas.
	Name string
	// Version is its version, written only when it is a semantic version
	// (a development build has none).
	Version string
	// Checks are the ids the results may name, with a sentence for each.
	Checks []Check
}

// Check is one id a result names, with the sentence SARIF shows beside it.
type Check struct {
	ID   string
	Text string
}

// Result is one thing a run says, as a SARIF result: a finding, or a notice,
// which SARIF writes at level note.
type Result struct {
	// Check is the id of the check or rule that says it, one of the tool's.
	Check string
	// File is the file it is about, slash-separated and relative, and ""
	// when it is about none; Line is the line, 0 when there is none.
	File string
	Line int
	// Msg is what is wrong, and Fix the fix when one is offered.
	Msg string
	Fix string
	// Notice is whether it is a notice: information that never changes the
	// closing word.
	Notice bool
}

var (
	idText     = regexp.MustCompile(`^[a-z]+\.[a-z0-9]+(-[a-z0-9]+)*\.[1-9][0-9]*$`)
	semverText = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// EncodeFindings is results as one SARIF document of the contract (A34,
// cli.output.2): one run of tool, in the order results are given, through
// Encode. It refuses a result naming a check the tool does not list, an id
// that is not of the form part.concern.n, and an empty message, each of which
// the SARIF schema of the contract would refuse.
func EncodeFindings(tool Tool, results []Result) ([]byte, error) {
	if tool.Name == "" {
		return nil, fmt.Errorf("encode SARIF: the tool has no name")
	}
	driver := mc.SarifDocumentRunsToolDriver{Name: mc.Text(tool.Name)}
	if semverText.MatchString(tool.Version) {
		v := mc.Semver(tool.Version)
		driver.Version = &v
	}
	var ids []string
	for _, c := range tool.Checks {
		if !idText.MatchString(c.ID) || c.Text == "" || slices.Contains(ids, c.ID) {
			return nil, fmt.Errorf("encode SARIF: the check %q needs an id of the form part.concern.n, named once, and a sentence", clip(c.ID))
		}
		ids = append(ids, c.ID)
		driver.Rules = append(driver.Rules, mc.SarifDocumentRunsToolDriverRules{
			ID: mc.ID(c.ID), ShortDescription: &mc.SarifDocumentRunsToolDriverRulesShortDescription{Text: mc.Text(c.Text)},
		})
	}
	run := mc.SarifDocumentRuns{Tool: mc.SarifDocumentRunsTool{Driver: driver}, Results: []mc.SarifDocumentRunsResults{}}
	for i, r := range results {
		out, err := result(r, ids)
		if err != nil {
			return nil, fmt.Errorf("encode SARIF: result %d: %w", i+1, err)
		}
		run.Results = append(run.Results, out)
	}
	return Encode(SARIF, mc.SarifDocument{Version: SARIFVersion, Runs: []mc.SarifDocumentRuns{run}})
}

// result is r as a SARIF result, given the ids the tool lists.
func result(r Result, ids []string) (mc.SarifDocumentRunsResults, error) {
	if !slices.Contains(ids, r.Check) {
		return mc.SarifDocumentRunsResults{}, fmt.Errorf("it names the check %q, which the tool does not list", clip(r.Check))
	}
	if r.Msg == "" {
		return mc.SarifDocumentRunsResults{}, fmt.Errorf("it has no message")
	}
	out := mc.SarifDocumentRunsResults{RuleID: mc.ID(r.Check), Message: mc.SarifDocumentRunsResultsMessage{Text: mc.Text(r.Msg)}}
	if r.File != "" {
		loc := mc.SarifDocumentRunsResultsLocationsPhysicalLocation{
			ArtifactLocation: mc.SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation{URI: mc.Text(r.File)},
		}
		if r.Line > 0 {
			line := r.Line
			loc.Region = &mc.SarifDocumentRunsResultsLocationsPhysicalLocationRegion{StartLine: &line}
		}
		out.Locations = []mc.SarifDocumentRunsResultsLocations{{PhysicalLocation: loc}}
	}
	if r.Notice {
		note := mc.SarifDocumentRunsResultsLevelNote
		out.Level = &note
	}
	if r.Fix != "" || r.Notice {
		out.Properties = &mc.SarifDocumentRunsResultsProperties{
			Gragas: &mc.SarifDocumentRunsResultsPropertiesGragas{Fix: mc.TextOrEmpty(r.Fix), Notice: r.Notice},
		}
	}
	return out, nil
}
