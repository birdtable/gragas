// Package pluginbundle is the one digest of a plugin bundle and the one
// judgement of it as port.Bundles read it: host/plugin's Open and host/fs's
// add both call Judge, and neither keeps a copy. It reads bytes
// and runs nothing; whether the engine has the features the manifest needs,
// whether the engine's judge allows the manifest, whether its digest is
// approved and whether the module imports only WASI preview 1 are its
// callers' to ask.
package pluginbundle

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// contract is the one contract number the manifest's type reads.
const contract = 1

// The two files of a bundle, by their path below its folder.
const (
	// ManifestFile is the bundle's manifest.
	ManifestFile = "plugin.yaml"
	// ModuleFile is the bundle's module.
	ModuleFile = "plugin.wasm"
)

// MaxManifest is the most bytes a plugin.yaml may hold: Judge refuses one
// over it before the bundle is summed or a byte of it parsed, for both of its
// callers, as a reader's maker bounds the module.
const MaxManifest = 64 << 10

// MaxBundle is the most bytes all the files of a bundle may hold together:
// Judge refuses a bundle over it before any file is parsed, so the guarantee
// does not depend on the adapter that read or wrote it, and the reader's own
// total is held to it.
const MaxBundle = 64 << 20

// MaxFiles is the most files a bundle may hold, plugin.yaml and plugin.wasm
// among them: Judge refuses a bundle of more before it compares a name or
// sums a byte, so the compare of every two names stays bounded.
const MaxFiles = 1024

// Digest is the digest of a bundle's folder, the one spelling the
// reader, the lock's writer and Judge reach: each file in the order of its
// path as bytes, written as the length of its path, a colon, its path, the
// length of its bytes, a colon, the sum of its bytes (digest.Hex) and a
// newline, and the whole summed. A path and a length are each framed by a
// length or a colon, and every sum is sixty-four digits, so two sets of
// files whose bytes run together alike sum apart. It covers every file it is
// handed: the grants, in plugin.yaml, the module, and every other file of the
// folder, testdata/ among them. It judges no path: Judge does, before it sums.
func Digest(files []model.File) model.Digest {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
	var sb strings.Builder
	for _, f := range sorted {
		sb.WriteString(strconv.Itoa(len(f.Path)))
		sb.WriteByte(':')
		sb.WriteString(f.Path)
		sb.WriteString(strconv.Itoa(len(f.Data)))
		sb.WriteByte(':')
		sb.WriteString(digest.Hex(f.Data))
		sb.WriteByte('\n')
	}
	return model.Digest(digest.Prefixed([]byte(sb.String())))
}

