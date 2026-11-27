package pluginbundle_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// wasm is the bundle's module: any bytes, since Judge runs nothing.
func wasm() []byte { return []byte("\x00asm\x01\x00\x00\x00standin") }

// manifest is a manifest of standin 1.0.0 that Judge passes, with the lines a
// row replaces to plant one fault.
func manifest(t *testing.T, replace ...string) []byte {
	t.Helper()
	m := `contract: 1
needs: []
name: standin
version: 1.0.0
tool: standin
tool_versions: ">=1.0.0 <2.0.0"
targets: [source]
provides: [entries]
extends: false
warning: false
programs:
  - id: standin
    path: bin/standin
    version_argv: [--version]
    versions: ">=1.0.0 <2.0.0"
wasm: ` + digest.Prefixed(wasm()) + "\n"
	for i := 0; i+1 < len(replace); i += 2 {
		if !strings.Contains(m, replace[i]) {
			t.Fatalf("the plant %q is not in the manifest", replace[i])
		}
		m = strings.Replace(m, replace[i], replace[i+1], 1)
	}
	return []byte(m)
}

// standin is the ref the lock installs the bundle under.
func standin() model.PluginRef { return model.PluginRef{Name: "standin", Version: "1.0.0"} }

// locked is a bundle read as it was locked: the lock and the reader each
// hold the digest of its two files and the others.
func locked(manifest, module []byte, other ...model.File) model.PluginBundle {
	d := sumOf(manifest, module, other...)
	return model.PluginBundle{Locked: d, Read: d, Manifest: manifest, Module: module, Other: other}
}

// sumOf is the digest of a bundle of the two files and the others.
func sumOf(manifest, module []byte, other ...model.File) model.Digest {
	return pluginbundle.Digest(append([]model.File{{Path: "plugin.yaml", Data: manifest}, {Path: "plugin.wasm", Data: module}}, other...))
}

// testdata is the other files of a bundle that carries its fixtures.
func testdata() []model.File {
	return []model.File{{Path: "testdata/a/config.yml", Data: []byte("a: 1\n")}, {Path: "testdata/a/want.json", Data: []byte("{}\n")}}
}

// withOther is the bundle of standin 1.0.0 locked with one other file at path.
func withOther(t *testing.T, path ...string) model.PluginBundle {
	t.Helper()
	var other []model.File
	for _, p := range path {
		other = append(other, model.File{Path: p, Data: []byte("x")})
	}
	return locked(manifest(t), wasm(), other...)
}

// many is n other files, each by its own clean path.
func many(n int) []model.File {
	out := make([]model.File, n)
	for i := range out {
		out[i] = model.File{Path: "testdata/f" + strconv.Itoa(i), Data: []byte("x")}
	}
	return out
}

// padded is the manifest of standin 1.0.0 grown by a comment to n bytes.
func padded(t *testing.T, n int) []byte {
	t.Helper()
	m := manifest(t)
	return append(m, []byte("#"+strings.Repeat("x", n-len(m)-2)+"\n")...)
}

// TestABundleAsLockedIsJudgedWhole: a bundle read as it was locked, whose
// manifest is the ref's and names the module's digest, answers its manifest.
func TestABundleAsLockedIsJudgedWhole(t *testing.T) {
	t.Parallel()
	m, err := pluginbundle.Judge(standin(), locked(manifest(t), wasm()))
	if err != nil {
		t.Fatalf("Judge = %v, want the manifest", err)
	}
	if m.Name != "standin" || m.Version != "1.0.0" || m.Wasm != digest.Prefixed(wasm()) {
		t.Errorf("Judge's manifest = %s %s %s, want the bundle's", m.Name, m.Version, m.Wasm)
	}
}

// TestABundleWithOtherFilesLockedWithThemIsJudgedWhole: the files of a
// bundle beside its two, testdata/ among them, are summed into its digest,
// and a bundle locked with them opens.
func TestABundleWithOtherFilesLockedWithThemIsJudgedWhole(t *testing.T) {
	t.Parallel()
	if _, err := pluginbundle.Judge(standin(), locked(manifest(t), wasm(), testdata()...)); err != nil {
		t.Fatalf("Judge = %v, want the manifest", err)
	}
}

