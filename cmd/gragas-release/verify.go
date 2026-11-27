package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	whole "github.com/birdtable/go-files/write"
	"github.com/birdtable/go-proc/proc"
	"github.com/birdtable/gragas/internal/dev/say"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// sumsName is the checksum file goreleaser writes beside the archives
// (.goreleaser.yaml, checksum.name_template).
const sumsName = "checksums.txt"

// carried are the files every archive holds beside the binary, each the same
// bytes as the tree's (supply.licences.6): the licence, the notice, and one
// notice per linked module, from go-licenses' report.
func carried() []string { return []string{"LICENSE", "NOTICE", "notices/gragas.txt"} }

// readSums reads a checksum file: one line per archive, a digest, two spaces
// and a name that stays in its folder, no name twice. It answers the digests
// by name and the names in the file's order.
func readSums(raw []byte) (map[string]string, []string, error) {
	sums, names := map[string]string{}, []string{}
	text := strings.TrimSuffix(string(raw), "\n")
	if text == "" {
		return nil, nil, errors.New("it lists no archive")
	}
	for l := range strings.SplitSeq(text, "\n") {
		d, name, ok := strings.Cut(l, "  ")
		if !ok || len(d) != 64 || strings.Trim(d, "0123456789abcdef") != "" || !safeName(name) || strings.Contains(name, "/") {
			return nil, nil, fmt.Errorf("%w: %q", errNoLine, l)
		}
		if _, twice := sums[name]; twice {
			return nil, nil, fmt.Errorf("it lists %s twice", name)
		}
		sums[name] = d
		names = append(names, name)
	}
	return sums, names, nil
}

// archiveName is goreleaser's name for the archive of s at the tag, from
// .goreleaser.yaml's name_template: the version is the tag without its v.
func archiveName(tag string, s system) string {
	return folderName(tag, s) + ".tar.gz"
}

// folderName is the one folder an archive unpacks into (wrap_in_directory).
func folderName(tag string, s system) string {
	return "gragas_" + strings.TrimPrefix(tag, "v") + "_" + s.OS + "_" + s.Arch
}

// verifyStep is the unpack step (supply.release.6). It reads the checksum file
// back and holds it to the folder both ways: a line for the archive of each
// system, and a line for every .tar.gz the folder holds, so nothing ships
// unchecked and no system drops out unseen. It holds each archive to its line,
// unpacks it, holds what it carries, and holds the binary it unpacks to
// against the binary goreleaser built for that system, read from the build's
// folder as artifacts.json names it and never from the archive. For the
// archive of the host it also runs the binary, in a folder of its own with its
// own home and temp folder, behind go-proc's block, and refused where the
// system gives none: version, which must print the tag, help, and
// contract fixture, which writes the fixture rulebook. An archive for another system is checked and not run.
// What it found goes in rec, pass or fail.
func verifyStep(ctx context.Context, tc toolchain, req request, out *say.Speaker, rec *record) error {
	raw, err := os.ReadFile(filepath.Join(req.dist, sumsName))
	if err != nil {
		return err
	}
	rec.Sums = sum(raw)
	sums, names, err := readSums(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", sumsName, err)
	}
	if err := holdsFolder(req, sums); err != nil {
		return err
	}
	built, err := builtBinaries(req)
	if err != nil {
		return err
	}
	want := map[string][]byte{}
	for _, f := range carried() {
		if want[f], err = os.ReadFile(filepath.Join(req.repo, filepath.FromSlash(f))); err != nil {
			return err
		}
	}
	host, err := tc.Host(ctx)
	if err != nil {
		return err
	}
	rec.Host = host.String()
	ran := false
	for _, s := range req.systems {
		name := archiveName(req.tag, s)
		bin, err := checkArchive(req, s, sums[name], want)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := heldToBuilt(bin.Data, built[s]); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		rec.Checked = append(rec.Checked, recordArchive{Name: name, System: s.String(), SHA256: sums[name], Binary: sum(bin.Data)})
		if s == host {
			rec.Network = tc.Network()
			if err := start(ctx, tc, req.tag, s, bin, rec); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			ran = true
		}
	}
	line := fmt.Sprintf("verify: %d archives hold to %s, each with its licence and notices, and each binary is the one built", len(names), sumsName)
	if ran {
		line += "; the " + host.String() + " binary printed " + req.tag + ", started and wrote the fixture, " + rec.Network
	} else {
		line += "; no archive is for this machine's " + host.String() + ", so none was run"
	}
	out.Text(line + "\n")
	return nil
}

// holdsFolder says whether the names the checksum file lists are the archives
// of the systems and the archives the folder holds, no more and no fewer.
func holdsFolder(req request, sums map[string]string) error {
	expected := map[string]bool{}
	for _, s := range req.systems {
		name := archiveName(req.tag, s)
		if _, ok := sums[name]; !ok {
			return fmt.Errorf("%s has no line for %s, the archive of %s", sumsName, name, s)
		}
		expected[name] = true
	}
	for name := range sums {
		if !expected[name] {
			return fmt.Errorf("%s lists %s, which is the archive of no system of the list", sumsName, name)
		}
	}
	held, err := filepath.Glob(filepath.Join(req.dist, "*.tar.gz"))
	if err != nil {
		return err
	}
	for _, p := range held {
		if name := filepath.Base(p); !expected[name] {
			return fmt.Errorf("%s is in the folder and %s does not list it", name, sumsName)
		}
	}
	return nil
}

