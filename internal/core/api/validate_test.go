package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func validateSchemas() []string {
	return []string{"v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json"}
}

func validateWorld(t *testing.T, files map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(validateSchemas()...)
	return w, w.Seed("repo", files)
}

func goodBook() map[string]string {
	return map[string]string{"rulebook.yaml": bookIdentity, "structure.yaml": bookStructure, "language/go/rules/errors.yaml": bookRules}
}

// TestValidateOfAGoodBookIsDoneWithItsRuleCount: no finding, the rules counted.
func TestValidateOfAGoodBookIsDoneWithItsRuleCount(t *testing.T) {
	t.Parallel()
	w, root := validateWorld(t, goodBook())
	v, err := coreapi.Validate(context.Background(), w.Ports(), root, "")
	if err != nil || len(v.Findings) != 0 || v.Rules != 1 || v.Word() != model.Done {
		t.Fatalf("Validate = %+v, %v", v, err)
	}
}

// TestValidateListsAFindingWithItsFileAndLineAndEndsFindings: a notice beside
// it never changes the word, a finding always does.
func TestValidateListsAFindingWithItsFileAndLineAndEndsFindings(t *testing.T) {
	t.Parallel()
	files := goodBook()
	files["rulebook.yaml"] = "format: 3\nname: a\nname: b\n"
	w, root := validateWorld(t, files)
	v, err := coreapi.Validate(context.Background(), w.Ports(), root, "")
	if err != nil || len(v.Findings) != 1 || v.Findings[0].Entity != "rulebook.yaml:3" || v.Word() != model.Findings {
		t.Fatalf("Validate = %+v, %v", v, err)
	}
	if len(v.Notices) == 0 {
		t.Error("the names check that did not run is a notice, and none was listed")
	}
}

// TestValidateEndsNotReadyOnAnOpenRunNamingJournalRecover: a run that never
// closed is refused before any file is read. Planted failure: Validate not
// asking the journal.
func TestValidateEndsNotReadyOnAnOpenRunNamingJournalRecover(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, goodBook())
	j.w.SeedSchemas(validateSchemas()...)
	j.copied("run-open", "rulebook.yaml", "x", true, now)
	_, err := coreapi.Validate(context.Background(), j.ports, j.root, "")
	if !coreapi.IsOpenRecord(err) || !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "run-open") {
		t.Fatalf("err = %v; want the open record, not ready, naming the run", err)
	}
	j.closed("run-open", "rolled back", "", "", "", now+1)
	if _, err := coreapi.Validate(context.Background(), j.ports, j.root, ""); err != nil {
		t.Errorf("a closed run still refuses: %v", err)
	}
}

// TestValidateOfANewerFormatEndsNotReadyNamingBothVersions.
func TestValidateOfANewerFormatEndsNotReadyNamingBothVersions(t *testing.T) {
	t.Parallel()
	files := goodBook()
	files["rulebook.yaml"] = strings.Replace(bookIdentity, "format: 3", "format: 4", 1)
	w, root := validateWorld(t, files)
	_, err := coreapi.Validate(context.Background(), w.Ports(), root, "")
	if !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "format 4") || !strings.Contains(err.Error(), "format 3") {
		t.Fatalf("err = %v; want not ready naming both versions", err)
	}
}

// TestAValidationDocNeverCutsAListSilently: 200 findings show Limit of them,
// and the document carries the total, the shown count and the contract.
func TestAValidationDocNeverCutsAListSilently(t *testing.T) {
	t.Parallel()
	v := model.Validation{Rules: 3}
	for range 200 {
		v.Findings = append(v.Findings, model.Finding{Entity: "a.yaml:1", Msg: "m"})
	}
	d := coreapi.ValidationDoc(v, 50, "")
	if d.Total != 200 || d.Shown != 50 || len(d.Findings) != 50 || d.Word != "findings" || d.Contract != model.ContractVersion || d.Notices == nil {
		t.Errorf("doc = %+v", d)
	}
}
