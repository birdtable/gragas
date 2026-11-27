package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/ordered"
)

// toolsName is the repository's file of pinned tools, and goreleaserID the row
// of goreleaser in it.
const (
	toolsName    = "tools.yaml"
	goreleaserID = "goreleaser"
)

// sumForm is a sha256 as tools.yaml writes one: 64 lower-case hex digits.
var sumForm = regexp.MustCompile(`^[0-9a-f]{64}$`)

// pinnedSum is the sha256 the tools file's text pins for the program of the
// release tool id on host: the sha256 key of its platforms' os/arch entry, the
// program inside the archive, as task tools placed it. A row that is missing,
// written twice, not a release tool, or with no sha256 for host is refused.
func pinnedSum(pins []byte, id string, host system) (string, error) {
	v, err := ordered.ParseYAML(pins)
	if err != nil {
		return "", fmt.Errorf("%s does not parse: %w", toolsName, err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok || doc == nil {
		return "", fmt.Errorf("%s is not a mapping", toolsName)
	}
	var rows []*ordered.Map
	for _, m := range ordered.Maps(doc.List("tools")) {
		if m.Str("id") == id {
			rows = append(rows, m)
		}
	}
	switch {
	case len(rows) == 0:
		return "", fmt.Errorf("%s has no row for %s", toolsName, id)
	case len(rows) > 1:
		return "", fmt.Errorf("%s has %d rows for %s, and a pin is one", toolsName, len(rows), id)
	case rows[0].Str("from") != "release":
		return "", fmt.Errorf("%s's row for %s is not a release tool, so it pins no program by sha256", toolsName, id)
	}
	platform := host.OS + "/" + host.Arch
	var sum string
	if p := rows[0].Sub("platforms").Sub(platform); p != nil {
		sum = p.Str("sha256")
	}
	if !sumForm.MatchString(sum) {
		return "", fmt.Errorf("%s pins no sha256 of %s for %s (it has %q)", toolsName, id, platform, sum)
	}
	return sum, nil
}

// heldGoreleaser refuses req's goreleaser unless it is an absolute path to the
// program tools.yaml pins for the host: its sha256, summed by internal/digest,
// is the pin. buildStep runs it before it empties a folder, clones or starts
// anything, so a program that is not the pin never runs and is never handed
// GITHUB_TOKEN. It holds the file as it
// is when summed: one that is swapped after it is not caught here, and the
// folder it lives in, .tools/bin, is the person's own.
func heldGoreleaser(ctx context.Context, tc toolchain, req buildRequest) error {
	if !filepath.IsAbs(req.goreleaser) {
		return fmt.Errorf("-goreleaser %q is not an absolute path, so it is not started", req.goreleaser)
	}
	host, err := tc.Host(ctx)
	if err != nil {
		return err
	}
	pins, err := os.ReadFile(filepath.Join(req.repo, toolsName))
	if err != nil {
		return fmt.Errorf("the pins of the release tools: %w", err)
	}
	want, err := pinnedSum(pins, goreleaserID, host)
	if err != nil {
		return err
	}
	got, err := fileSum(req.goreleaser)
	if err != nil {
		return fmt.Errorf("goreleaser %s: %w", req.goreleaser, err)
	}
	if got != want {
		return fmt.Errorf("goreleaser %s has sha256 %s and %s pins %s for %s/%s: it is not the pinned goreleaser, so nothing is built (run task tools)", req.goreleaser, got, toolsName, want, host.OS, host.Arch)
	}
	return nil
}

// fileSum is the sha256 of the file at path, in hex, read through.
func fileSum(path string) (sum string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	sum, _, err = digest.Reader(f)
	return sum, err
}