// TestEachFaultOfABundleIsRefusedWithItsSentinel: each row plants one fault,
// and Judge refuses it with its sentinel and says what differs.
func TestEachFaultOfABundleIsRefusedWithItsSentinel(t *testing.T) {
	t.Parallel()
	// A bundle changed since it was locked: the reader sums the bytes it
	// answers, and the lock holds the sum of others.
	changed := locked(manifest(t), wasm())
	changed.Locked = sumOf(manifest(t), []byte("the module as it was locked"))
	// A changed bundle is refused on its digest before its manifest is read,
	// so even one that is no YAML at all is ErrDigest.
	changedAndBroken := locked([]byte("{ not: [yaml"), wasm())
	changedAndBroken.Locked = changed.Locked
	// A reader that reports the lock's digest over bytes that are not the
	// lock's: the judge sums the bytes and does not take its word.
	lying := locked(manifest(t), []byte("a module swapped in"))
	lying.Locked, lying.Read = changed.Locked, changed.Locked
	// A file added to the folder after its lock: the reader sums what is
	// there, and the lock holds the sum without it.
	added := locked(manifest(t), wasm(), testdata()...)
	added.Locked = sumOf(manifest(t), wasm(), testdata()[0])
	// Every bound is checked before the sum: a manifest over its cap in a
	// bundle changed since its lock is refused on its size.
	overAndChanged := locked(padded(t, pluginbundle.MaxManifest+1), wasm())
	overAndChanged.Locked = changed.Locked
	tooMany := locked(manifest(t), wasm(), many(pluginbundle.MaxFiles-1)...)
	tooMany.Locked = changed.Locked
	silent := locked(manifest(t), wasm())
	silent.Read = ""
	// A lock that holds no digest is refused even where the read agrees with it.
	unlocked := locked(manifest(t), wasm())
	unlocked.Locked, unlocked.Read = "", ""
	bareHex := locked(manifest(t), wasm())
	bareHex.Locked = model.Digest(digest.Hex(wasm()))
	bareHex.Read = bareHex.Locked
	painted := "stand\x1b[31min"
	rows := []struct {
		name string
		ref  model.PluginRef
		b    model.PluginBundle
		want error
		says string
	}{
		{"a lock that holds no digest, and a read that agrees", standin(), unlocked, port.ErrDigest, "the lock of standin 1.0.0 holds nothing, which is not a digest"},
		{"a lock that holds bare hex, and a read that agrees", standin(), bareHex, port.ErrDigest, "which is not a digest"},
		{"a reader whose digest is not its bytes'", standin(), lying, port.ErrDigest, "the reader says the bundle of standin 1.0.0 sums to " + string(changed.Locked) + ", and its bytes sum to " + string(sumOf(manifest(t), []byte("a module swapped in")))},
		{"a reader that reports no digest", standin(), silent, port.ErrDigest, "sums to nothing"},
		{"a file added to the folder since its lock", standin(), added, port.ErrDigest, "and its lock holds " + string(added.Locked)},
		{"a bundle of a file over its bound, changed since its lock", standin(), tooMany, port.ErrDigest, "holds 1025 files, over the 1024 a bundle may hold"},
		{"a manifest over its cap, changed since its lock", standin(), overAndChanged, port.ErrManifest, "over the 65536 a manifest may hold"},
		{"a path that climbs out of the folder", standin(), withOther(t, "../x"), port.ErrDigest, "holds ../x, which is not a clean path below its folder"},
		{"a path with a .. step inside it", standin(), withOther(t, "testdata/../../x"), port.ErrDigest, "not a clean path"},
		{"a path from the root", standin(), withOther(t, "/x"), port.ErrDigest, "not a clean path"},
		{"a path with a . step", standin(), withOther(t, "./testdata/x"), port.ErrDigest, "not a clean path"},
		{"a path with an empty step", standin(), withOther(t, "testdata//x"), port.ErrDigest, "not a clean path"},
		{"a path with a backslash", standin(), withOther(t, "testdata/x\\y"), port.ErrDigest, "not a clean path"},
		{"a path that is the folder", standin(), withOther(t, "."), port.ErrDigest, "not a clean path"},
		{"a file of no path", standin(), withOther(t, ""), port.ErrDigest, "holds a file of no path"},
		{"a path that paints the screen", standin(), withOther(t, "\x1b[31m/../x"), port.ErrDigest, "holds \\x1b[31m/../x"},
		{"another file named plugin.yaml", standin(), withOther(t, "plugin.yaml"), port.ErrDigest, "holds plugin.yaml and plugin.yaml, which one folder cannot both hold"},
		{"plugin.yaml again in another case", standin(), withOther(t, "Plugin.YAML"), port.ErrDigest, "holds plugin.yaml and Plugin.YAML"},
		{"two folders that differ in case", standin(), withOther(t, "testdata/A/x", "testdata/a/y"), port.ErrDigest, "holds testdata/A/x and testdata/a/y"},
		{"a name and that name with a dot after it", standin(), withOther(t, "testdata/x", "testdata/x."), port.ErrDigest, "which one folder cannot both hold"},
		{"a file and a folder of one name", standin(), withOther(t, "testdata", "testdata/x"), port.ErrDigest, "holds testdata and testdata/x"},
		{"a bundle changed since its lock", standin(), changed, port.ErrDigest, "and its lock holds " + string(changed.Locked)},
		{"a changed bundle whose manifest is no YAML", standin(), changedAndBroken, port.ErrDigest, "reads as " + string(changedAndBroken.Read)},
		{"a lock whose ref paints the screen", model.PluginRef{Name: painted, Version: "1.0.0"}, bareHex, port.ErrDigest, "the lock of stand\\x1b[31min 1.0.0"},
		{"a manifest name with a control character", standin(), locked(manifest(t, "name: standin", "name: \"stand\\x1b[31min\""), wasm()), port.ErrManifest, "/name"},
		{"a lock whose name paints the screen, and a manifest that is not it", model.PluginRef{Name: painted, Version: "1.0.0"}, locked(manifest(t), wasm()), port.ErrManifest, "plugin.yaml names standin 1.0.0, and the lock installs it as stand\\x1b[31min 1.0.0"},
		{"a manifest a byte over its cap", standin(), locked(padded(t, pluginbundle.MaxManifest+1), wasm()), port.ErrManifest, "is 65537 bytes, over the 65536 a manifest may hold"},
		{"a contract that is NaN", standin(), locked(manifest(t, "contract: 1", "contract: .nan"), wasm()), port.ErrManifest, "no JSON form (NaN)"},
		{"a contract that is infinite", standin(), locked(manifest(t, "contract: 1", "contract: .inf"), wasm()), port.ErrManifest, "no JSON form (+Inf)"},
		{"a value that is minus infinity", standin(), locked(manifest(t, "warning: false", "warning: -.inf"), wasm()), port.ErrManifest, "no JSON form (-Inf)"},
		{"a manifest that is no YAML", standin(), locked([]byte("{ not: [yaml"), wasm()), port.ErrManifest, "is not one plain YAML document"},
		{"no plugin.yaml", standin(), locked(nil, wasm()), port.ErrManifest, "the bundle of standin 1.0.0 holds no plugin.yaml"},
		{"a plugin.yaml of no bytes", standin(), locked([]byte{}, wasm()), port.ErrManifest, "holds no plugin.yaml, or an empty one"},
		{"no plugin.wasm", standin(), locked(manifest(t), nil), port.ErrModule, "the bundle of standin 1.0.0 holds no plugin.wasm"},
		{"a plugin.wasm of no bytes, which its manifest names", standin(), locked(manifest(t, "wasm: "+digest.Prefixed(wasm()), "wasm: "+digest.Prefixed(nil)), []byte{}), port.ErrModule, "holds no plugin.wasm, or an empty one"},
		{"a manifest that is a comment alone", standin(), locked([]byte("# nothing\n"), wasm()), port.ErrManifest, "is not a mapping"},
		{"a manifest that is a list", standin(), locked([]byte("- contract\n"), wasm()), port.ErrManifest, "is not a mapping"},
		{"a manifest with an alias", standin(), locked(manifest(t, "tool: standin", "tool: &t standin\nwarning_tool: *t"), wasm()), port.ErrManifest, "plugin.yaml of standin 1.0.0 is not one plain YAML document"},
		{"a painted key written twice, which the YAML error quotes", standin(), locked(manifest(t, "warning: false", "warning: false\n\"pa\\x1b[31mint\": 1\n\"pa\\x1b[31mint\": 2"), wasm()), port.ErrManifest, "pa\\x1b[31mint"},
		{"a key written twice", standin(), locked(manifest(t, "tool: standin", "tool: standin\ntool: other"), wasm()), port.ErrManifest, "is not one plain YAML document"},
		{"a timestamp", standin(), locked(manifest(t, "warning: false", "warning: 2026-11-23"), wasm()), port.ErrManifest, "no JSON form"},
		{"another contract", standin(), locked(manifest(t, "contract: 1", "contract: 2"), wasm()), port.ErrContract, "is of contract 2, and the engine's is 1"},
		{"another contract with a key contract 1 lacks", standin(), locked(manifest(t, "contract: 1", "contract: 2\nlater: true"), wasm()), port.ErrContract, "contract 2"},
		{"no contract", standin(), locked(manifest(t, "contract: 1\n", ""), wasm()), port.ErrManifest, "contract"},
		{"an unknown key", standin(), locked(manifest(t, "warning: false", "warning: false\nnetwork: true"), wasm()), port.ErrManifest, "network"},
		{"another name", standin(), locked(manifest(t, "name: standin", "name: other"), wasm()), port.ErrManifest, "plugin.yaml names other 1.0.0, and the lock installs it as standin 1.0.0"},
		{"another version", standin(), locked(manifest(t, "version: 1.0.0", "version: 1.0.1"), wasm()), port.ErrManifest, "names standin 1.0.1"},
		{"a module the manifest's wasm is not", standin(), locked(manifest(t), []byte("another module")), port.ErrDigest, "and the module's is " + digest.Prefixed([]byte("another module"))},
		{"a wasm in bare hex", standin(), locked(manifest(t, "wasm: "+digest.Prefixed(wasm()), "wasm: "+digest.Hex(wasm())), wasm()), port.ErrManifest, "/wasm does not match"},
	}
	sentinels := []error{port.ErrDigest, port.ErrManifest, port.ErrContract, port.ErrModule}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			_, err := pluginbundle.Judge(r.ref, r.b)
			if !errors.Is(err, r.want) {
				t.Fatalf("Judge = %v, want %v", err, r.want)
			}
			for _, s := range sentinels {
				if !errors.Is(s, r.want) && errors.Is(err, s) {
					t.Errorf("Judge = %v, which matches %v as well as %v", err, s, r.want)
				}
			}
			if i := strings.IndexFunc(err.Error(), unicode.IsControl); i >= 0 {
				t.Errorf("Judge = %q, which holds a control character at %d", err, i)
			}
			if !strings.Contains(err.Error(), r.says) {
				t.Errorf("Judge = %q, want it to say %q", err, r.says)
			}
		})
	}
}

