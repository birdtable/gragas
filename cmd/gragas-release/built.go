package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// artifactsName is the file goreleaser writes beside the archives naming
// everything it made, each built binary with the path it was built to.
const artifactsName = "artifacts.json"

// artifact is one entry of artifacts.json, as much of it as the unpack step
// reads.
type artifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Type   string `json:"type"`
}

// builtBinaries reads artifacts.json and answers, for each system, the path
// of the binary goreleaser built for it. A system with no binary, or with
// two, is an error naming it, and so is a path that is not inside the
// release folder: the step reads the built binary from the build's own
// folder and never from anything an archive holds.
func builtBinaries(req request) (map[system]string, error) {
	raw, err := os.ReadFile(filepath.Join(req.dist, artifactsName))
	if err != nil {
		return nil, fmt.Errorf("the build's record of what it built could not be read, so no unpacked binary can be held to the built one: %w", err)
	}
	dist, err := filepath.Abs(req.dist)
	if err != nil {
		return nil, err
	}
	return readBuilt(raw, dist, req.repo, req.systems)
}

// readBuilt reads artifacts.json's bytes: for each of systems, the absolute
// path of its one built binary, inside dist, which is absolute; paths in the
// file are from repo. It answers the paths or an error, never both.
func readBuilt(raw []byte, dist, repo string, systems []system) (map[system]string, error) {
	var all []artifact
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("%s: %w", artifactsName, err)
	}
	want := map[system]bool{}
	for _, s := range systems {
		want[s] = true
	}
	out := map[system]string{}
	for _, a := range all {
		s := system{OS: a.GOOS, Arch: a.GOARCH}
		if a.Type != "Binary" || !want[s] {
			continue
		}
		if _, twice := out[s]; twice {
			return nil, fmt.Errorf("%s names two binaries built for %s, so the unpacked one has no one binary to be held to", artifactsName, s)
		}
		p, err := inside(dist, repo, a.Path)
		if err != nil {
			return nil, fmt.Errorf("%s: the binary built for %s: %w", artifactsName, s, err)
		}
		out[s] = p
	}
	for _, s := range systems {
		if _, ok := out[s]; !ok {
			return nil, fmt.Errorf("%s names no binary built for %s, so its archive's binary cannot be held to the built one", artifactsName, s)
		}
	}
	return out, nil
}

// inside is rel, a path goreleaser wrote relative to the project folder repo,
// as an absolute path, when it names a file inside dist; anything else is an
// error.
func inside(dist, repo, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return "", fmt.Errorf("the path %q is not a relative path", rel)
	}
	p, err := filepath.Abs(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(dist, p)
	if err != nil || r == "." || !filepath.IsLocal(r) {
		return "", fmt.Errorf("%s is not inside the release folder %s", rel, dist)
	}
	return p, nil
}

// heldToBuilt holds the bytes an archive's binary unpacked to against the
// binary goreleaser built at path, by digest.
func heldToBuilt(unpacked []byte, path string) error {
	built, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the built binary %s is gone, so the unpacked one cannot be held to it", path)
	}
	if err != nil {
		return err
	}
	if got, want := sum(unpacked), sum(built); got != want {
		return fmt.Errorf("the binary it unpacks to is not the one built: unpacked %s, built %s (%s)", got, want, path)
	}
	return nil
}
