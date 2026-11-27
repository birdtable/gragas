package coreapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// sarifResult is the part of a SARIF result these tests read.
type sarifResult struct {
	RuleID    string `json:"ruleId"`
	Level     string `json:"level"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region *struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
}

// sarifResults is the results of the one run of doc, and its rule ids.
func sarifResults(t *testing.T, doc []byte) ([]sarifResult, []string) {
	t.Helper()
	var d struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []sarifResult `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(doc, &d); err != nil || len(d.Runs) != 1 {
		t.Fatalf("not one SARIF run: %v\n%s", err, doc)
	}
	var ids []string
	for _, r := range d.Runs[0].Tool.Driver.Rules {
		ids = append(ids, r.ID)
	}
	return d.Runs[0].Results, ids
}

// where is r's file and line, "" and 0 for none, and whether it has a
// location at all.
func where(r sarifResult) (string, int, bool) {
	if len(r.Locations) == 0 {
		return "", 0, false
	}
	l := r.Locations[0].PhysicalLocation
	if l.Region == nil {
		return l.ArtifactLocation.URI, 0, true
	}
	return l.ArtifactLocation.URI, l.Region.StartLine, true
}

// TestValidationSARIFTakesThePlaceFromTheFieldsNeverTheEntity: a file named
// x:12 with no line is that file and no line, and the names check, which is
// about no file, has no location, whatever entity says for display. Planted
// failure: the place read back out of entity.
func TestValidationSARIFTakesThePlaceFromTheFieldsNeverTheEntity(t *testing.T) {
	t.Parallel()
	v := model.Validation{
		Findings: []model.Finding{
			{Entity: "x:12", Msg: "a whole file", File: "x:12"},
			{Entity: "rulebook.yaml:3", Msg: "a line", Fix: "keep one", File: "rulebook.yaml", Line: 3},
		},
		Notices: []model.Notice{{Entity: "names check", Msg: "the names check did not run"}},
	}
	doc, err := coreapi.ValidationSARIF(v, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	got, ids := sarifResults(t, doc)
	if len(ids) != 1 || ids[0] != model.ValidateCheck {
		t.Errorf("rules = %v, want the validate check alone", ids)
	}
	want := []struct {
		claim string
		file  string
		line  int
		has   bool
		level string
	}{
		{"a file named x:12 is that file with no line", "x:12", 0, true, ""},
		{"a finding at a line is at that file and line", "rulebook.yaml", 3, true, ""},
		{"the names check notice has no location", "", 0, false, "note"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d results, want %d:\n%s", len(got), len(want), doc)
	}
	for i, w := range want {
		file, line, has := where(got[i])
		if file != w.file || line != w.line || has != w.has || got[i].Level != w.level || got[i].RuleID != model.ValidateCheck {
			t.Errorf("%s: result %d = %q line %d located %v level %q, want %q line %d located %v level %q", w.claim, i+1, file, line, has, got[i].Level, w.file, w.line, w.has, w.level)
		}
	}
}

// TestValidateSARIFOfABadBookHasEveryFindingAtItsLineAndTheNamesCheckNowhere:
// the loader's own file and line reach SARIF, and the names check notice it
// writes has no location. Planted failure: the names check's display name
// written as a file.
func TestValidateSARIFOfABadBookHasEveryFindingAtItsLineAndTheNamesCheckNowhere(t *testing.T) {
	t.Parallel()
	files := goodBook()
	files["rulebook.yaml"] = "format: 3\nname: a\nname: b\n"
	w, root := validateWorld(t, files)
	v, err := coreapi.Validate(context.Background(), w.Ports(), root, "")
	if err != nil || len(v.Findings) != 1 || len(v.Notices) == 0 {
		t.Fatalf("Validate = %+v, %v", v, err)
	}
	doc, err := coreapi.ValidationSARIF(v, "dev")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := sarifResults(t, doc)
	if len(got) != len(v.Findings)+len(v.Notices) {
		t.Fatalf("%d results, want every finding and notice:\n%s", len(got), doc)
	}
	if file, line, _ := where(got[0]); file != "rulebook.yaml" || line != 3 || got[0].Level != "" {
		t.Errorf("the finding is at %q line %d level %q, want rulebook.yaml line 3 at the default level", file, line, got[0].Level)
	}
	names := 0
	for _, r := range got[1:] {
		if r.Level != "note" {
			t.Errorf("notice %q is at level %q, want note", r.Message.Text, r.Level)
		}
		if !strings.HasPrefix(r.Message.Text, "the names check did not run") {
			continue
		}
		names++
		if file, line, has := where(r); has {
			t.Errorf("the names check notice is located at %q line %d, want no location: it is about no file", file, line)
		}
	}
	if names != 1 {
		t.Errorf("%d names check notices, want 1:\n%s", names, doc)
	}
}

// TestValidationSARIFNamesTheReleaseTagsVersion: a release tag is written
// without its v, and a build past a tag or with none has no version. Planted
// failure: the tag written as it is, which the schema refuses.
func TestValidationSARIFNamesTheReleaseTagsVersion(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ tag, want string }{
		"a release tag is written without its v": {"v1.2.3", `"version": "1.2.3"`},
		"a build past a tag has no version":      {"v1.2.3-3-gabc1234", ""},
		"a development build has no version":     {"dev", ""},
		"a build with no tag has no version":     {"", ""},
	} {
		tag, want := c.tag, c.want
		doc, err := coreapi.ValidationSARIF(model.Validation{}, tag)
		if err != nil {
			t.Fatal(err)
		}
		// The document's own "version": "2.1.0" is SARIF's, not the engine's.
		got := strings.Count(string(doc), `"version": "`) - 1
		if want == "" && got != 0 || want != "" && (got != 1 || !strings.Contains(string(doc), want)) {
			t.Errorf("%s: ValidationSARIF(tag %q) =\n%s\nwant the engine's version %q", name, tag, doc, want)
		}
	}
}

// TestAMissingNamesFileIsNamedBySettingNeverByPath: when the names file the
// settings name does not exist, the notice says so by the settings that name
// it. No part of its path reaches the notice, so none reaches --json or SARIF,
// which leave the machine: the path says where the private list lives, and a
// home folder often holds a person's name. Planted failure: the path written
// into the notice, as it was before.
func TestAMissingNamesFileIsNamedBySettingNeverByPath(t *testing.T) {
	t.Parallel()
	w, root := validateWorld(t, goodBook())
	dir := "plant-home-folder"
	base := "plant-no-such-names.txt"
	missing := "/" + dir + "/" + base
	v, err := coreapi.Validate(context.Background(), w.Ports(), root, missing)
	if err != nil {
		t.Fatalf("Validate with a missing names file = %v, want a notice", err)
	}
	doc, err := coreapi.ValidationSARIF(v, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	said := 0
	for _, n := range v.Notices {
		if !strings.HasPrefix(n.Msg, "the names check did not run") {
			continue
		}
		said++
		if !strings.Contains(n.Msg, "GRAGAS_NAMES_FILE") || !strings.Contains(n.Msg, "names-file") {
			t.Errorf("the notice %q does not name the settings that name the file", n.Msg)
		}
	}
	if said != 1 {
		t.Fatalf("%d names check notices, want 1: %+v", said, v.Notices)
	}
	for _, part := range []string{dir, base} {
		for where, text := range map[string]string{"the notices": fmt.Sprint(v.Notices), "the SARIF": string(doc)} {
			if strings.Contains(text, part) {
				t.Errorf("%s: %q, a part of the names file's path, is there:\n%s", where, part, text)
			}
		}
	}
}

// TestCoverageSARIFNamesEachUnmappedEntryInTheFileOfItsFlattenedSet: one result
// for each entry no rule explains, in the file of the flattened set that holds
// it below the rulebook's folder, and none for a listing with nothing unmapped.
// gragas check reports these; coverage itself ends done and draws a listing.
func TestCoverageSARIFNamesEachUnmappedEntryInTheFileOfItsFlattenedSet(t *testing.T) {
	t.Parallel()
	c := model.Coverage{Dir: "book", Unmapped: []model.CoverUnmapped{{Tool: "lint", Entry: "a"}, {Tool: "lint", Entry: "b"}}}
	doc, err := coreapi.CoverageSARIF(c, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(doc), `"ruleId"`); n != 2 || !strings.Contains(string(doc), "book/upstream/lint.flat.json") {
		t.Errorf("%d results\n%s", n, doc)
	}
	if doc, err = coreapi.CoverageSARIF(model.Coverage{}, "v1.2.3"); err != nil || strings.Contains(string(doc), `"ruleId"`) {
		t.Errorf("an empty coverage has a result: %v\n%s", err, doc)
	}
}