// TestAManifestAtItsCapIsJudgedWhole: a plugin.yaml of MaxManifest bytes is
// read, and one a byte over is refused (the row above).
//
// boundary: MaxManifest
func TestAManifestAtItsCapIsJudgedWhole(t *testing.T) {
	t.Parallel()
	m := padded(t, pluginbundle.MaxManifest)
	if len(m) != pluginbundle.MaxManifest {
		t.Fatalf("the padded manifest is %d bytes, want %d", len(m), pluginbundle.MaxManifest)
	}
	if _, err := pluginbundle.Judge(standin(), locked(m, wasm())); err != nil {
		t.Fatalf("Judge = %v, want the manifest", err)
	}
}

// TestABundleOfFilesAtItsBoundIsJudgedWhole: a bundle of MaxFiles files is
// read, and one of a file more is refused (the row above).
//
// boundary: MaxFiles
func TestABundleOfFilesAtItsBoundIsJudgedWhole(t *testing.T) {
	t.Parallel()
	b := locked(manifest(t), wasm(), many(pluginbundle.MaxFiles-2)...)
	if _, err := pluginbundle.Judge(standin(), b); err != nil {
		t.Fatalf("Judge of %d files = %v, want the manifest", 2+len(b.Other), err)
	}
}

// TestTheDigestIsOfEveryFileInPathOrder: Digest sums each file as the length
// of its path, a colon, its path, its length, a colon, its sum (digest.Hex)
// and a newline, sorted by path as bytes, so the order files are handed in
// does not change it and a third file does.
func TestTheDigestIsOfEveryFileInPathOrder(t *testing.T) {
	t.Parallel()
	y, w := []byte("name: x\n"), wasm()
	want := model.Digest(digest.Prefixed([]byte("11:plugin.wasm" + strconv.Itoa(len(w)) + ":" + digest.Hex(w) + "\n11:plugin.yaml8:" + digest.Hex(y) + "\n")))
	two := []model.File{{Path: "plugin.yaml", Data: y}, {Path: "plugin.wasm", Data: w}}
	if got := pluginbundle.Digest(two); got != want {
		t.Errorf("Digest = %s, want %s", got, want)
	}
	if got := pluginbundle.Digest([]model.File{two[1], two[0]}); got != want {
		t.Errorf("Digest in the other order = %s, want %s", got, want)
	}
	if got := pluginbundle.Digest(append(two, model.File{Path: "README", Data: nil})); got == want {
		t.Errorf("Digest with a third file = %s, the two files'", got)
	}
	upper := []model.File{{Path: "B", Data: nil}, {Path: "a", Data: nil}}
	if pluginbundle.Digest(upper) != pluginbundle.Digest([]model.File{upper[1], upper[0]}) {
		t.Errorf("Digest of B and a depends on the order they are handed in")
	}
}

