package coreapi

import (
	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/model"
)

// validateCheckText is the sentence SARIF shows beside model.ValidateCheck.
const validateCheckText = "The validate check: the rulebook's files are written as the format needs (not a rulebook rule)"

// ValidationSARIF is v as one SARIF document of the contract (cli.output.2):
// every finding, then every notice at level note, each with the file and line
// the loader read it at. The listing's limit is the screen's, so SARIF holds
// all of them. tag is the engine's release tag (v1.2.3): SARIF names its
// version without the v, and none for a build past a tag or with no tag.
func ValidationSARIF(v model.Validation, tag string) ([]byte, error) {
	version := tag
	if len(tag) > 1 && tag[0] == 'v' {
		version = tag[1:]
	}
	tool := contract.Tool{Name: "gragas", Version: version, Checks: []contract.Check{{ID: model.ValidateCheck, Text: validateCheckText}}}
	results := make([]contract.Result, 0, len(v.Findings)+len(v.Notices))
	for _, f := range v.Findings {
		results = append(results, contract.Result{Check: model.ValidateCheck, File: f.File, Line: f.Line, Msg: f.Msg, Fix: f.Fix})
	}
	for _, n := range v.Notices {
		results = append(results, contract.Result{Check: model.ValidateCheck, File: n.File, Line: n.Line, Msg: n.Msg, Notice: true})
	}
	return contract.EncodeFindings(tool, results)
}