// Judge holds b, the bundle the lock names under ref, to what an installed
// bundle is, and answers its manifest. It trusts no digest it is handed: it
// sums the bytes itself, so a reader
// that reports a digest of other bytes is refused here, once, for every
// reader. The sum covers plugin.yaml, plugin.wasm and every other file of
// b, so a file added to the folder after its lock sums to another digest and
// is refused. A link, a folder and every file that is not regular are the
// reader's to refuse, since b holds bytes alone (port.Bundles). It asks in
// this order, and the first that fails ends it:
//
//  1. the lock's digest is formed (model.Digest.Formed), else
//     port.ErrDigest;
//  2. every bound before the sum: the bundle holds no more than MaxFiles
//     files and MaxBundle bytes, and plugin.yaml no more than MaxManifest
//     bytes, else
//     port.ErrDigest and port.ErrManifest;
//  3. every path of the bundle is relative and slash-separated, with no
//     empty, "." or ".." step and no backslash (fs.ValidPath), and no two
//     clash: one path twice, a file and a folder of one name, or two
//     names that are one (model.SameName), as a second plugin.yaml in
//     another case is, else port.ErrDigest;
//  4. the digest of its files, summed here, is the reader's and the lock's,
//     else port.ErrDigest: a reader whose digest is not of its bytes and a
//     bundle changed since it was locked are refused before any byte of it
//     is parsed;
//  5. plugin.yaml is there, else port.ErrManifest, and plugin.wasm is, else
//     port.ErrModule; a file of no bytes is not there, since neither file
//     of a bundle can be empty;
//  6. plugin.yaml is one strict YAML mapping (ordered.ParseStrict: no anchor,
//     alias, merge key, tag or key written twice) of values that have a JSON
//     form (no timestamp, no NaN, no infinity), else port.ErrManifest;
//  7. its contract, where it is a whole number, is the engine's, else
//     port.ErrContract;
//  8. it reads as the manifest's type, which refuses a key it does not know
//     and every other thing its schema does not allow, else port.ErrManifest;
//  9. its name and version are the ref's, else port.ErrManifest;
//  10. its wasm is the module's digest, else port.ErrDigest.
//
// Every refusal matches its sentinel with errors.Is and says what differs.
// The text it quotes from the lock or the bundle is written by plugin.Shown,
// so no refusal paints the screen it reaches.
func Judge(ref model.PluginRef, b model.PluginBundle) (plugin.Manifest, error) {
	who := plugin.Shown(ref.Name) + " " + plugin.Shown(ref.Version)
	if !b.Locked.Formed() {
		return plugin.Manifest{}, fmt.Errorf("%w: the lock of %s holds %s, which is not a digest", port.ErrDigest, who, shownOr(string(b.Locked), "nothing"))
	}
	files, err := filesOf(who, b)
	if err != nil {
		return plugin.Manifest{}, err
	}
	sum := Digest(files)
	if b.Read != sum {
		return plugin.Manifest{}, fmt.Errorf("%w: the reader says the bundle of %s sums to %s, and its bytes sum to %s", port.ErrDigest, who, shownOr(string(b.Read), "nothing"), sum)
	}
	if sum != b.Locked {
		return plugin.Manifest{}, fmt.Errorf("%w: %s reads as %s, and its lock holds %s", port.ErrDigest, who, sum, b.Locked)
	}
	if len(b.Manifest) == 0 {
		return plugin.Manifest{}, fmt.Errorf("%w: the bundle of %s holds no %s, or an empty one", port.ErrManifest, who, ManifestFile)
	}
	if len(b.Module) == 0 {
		return plugin.Manifest{}, fmt.Errorf("%w: the bundle of %s holds no %s, or an empty one", port.ErrModule, who, ModuleFile)
	}
	o, err := readObject(b.Manifest)
	if err != nil {
		return plugin.Manifest{}, fmt.Errorf("%w: %s of %s %w", port.ErrManifest, ManifestFile, who, err)
	}
	if n, ok := o["contract"].(canon.Int); ok && n != contract {
		return plugin.Manifest{}, fmt.Errorf("%w: %s of %s is of contract %d, and the engine's is %d", port.ErrContract, ManifestFile, who, n, contract)
	}
	var m plugin.Manifest
	if err := m.FromCanon(o); err != nil {
		return plugin.Manifest{}, fmt.Errorf("%w: %s of %s: %w", port.ErrManifest, ManifestFile, who, err)
	}
	if m.Name != ref.Name || m.Version != ref.Version {
		return plugin.Manifest{}, fmt.Errorf("%w: %s names %s %s, and the lock installs it as %s", port.ErrManifest, ManifestFile, plugin.Shown(m.Name), plugin.Shown(m.Version), who)
	}
	if got := digest.Prefixed(b.Module); m.Wasm != got {
		return plugin.Manifest{}, fmt.Errorf("%w: %s of %s names the module's digest as %s, and the module's is %s", port.ErrDigest, ManifestFile, who, plugin.Shown(m.Wasm), got)
	}
	return m, nil
}