// TestTwoSetsOfFilesWhoseBytesRunTogetherAlikeSumApart: each file's path
// and length are framed before its sum, so moving a byte from one file to the
// next, from a path into its bytes or from one path into the next gives
// another digest.
func TestTwoSetsOfFilesWhoseBytesRunTogetherAlikeSumApart(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		name string
		a, b []model.File
	}{
		{"a byte moved from one file to the next", []model.File{{Path: "a", Data: []byte("12")}, {Path: "b", Data: []byte("3")}}, []model.File{{Path: "a", Data: []byte("1")}, {Path: "b", Data: []byte("23")}}},
		{"a byte moved from a path into its bytes", []model.File{{Path: "ab", Data: []byte("c")}}, []model.File{{Path: "a", Data: []byte("bc")}}},
		{"a path whose digits read as the next length", []model.File{{Path: "a1", Data: nil}}, []model.File{{Path: "a", Data: []byte("1")}}},
		{"an empty file beside one, and one", []model.File{{Path: "a", Data: []byte("x")}, {Path: "b", Data: nil}}, []model.File{{Path: "a", Data: []byte("x")}}},
		// Without the length of each path before it, this one path would write
		// the very text of the two empty files a and b.
		{"one path, and the two whose framing it spells", []model.File{{Path: "a0:" + digest.Hex(nil) + "\nb", Data: nil}}, []model.File{{Path: "a", Data: nil}, {Path: "b", Data: nil}}},
	}
	for _, p := range pairs {
		if pluginbundle.Digest(p.a) == pluginbundle.Digest(p.b) {
			t.Errorf("%s: both sum to %s", p.name, pluginbundle.Digest(p.a))
		}
	}
}

// TestABundleOverMaxBundleIsRefusedBeforeItIsSummed: the files together are
// one byte over MaxBundle, which Judge refuses with port.ErrDigest whoever
// read them; at the bound they are judged. Planted: the total check gone.
//
// boundary: MaxBundle
func TestABundleOverMaxBundleIsRefusedBeforeItIsSummed(t *testing.T) {
	t.Parallel()
	m := manifest(t)
	fill := func(n int) model.File { return model.File{Path: "testdata/fill", Data: make([]byte, n)} }
	room := pluginbundle.MaxBundle - len(m) - len(wasm())
	if _, err := pluginbundle.Judge(standin(), locked(m, wasm(), fill(room))); err != nil {
		t.Fatalf("Judge of a bundle at MaxBundle = %v, want the manifest", err)
	}
	over := locked(m, wasm(), fill(room+1))
	if _, err := pluginbundle.Judge(standin(), over); !errors.Is(err, port.ErrDigest) {
		t.Errorf("Judge of a bundle one byte over MaxBundle = %v, want port.ErrDigest", err)
	}
}
