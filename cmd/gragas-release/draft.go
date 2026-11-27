package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// release is what draftStep reads of one release in GitHub's listing of the
// repository's releases, as gh api --paginate prints it: one array a page.
type release struct {
	ID      int64   `json:"id"`
	TagName string  `json:"tag_name"`
	Draft   bool    `json:"draft"`
	Assets  []asset `json:"assets"`
}

// asset is one file of a release: its name and the digest GitHub gives it,
// sha256:<hex>.
type asset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// draftStep names the one draft release of tag that holds the bytes verify
// read, and nothing else: its assets are the archives of the checksum file
// at checksums, each with its digest, and that file with its own, both ways.
// It answers the draft's id on stdout, so the release is published by its
// id, never by its tag. No draft, two drafts, an asset missing, an asset
// over, or a digest that differs or is not given each leave it a draft.
func draftStep(listing, tag, checksums string, out *say.Speaker) error {
	raw, err := os.ReadFile(listing)
	if err != nil {
		return err
	}
	all, err := readReleases(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", listing, err)
	}
	file, err := os.ReadFile(checksums)
	if err != nil {
		return err
	}
	sums, _, err := readSums(file)
	if err != nil {
		return fmt.Errorf("%s: %w", checksums, err)
	}
	want := maps.Clone(sums)
	name := filepath.Base(checksums)
	if _, listed := want[name]; listed {
		return fmt.Errorf("%s lists itself", checksums)
	}
	want[name] = sum(file)
	d, err := theDraft(all, tag)
	if err != nil {
		return err
	}
	if err := holds(d, want); err != nil {
		return fmt.Errorf("the draft of %s, id %d, stays a draft: %w", tag, d.ID, err)
	}
	out.Text(strconv.FormatInt(d.ID, 10) + "\n")
	return nil
}

// readReleases reads every page of the listing; a listing of no page, or a
// page that is not an array of releases, is refused.
func readReleases(raw []byte) ([]release, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var all []release
	pages := 0
	for {
		var page []release
		err := dec.Decode(&page)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("not a listing of releases: %w", err)
		}
		pages++
		all = append(all, page...)
	}
	if pages == 0 {
		return nil, errors.New("not a listing of releases: it is empty")
	}
	return all, nil
}

// theDraft is the one draft release whose tag is tag.
func theDraft(all []release, tag string) (release, error) {
	var drafts []release
	for _, r := range all {
		if r.Draft && r.TagName == tag {
			drafts = append(drafts, r)
		}
	}
	switch len(drafts) {
	case 0:
		return release{}, fmt.Errorf("the repository holds no draft of %s, so nothing is published", tag)
	case 1:
		return drafts[0], nil
	default:
		ids := make([]string, len(drafts))
		for i, r := range drafts {
			ids[i] = strconv.FormatInt(r.ID, 10)
		}
		return release{}, fmt.Errorf("the repository holds %d drafts of %s (ids %s), and only one is what verify read; delete the stale ones, then run the release again", len(drafts), tag, strings.Join(ids, ", "))
	}
}

// holds answers the first asset, in name order, on which the draft and want
// differ: one the draft lacks, one it has over, or one whose digest differs
// or is not given.
func holds(d release, want map[string]string) error {
	have := map[string]string{}
	for _, a := range d.Assets {
		if _, twice := have[a.Name]; twice {
			return fmt.Errorf("it holds %s twice", a.Name)
		}
		have[a.Name] = a.Digest
	}
	both := maps.Clone(want)
	maps.Copy(both, have)
	for _, n := range slices.Sorted(maps.Keys(both)) {
		w, inWant := want[n]
		h, inHave := have[n]
		switch {
		case !inHave:
			return fmt.Errorf("it lacks %s, which the build made", n)
		case !inWant:
			return fmt.Errorf("it holds %s, which the build did not make", n)
		case !strings.HasPrefix(h, "sha256:"):
			return fmt.Errorf("GitHub gives %s no sha256 digest (%q), so its bytes are not known to be the ones verify read", n, h)
		case strings.TrimPrefix(h, "sha256:") != w:
			return fmt.Errorf("its %s is %s, and the build made %s", n, strings.TrimPrefix(h, "sha256:"), w)
		}
	}
	return nil
}
