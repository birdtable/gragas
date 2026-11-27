package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestAGuideFolderOverTheCapIsRefusedAndOneAtItIsRead holds the cap on the
// Markdown files of a guide's folder: 256 are read, 257 are refused with the
// count and the cap in the sentence.
// boundary: maxGuideFiles
// Risk: a folder of another repository's read without a bound.
func TestAGuideFolderOverTheCapIsRefusedAndOneAtItIsRead(t *testing.T) {
	t.Parallel()
	for _, n := range []int{256, 257} {
		w, _ := ruleWorld(t)
		files := map[string]string{
			"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
			"book/language/go/rules/errors.yaml": bookRules, "book/language/go/guide.md": ruleGuide,
		}
		for i := range n {
			files[fmt.Sprintf("guide/g%03d.md", i)] = "# T\n\nIt MUST work.\n"
		}
		root := w.Seed("repo", files)
		_, _, err := coreapi.Import(context.Background(), w.Ports(), changeAsk(root, "import"),
			model.ImportInput{Part: "go", Concern: "errors", GuideDir: "guide",
				Guide: model.GuideRef{Slug: "g", Title: "G", URL: "https://example.org/g", Tag: "v1.0.0"}})
		over := err != nil && strings.Contains(err.Error(), "over the 256 read")
		if over != (n == 257) {
			t.Errorf("%d files: err = %v, want the cap's refusal only at 257", n, err)
		}
	}
}

// TestAnImportOfAConfigWithNoPluginIsRefusedNamingTheTool: with no
// plugin installed for the tool no adapter answers the read, and the refusal
// says which tool and how to install one.
// Risk: a config read by a built-in adapter release 1 does not ship.
func TestAnImportOfAConfigWithNoPluginIsRefusedNamingTheTool(t *testing.T) {
	t.Parallel()
	w, _ := ruleWorld(t)
	root := w.Seed("repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": bookRules, "book/language/go/guide.md": ruleGuide,
		".golangci.yml": "version: \"2\"\n",
	})
	_, _, err := coreapi.Import(context.Background(), w.Ports(), changeAsk(root, "import"),
		model.ImportInput{Part: "go", Concern: "errors", Tool: "golangci-lint", Target: model.TargetConfig, Configs: []string{".golangci.yml"}})
	if !errors.Is(err, coreapi.ErrNoPlugin) || !strings.Contains(err.Error(), "golangci-lint") || !strings.Contains(err.Error(), "gragas plugin add") {
		t.Errorf("err = %v, want ErrNoPlugin naming golangci-lint and how to install one", err)
	}
}
