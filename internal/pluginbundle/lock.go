package pluginbundle

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// PluginLockVersion is the newest version of the plugin lock this gragas
// reads and the one it writes.
const PluginLockVersion = 1

// The places of the plugins in a rulebook: the bundle of a plugin is
// plugins/<name>/<version>/, and the lock that pins each bundle's digest is
// beside them.
const (
	// PluginsFolder is the folder of the plugins in a rulebook's root.
	PluginsFolder = "plugins"
	// PluginLockPath is the lock's path in a rulebook's root.
	PluginLockPath = PluginsFolder + "/lock.yaml"
)

// errPluginLock is the refusal of a lock that is not whole YAML of its form:
// it is never read as an answer, and never written over.
var errPluginLock = errors.New("the plugin lock is not one this gragas reads")

var (
	nameForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// versionForm is a plugin's version, as the manifest schema's: three
	// whole numbers with no leading zero, so semver orders it.
	versionForm = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	// zeroLed is the form a lock took before this one: three whole
	// numbers, a leading zero allowed. An entry of it is refused by name,
	// with its fix, never read as a version semver cannot order.
	zeroLed = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// NameFormed is whether name is a plugin's name as the lock holds one: at
// most 64 bytes of lower-case letters and digits, in words joined by one
// hyphen. A name that is formed holds no slash, dot or other step of a path.
func NameFormed(name string) bool { return len(name) <= 64 && nameForm.MatchString(name) }

// VersionFormed is whether version has the shape of a plugin's version: three
// whole numbers joined by dots, a leading zero allowed, so a caller can refuse
// one semver cannot order by name. The lock holds only one with no leading
// zero.
func VersionFormed(version string) bool { return zeroLed.MatchString(version) }

// LockEntry is one plugin the lock holds: its ref, where it was added from,
// and the digest of its bundle's folder.
type LockEntry struct {
	Ref    model.PluginRef
	Source string
	Digest model.Digest
}

// lockTopKeys() and lockEntryKeys() are the keys of the lock, each exactly.
func lockTopKeys() []string   { return []string{"version", "plugins"} }
func lockEntryKeys() []string { return []string{"name", "version", "source", "digest"} }

// DecodePluginLock reads a lock: one mapping of exactly a version this gragas
// reads and a list of entries, each of exactly a name, a version, a source
// and a digest, no ref twice, and answers them by name and then version as
// bytes. Anything else is refused whole, matching errPluginLock, with the
// line.
func DecodePluginLock(b []byte) ([]LockEntry, error) {
	v, err := ordered.ParseStrict(b)
	if err != nil {
		return nil, refuseLock(0, "it is not one YAML document: %v", err)
	}
	top, ok := v.(*ordered.Map)
	if !ok || !hasExactly(top, lockTopKeys()) {
		return nil, refuseLock(lineOf(v), "it is not a mapping of exactly %v", lockTopKeys())
	}
	switch n, is := ordered.Version(mustGet(top, "version"), PluginLockVersion); is {
	case ordered.VersionRead:
		// The version this gragas reads: the list is judged next.
	case ordered.VersionNewer:
		return nil, refuseLock(top.KeyLine("version"), "its version is %d, and this gragas reads %d", n, PluginLockVersion)
	default:
		return nil, refuseLock(top.KeyLine("version"), "its version is not a whole number from 1")
	}
	list, ok := mustGet(top, "plugins").([]any)
	if !ok {
		return nil, refuseLock(top.KeyLine("plugins"), "plugins is not a list")
	}
	out := make([]LockEntry, 0, len(list))
	for i, item := range list {
		e, err := decodeLockEntry(item)
		if err != nil {
			return nil, refuseLock(lineOf(item), "entry %d: %v", i, err)
		}
		if slices.ContainsFunc(out, func(o LockEntry) bool { return o.Ref == e.Ref }) {
			return nil, refuseLock(lineOf(item), "entry %d: %s@%s is listed twice", i, e.Ref.Name, e.Ref.Version)
		}
		if n := len(out); n > 0 && entryBefore(e, out[n-1]) {
			return nil, refuseLock(lineOf(item), "entry %d: %s@%s is out of order; the lock is sorted by name and then version", i, e.Ref.Name, e.Ref.Version)
		}
		out = append(out, e)
	}
	return out, nil
}

// decodeLockEntry reads one entry of the lock.
func decodeLockEntry(v any) (LockEntry, error) {
	m, ok := v.(*ordered.Map)
	if !ok || !hasExactly(m, lockEntryKeys()) {
		return LockEntry{}, fmt.Errorf("it is not a mapping of exactly %v", lockEntryKeys())
	}
	name, version := m.Str("name"), m.Str("version")
	source, d := m.Str("source"), model.Digest(m.Str("digest"))
	switch {
	case !NameFormed(name):
		return LockEntry{}, fmt.Errorf("name %q is not a plugin's name", name)
	case zeroLed.MatchString(version) && !versionForm.MatchString(version):
		return LockEntry{}, fmt.Errorf("plugin %s's version %q has a leading zero, so it cannot be ordered: "+
			"remove the entry and the folder %s, then add the plugin again from a bundle whose version has none (gragas plugin add <folder>)",
			name, version, FolderOf(model.PluginRef{Name: name, Version: version}))
	case !versionForm.MatchString(version):
		return LockEntry{}, fmt.Errorf("version %q is not a version of three whole numbers", version)
	case source == "" || len(source) > 4096 || strings.ContainsFunc(source, func(r rune) bool { return r < ' ' || r == 0x7f }):
		return LockEntry{}, errors.New("source is empty, over 4096 bytes or holds a control character")
	case !d.Formed():
		return LockEntry{}, fmt.Errorf("digest %q is not sha256: and 64 lower-case hex digits", string(d))
	}
	return LockEntry{Ref: model.PluginRef{Name: name, Version: version}, Source: source, Digest: d}, nil
}

// EncodePluginLock is the lock of entries, written as DecodePluginLock reads
// it, sorted by name and then version, one way for one set of entries.
func EncodePluginLock(entries []LockEntry) []byte {
	sorted := slices.Clone(entries)
	sortEntries(sorted)
	list := make([]any, 0, len(sorted))
	for _, e := range sorted {
		m := ordered.NewMap(0)
		m.Set("name", e.Ref.Name, 0)
		m.Set("version", e.Ref.Version, 0)
		m.Set("source", e.Source, 0)
		m.Set("digest", string(e.Digest), 0)
		list = append(list, m)
	}
	top := ordered.NewMap(0)
	top.Set("version", int64(PluginLockVersion), 0)
	top.Set("plugins", list, 0)
	return []byte(ordered.YAML(top))
}

func entryBefore(a, b LockEntry) bool {
	return a.Ref.Name+"\x00"+a.Ref.Version < b.Ref.Name+"\x00"+b.Ref.Version
}

func sortEntries(es []LockEntry) {
	slices.SortFunc(es, func(a, b LockEntry) int {
		return strings.Compare(a.Ref.Name+"\x00"+a.Ref.Version, b.Ref.Name+"\x00"+b.Ref.Version)
	})
}

func hasExactly(m *ordered.Map, keys []string) bool {
	if m.Len() != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m.Get(k); !ok {
			return false
		}
	}
	return true
}

func mustGet(m *ordered.Map, k string) any {
	v, _ := m.Get(k)
	if i, ok := v.(int); ok {
		return int64(i)
	}
	return v
}

func lineOf(v any) int {
	if m, ok := v.(*ordered.Map); ok {
		return m.Line
	}
	return 0
}

func refuseLock(line int, format string, a ...any) error {
	where := ""
	if line > 0 {
		where = " (line " + strconv.Itoa(line) + ")"
	}
	return fmt.Errorf("%w%s: %s: %w", errPluginLock, where, fmt.Sprintf(format, a...), fs.ErrInvalid)
}

// FolderOf is the folder a bundle is installed in below a rulebook's root:
// plugins/<name>/<version>.
func FolderOf(r model.PluginRef) string { return path.Join(PluginsFolder, r.Name, r.Version) }