// filesOf is every file of b, its two and the others, once each bound and
// path of the bundle holds (steps 2 and 3 of Judge), before a byte is summed.
func filesOf(who string, b model.PluginBundle) ([]model.File, error) {
	if n := 2 + len(b.Other); n > MaxFiles {
		return nil, fmt.Errorf("%w: the bundle of %s holds %d files, over the %d a bundle may hold", port.ErrDigest, who, n, MaxFiles)
	}
	if len(b.Manifest) > MaxManifest {
		return nil, fmt.Errorf("%w: %s of %s is %d bytes, over the %d a manifest may hold", port.ErrManifest, ManifestFile, who, len(b.Manifest), MaxManifest)
	}
	var total int
	total += len(b.Manifest) + len(b.Module)
	for _, f := range b.Other {
		total += len(f.Data)
	}
	if total > MaxBundle {
		return nil, fmt.Errorf("%w: the bundle of %s holds %d bytes, over the %d a bundle may hold", port.ErrDigest, who, total, MaxBundle)
	}
	files := append([]model.File{{Path: ManifestFile, Data: b.Manifest}, {Path: ModuleFile, Data: b.Module}}, b.Other...)
	for i, f := range files {
		if !cleanPath(f.Path) {
			return nil, fmt.Errorf("%w: the bundle of %s holds %s, which is not a clean path below its folder", port.ErrDigest, who, shownOr(f.Path, "a file of no path"))
		}
		for _, g := range files[:i] {
			if clash(f.Path, g.Path) {
				return nil, fmt.Errorf("%w: the bundle of %s holds %s and %s, which one folder cannot both hold", port.ErrDigest, who, plugin.Shown(g.Path), plugin.Shown(f.Path))
			}
		}
	}
	return files, nil
}

// cleanPath says whether p names a file below a folder: relative and
// slash-separated, with no empty, "." or ".." step (fs.ValidPath), not the
// folder itself, and with no backslash, which one system reads as a step.
func cleanPath(p string) bool {
	return p != "." && fs.ValidPath(p) && !strings.Contains(p, `\`)
}

// clash says whether a and b cannot both be files of one folder: they are
// one path, or one names as a file a folder the other is in, or at the first
// step where they differ the two names are one (model.SameName), as on a
// system whose names differ only in case, so "A/x" and "a/y" clash.
func clash(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := range min(len(as), len(bs)) {
		if as[i] != bs[i] {
			return model.SameName(as[i], bs[i])
		}
	}
	return true
}

// shownOr is s as plugin.Shown writes it, or none where s is empty.
func shownOr(s, none string) string {
	if s == "" {
		return none
	}
	return plugin.Shown(s)
}

// readObject is data read as one strict YAML mapping, in canon's values.
func readObject(data []byte) (canon.Object, error) {
	doc, err := ordered.ParseStrict(data)
	if err != nil {
		return nil, fmt.Errorf("is not one plain YAML document: %s", plugin.Shown(err.Error()))
	}
	v, err := toCanon(doc)
	if err != nil {
		return nil, err
	}
	o, ok := v.(canon.Object)
	if !ok {
		return nil, errors.New("is not a mapping")
	}
	return o, nil
}

// toCanon turns the plain YAML value into canon's: a value of no JSON form, a
// timestamp, a NaN and an infinity among them, is refused.
func toCanon(v any) (canon.Value, error) {
	switch x := v.(type) {
	case nil:
		return canon.Null{}, nil
	case bool:
		return canon.Bool(x), nil
	case int64:
		return canon.Int(x), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("holds a number with no JSON form (%v)", x)
		}
		return canon.Float(x), nil
	case string:
		return canon.String(x), nil
	case []any:
		a := make(canon.Array, len(x))
		for i, e := range x {
			c, err := toCanon(e)
			if err != nil {
				return nil, err
			}
			a[i] = c
		}
		return a, nil
	case *ordered.Map:
		o := make(canon.Object, x.Len())
		for _, k := range x.Keys() {
			c, err := toCanon(x.Val(k))
			if err != nil {
				return nil, err
			}
			o[k] = c
		}
		return o, nil
	}
	return nil, fmt.Errorf("holds a value with no JSON form (%T)", v)
}
