package coreapi_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// listWorld is a repository holding files, with the schemas of a rulebook.
func listWorld(t *testing.T, files map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(validateSchemas()...)
	return w, w.Seed("repo", files)
}
