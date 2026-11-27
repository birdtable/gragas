package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/exit"
)

// built is the checksum file of a build of two archives, and the assets a
// draft that holds exactly that build has.
func built(t *testing.T) (string, []asset) {
	t.Helper()
	archives := map[string][]byte{"gragas_1.2.3_linux_amd64.tar.gz": []byte("linux"), "gragas_1.2.3_darwin_arm64.tar.gz": []byte("darwin")}
	path := writeSums(t, archives)
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assets := []asset{{Name: sumsName, Digest: "sha256:" + sum(file)}}
	for n, b := range archives {
		assets = append(assets, asset{Name: n, Digest: "sha256:" + sum(b)})
	}
	return path, assets
}

// listing writes pages, as gh api --paginate prints them, one array a page,
// each release with a field draftStep does not read, as GitHub's have.
// Every name and digest a test gives is plain ASCII, so %q quotes it as JSON
// does.
func listing(t *testing.T, pages ...[]release) string {
	t.Helper()
	var b strings.Builder
	for _, p := range pages {
		rs := make([]string, len(p))
		for i, r := range p {
			as := make([]string, len(r.Assets))
			for j, a := range r.Assets {
				as[j] = fmt.Sprintf(`{"name": %q, "digest": %q, "size": 1}`, a.Name, a.Digest)
			}
			rs[i] = fmt.Sprintf(`{"id": %d, "tag_name": %q, "draft": %t, "name": "x", "assets": [%s]}`, r.ID, r.TagName, r.Draft, strings.Join(as, ", "))
		}
		b.WriteString("[" + strings.Join(rs, ", ") + "]\n")
	}
	path := filepath.Join(t.TempDir(), "releases.json")
	write(t, path, b.String())
	return path
}

func runDraft(t *testing.T, list, sums string) (int, string, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := run(t.Context(), []string{"draft", list, testTag, sums}, &out, &errs, goodChain())
	return code, out.String(), errs.String()
}

// The one draft of the tag whose assets are the build's archives and checksum
// file, each by its sha256, is named by its id, from any page of the listing;
// a published release of the tag and a draft of another tag are not it.
func TestTheDraftThatHoldsTheBuildIsNamedByItsID(t *testing.T) {
	t.Parallel()
	sums, assets := built(t)
	list := listing(t,
		[]release{{ID: 1, TagName: "v1.2.2", Assets: assets}, {ID: 2, TagName: "v1.2.4", Draft: true, Assets: assets}},
		[]release{{ID: 3, TagName: testTag, Assets: assets}, {ID: 42, TagName: testTag, Draft: true, Assets: assets}},
	)
	if code, out, errs := runDraft(t, list, sums); code != exit.Done || out != "42\n" || errs != "" {
		t.Errorf("draft = %d, %q, %q; want the id 42 alone", code, out, errs)
	}
}

// A draft whose assets are not the bytes verify read stays a draft, each plant
// named: an archive whose digest differs, the checksum file's own digest
// differing, an archive missing, an asset over, no digest, a digest that is
// not sha256, an asset twice; and no draft, or two drafts of the tag.
// Risk: a release published with archives nobody checked, because a stale or
// tampered draft held as many assets as the build made.
func TestADraftThatIsNotTheBuildStaysADraft(t *testing.T) {
	t.Parallel()
	sums, good := built(t)
	with := func(change func([]asset) []asset) []release {
		return []release{{ID: 7, TagName: testTag, Draft: true, Assets: change(append([]asset(nil), good...))}}
	}
	at := func(as []asset, name string) int {
		for i, a := range as {
			if a.Name == name {
				return i
			}
		}
		t.Fatalf("no asset %s", name)
		return -1
	}
	linux := "gragas_1.2.3_linux_amd64.tar.gz"
	for name, c := range map[string]struct {
		releases []release
		says     string
	}{
		"an archive of other bytes": {with(func(as []asset) []asset {
			as[at(as, linux)].Digest = "sha256:" + sum([]byte("other"))
			return as
		}), "its " + linux + " is " + sum([]byte("other")) + ", and the build made " + sum([]byte("linux"))},
		"a checksum file of other bytes": {with(func(as []asset) []asset {
			as[at(as, sumsName)].Digest = "sha256:" + sum([]byte("stale"))
			return as
		}), "its " + sumsName + " is " + sum([]byte("stale"))},
		"an archive missing": {with(func(as []asset) []asset {
			return append(as[:at(as, linux)], as[at(as, linux)+1:]...)
		}), "it lacks " + linux + ", which the build made"},
		"an asset over, at the same count": {with(func(as []asset) []asset {
			as[at(as, linux)] = asset{Name: "gragas_1.2.3_plan9_amd64.tar.gz", Digest: "sha256:" + sum([]byte("linux"))}
			return as
		}), "it lacks " + linux + ", which the build made"},
		"an asset over": {with(func(as []asset) []asset {
			return append(as, asset{Name: "gragas_1.2.3_plan9_amd64.tar.gz", Digest: "sha256:" + sum([]byte("plan9"))})
		}), "it holds gragas_1.2.3_plan9_amd64.tar.gz, which the build did not make"},
		"no digest given": {with(func(as []asset) []asset {
			as[at(as, linux)].Digest = ""
			return as
		}), "GitHub gives " + linux + " no sha256 digest"},
		"a digest not sha256": {with(func(as []asset) []asset {
			as[at(as, linux)].Digest = "sha512:" + sum([]byte("linux"))
			return as
		}), "GitHub gives " + linux + " no sha256 digest"},
		"an asset twice": {with(func(as []asset) []asset {
			return append(as, as[at(as, linux)])
		}), "it holds " + linux + " twice"},
		"no draft of the tag": {[]release{{ID: 7, TagName: testTag, Assets: good}}, "the repository holds no draft of " + testTag},
		"two drafts of the tag": {[]release{{ID: 7, TagName: testTag, Draft: true, Assets: good}, {ID: 8, TagName: testTag, Draft: true, Assets: good}},
			"the repository holds 2 drafts of " + testTag + " (ids 7, 8)"},
	} {
		code, out, errs := runDraft(t, listing(t, c.releases), sums)
		if code != exit.Failed || out != "" || !strings.Contains(errs, c.says) {
			t.Errorf("%s: draft = %d, %q, %q; want it refused, saying %q", name, code, out, errs, c.says)
		}
	}
}

// A listing that is not one, or is empty, and a checksum file that lists
// itself, are refused before any draft is named.
func TestTheDraftStepRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	sums, good := built(t)
	empty := filepath.Join(t.TempDir(), "releases.json")
	write(t, empty, "")
	object := filepath.Join(t.TempDir(), "releases.json")
	write(t, object, `{"id": 1}`)
	listsItself := writeSums(t, map[string][]byte{sumsName: []byte("x")})
	for name, c := range map[string]struct{ list, sums, says string }{
		"an empty listing":               {empty, sums, "not a listing of releases: it is empty"},
		"a release, not a page":          {object, sums, "not a listing of releases"},
		"a checksum file listing itself": {listing(t, []release{{ID: 1, TagName: testTag, Draft: true, Assets: good}}), listsItself, "lists itself"},
	} {
		if code, out, errs := runDraft(t, c.list, c.sums); code != exit.Failed || out != "" || !strings.Contains(errs, c.says) {
			t.Errorf("%s: draft = %d, %q, %q; want it refused, saying %q", name, code, out, errs, c.says)
		}
	}
}