// checkArchive holds one archive to its digest, unpacks it, and holds every
// entry to its one folder: the binary of its system, and each carried file the
// same bytes as the tree's. It answers the binary.
func checkArchive(req request, s system, digest string, want map[string][]byte) (entry, error) {
	data, err := os.ReadFile(filepath.Join(req.dist, archiveName(req.tag, s)))
	if err != nil {
		return entry{}, err
	}
	if got := sum(data); got != digest {
		return entry{}, fmt.Errorf("the bytes differ from the checksum line: %s says %s, the archive is %s", sumsName, digest, got)
	}
	entries, err := ReadArchive(data)
	if err != nil {
		return entry{}, err
	}
	folder, binName := folderName(req.tag, s)+"/", proc.FileName(s.OS, "gragas")
	held := map[string]entry{}
	for _, e := range entries {
		rel, ok := strings.CutPrefix(e.Name, folder)
		if !ok {
			return entry{}, fmt.Errorf("the archive holds %s, outside its folder %s", e.Name, folder)
		}
		held[rel] = e
	}
	for _, f := range carried() {
		e, ok := held[f]
		if !ok {
			return entry{}, fmt.Errorf("the archive holds no %s", f)
		}
		if !bytes.Equal(e.Data, want[f]) {
			return entry{}, fmt.Errorf("the archive's %s is not the tree's", f)
		}
	}
	bin, ok := held[binName]
	if !ok {
		return entry{}, fmt.Errorf("the archive holds no %s binary", binName)
	}
	if extra := slices.DeleteFunc(slices.Sorted(maps.Keys(held)), func(n string) bool {
		return n == binName || slices.Contains(carried(), n)
	}); len(extra) > 0 {
		return entry{}, fmt.Errorf("the archive holds %s, which a release does not carry", strings.Join(extra, ", "))
	}
	return bin, nil
}

// start writes the binary into a folder of its own, with the home and temp
// folders its runs are given inside it, and runs version, whose first line must name the
// tag; then help, which reads no project and so is the same start test
// everywhere; then contract fixture, which writes the fixture rulebook into the
// folder and must leave it there. Each test that passes is added to rec.
func start(ctx context.Context, tc toolchain, tag string, s system, bin entry, rec *record) (err error) {
	// host/trust's FreshFolder: fresh at 0700 in the account's
	// own temp folder, which no other account can write, and never swept.
	dir, remove, err := hosttrust.FreshFolder(nil, "gragas-release-run-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, remove()) }()
	// the start test's own folders, and the one contract fixture writes into
	if err := madeFolders(dir, append(startEnv(dir), startFixtureDir+"="+filepath.Join(dir, startFixtureDir))); err != nil {
		return err
	}
	name := runName(s.OS)
	if err := whole.File(filepath.Join(dir, name), bin.Data, whole.Runnable()); err != nil {
		return err
	}
	for _, t := range []startTest{startVersion, startHelp, startFixture} {
		said, err := tc.Start(ctx, dir, name, t)
		if err != nil {
			if said = strings.TrimSpace(said); said != "" {
				return fmt.Errorf("the %s binary did not start with %s: %w; it said: %s", s, t, err, clipped(said))
			}
			return fmt.Errorf("the %s binary did not start with %s: %w", s, t, err)
		}
		first, _, _ := strings.Cut(said, "\n")
		switch {
		case strings.TrimSpace(said) == "":
			return fmt.Errorf("the %s binary printed nothing for %s", s, t)
		case t == startVersion && !strings.HasPrefix(first, "gragas "+tag+" "):
			return fmt.Errorf("the %s binary's version says %q, not the tag %s", s, first, tag)
		case t == startFixture:
			if wrote, err := os.ReadDir(filepath.Join(dir, startFixtureDir)); err != nil || len(wrote) == 0 {
				return fmt.Errorf("the %s binary's %s ended well and wrote no fixture into %s", s, t, startFixtureDir)
			}
		}
		rec.Started = append(rec.Started, t.String())
	}
	return nil
}

// madeFolders makes, at 0700, each folder env names beneath root: every value
// that is an absolute path inside root. A value elsewhere, the system's own
// or not a path, is left alone.
func madeFolders(root string, env []string) error {
	for _, kv := range env {
		if _, v, _ := strings.Cut(kv, "="); beneath(root, v) {
			if err := os.MkdirAll(v, 0o700); err != nil {
				return err
			}
		}
	}
	return nil
}

// beneath says whether p is an absolute path in root or root itself.
func beneath(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && filepath.IsAbs(p) && filepath.IsLocal(rel)
}

// clipped is what a binary printed, cut to its last 400 bytes, which hold
// the line it ended on, for an error that names it.
func clipped(said string) string {
	const most = 400
	if len(said) <= most {
		return said
	}
	return "..." + said[len(said)-most:]
}
