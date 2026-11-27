package config

// The settings file, <config>/gragas/settings.yaml: one YAML document that
// host/fs reads, after it checked who may write it, and cmd/gragas hands
// here as bytes. Every key has a reader, a default, a doc row and a
// refused-when (cli.config.6): the table is keys(), docs/settings.md is its
// doc, and a test holds the two together.

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/gragas/internal/ordered"
)

// Version is the settings file's version, which this build reads. A file
// with a newer one ends the run not ready, naming both; an older one, or
// none, is refused (store.schemas.2). It goes up when a key's meaning
// changes, and `migrate` (M10) will turn an older file into this one.
const Version = 1

// Settings is what the settings file says.
type Settings struct {
	Version  int
	Language string
	// Colour is auto, always or never.
	Colour string
	// Shell is the shell the lines to paste are written for: auto, or a
	// name of go-console's shell.Dialects, as String writes it.
	Shell    string
	Programs Programs
	// Sandbox is the adapter of the sandboxed level: none, container,
	// userns or macos. Its options are a table named after it.
	Sandbox string
	// Tools is the per-machine tools file, "" for <config>/gragas/tools.yaml.
	Tools string
	// ToolPaths is a tool's location on this machine, per project root:
	// root, then tool@version, then the path.
	ToolPaths map[string]map[string]string
	// Mirrors maps a rulebook source to a mirror of it.
	Mirrors map[string]string
	// Keep is how long the journal's copies and the caches are kept: a
	// count and d (days) or h (hours).
	Keep string
	// NamesFile is the forbidden-names file, "" for none.
	NamesFile string
	// BaseRef is the base ref check in a rulebook holds HEAD's history
	// against, by the merge base of the two, "" for none.
	BaseRef string
	// Mounts are the folders root mounted a disk on for this machine, each
	// in the slash form, on whose disks a name mounted nosuid, or noexec
	// without nosuid, is judged by its owners in place of refused.
	// Linux alone reads them.
	Mounts []string
}

// Programs are the absolute paths of the child programs gragas runs, ""
// where the settings name none. Git hands SSH and Credential to its shell,
// each in one pair of single quotes, so neither may hold a single quote.
type Programs struct {
	Git, Container, Userns, Macos, SSH, Credential string
}

// Defaults are the settings with no file.
func Defaults() Settings {
	return Settings{Version: Version, Language: "en", Colour: "auto", Shell: auto, Sandbox: "none", Keep: "30d"}
}

// Key is one key of the settings file, as docs/settings.md lists it.
type Key struct {
	Name string
	// Default is the default, as the doc says it.
	Default string
	// Holds says what the value is.
	Holds string
	// Refused says when a value is refused.
	Refused string
	// Env is the environment variable that sets it over the file, "" for none.
	Env  string
	read func(s *Settings, p place, v any) *Problem
}

// Keys are the keys of the settings file, in the order the doc lists them.
func Keys() []Key {
	k := keys()
	for i := range k {
		k[i].read = nil
	}
	return k
}

// sandboxes are the implementations of the sandboxed level, each a value
// of sandbox and the name of its options table (cli.config.11).
func sandboxes() []string { return []string{"container", "userns", "macos"} }

func keys() []Key {
	out := []Key{
		{Name: "version", Default: "none: it must be written", Holds: "the settings file's version, 1",
			Refused: "missing, not a whole number, or below 1; a newer one is not ready, naming both versions", read: readVersion},
		{Name: "language", Default: "en", Holds: "the language gragas speaks", Refused: "not en, the one this build has",
			read: oneOf(func(s *Settings) *string { return &s.Language }, "en")},
		{Name: "colour", Default: "auto", Holds: "colour: auto, always or never. NO_COLOR, CLICOLOR_FORCE, CI and --no-color decide over it",
			Refused: "not auto, always or never", read: oneOf(func(s *Settings) *string { return &s.Colour }, "auto", "always", "never")},
		{Name: "shell", Default: auto, Holds: "the shell the lines to paste are written for: " + either(shellNames()) + ", in any case. auto is the shell gragas was started from on Windows, else " + shell.POSIX.String(),
			Refused: shellRefused, Env: ShellVar, read: readShell},
		{Name: "programs", Default: "none named", Holds: "a table: the absolute path of git, container, userns, macos, ssh and credential",
			Refused: "not a table, a key that is not one of the six, a path that is not absolute and clean, or an ssh or credential path that holds a single quote",
			read:    readPrograms},
		{Name: "sandbox", Default: "none", Holds: "the sandboxed level's adapter: none, container, userns or macos, with its options in a table named after it",
			Refused: "another value, naming the ones this build has", read: oneOf(func(s *Settings) *string { return &s.Sandbox }, append([]string{"none"}, sandboxes()...)...)},
	}
	for _, name := range sandboxes() {
		out = append(out, Key{Name: name, Default: "no options", Holds: "the options of sandbox: " + name + "; this build has none",
			Refused: "not a table, any key in it, or sandbox is not " + name, read: readOptions(name)})
	}
	return append(out,
		Key{Name: "tools", Default: "the config folder's tools.yaml", Holds: "the per-machine tools file, held to the same owner and mode as this file",
			Refused: "a path that is not absolute and clean", read: readPath(func(s *Settings) *string { return &s.Tools })},
		Key{Name: "tool-paths", Default: "none", Holds: "a table: per project root, written with no link in it (as pwd -P prints it; a root written through a link matches no project), a table of tool@version to the tool's absolute path",
			Refused: "a root or path that is not absolute and clean, or a name that is not tool@version", read: readToolPaths},
		Key{Name: "mirrors", Default: "none", Holds: "a table: a rulebook source to a mirror of it, each an absolute path or an https, ssh or file URL",
			Refused: "a value that is neither, a URL with a password in it, a user on any but ssh, or a value, host or user that starts with a dash", read: readMirrors},
		Key{Name: "keep", Default: "30d", Holds: "how long the journal's copies and the caches are kept: a count of days (d) or hours (h)",
			Refused: "not a whole number from 1 to 9999 followed by d or h", read: readKeep},
		Key{Name: "names-file", Default: "none: the names check is skipped", Holds: "the private file of names public text must not hold, one regular expression a line",
			Refused: "a path that is not absolute and clean", Env: NamesFileVar, read: readNamesPath},
		Key{Name: "base-ref", Default: "none: check in a rulebook is not ready", Holds: "the branch, tag or commit check in a rulebook holds HEAD's history against, by the merge base of the two, such as origin/main",
			Refused: refRefused, Env: BaseRefVar, read: readRev},
		Key{Name: "mounts", Default: "none: a disk mounted nosuid, or noexec without nosuid, is refused", Holds: "a list of the folders root mounted a disk on for this machine, each the root of a mount itself, no link, and below no folder an account but root can change; a folder on such a disk is judged by its owners in place of refused, on Linux. The settings and tools files are read with " + MountsVar + " alone",
			Refused: "not a list, or an entry that is not an absolute, clean path", Env: MountsVar, read: readMounts},
	)
}

// place is where a value is: the file and the line of its key.
type place struct {
	file string
	line int
	key  string
}

func (p place) refuse(format string, args ...any) *Problem {
	return at(p.file, p.line, "%s %s", p.key, fmt.Sprintf(format, args...))
}

// Parse reads the settings file's bytes. file is its path, for the
// refusals, which each name the file and the line.
func Parse(file string, data []byte) (Settings, error) {
	s := Defaults()
	v, err := ordered.ParseYAML(data)
	if err != nil {
		return Defaults(), at(file, 0, "is not YAML: %v", err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return Defaults(), at(file, 0, "is not a table of settings: it must start with version: %d", Version)
	}
	if _, ok := doc.Get("version"); !ok {
		return Defaults(), at(file, doc.Line, "has no version: write version: %d first", Version)
	}
	known := keys()
	// version first, so a newer file is not ready before any key of it is
	// judged by this build's rules.
	order := append([]string{"version"}, slices.DeleteFunc(slices.Clone(doc.Keys()), func(k string) bool { return k == "version" })...)
	for _, k := range order {
		p := place{file: file, line: doc.KeyLine(k), key: k}
		i := slices.IndexFunc(known, func(x Key) bool { return x.Name == k })
		if i < 0 {
			return Defaults(), unknown(p, names(known))
		}
		val, _ := doc.Get(k)
		if prob := known[i].read(&s, p, val); prob != nil {
			return Defaults(), prob
		}
		if k != "version" {
			continue
		}
		// After the version, so a newer file is not ready; before any
		// other key, since a file that says two things is ambiguous.
		if prob := repeated(file, doc, ""); prob != nil {
			return Defaults(), prob
		}
	}
	for _, name := range sandboxes() {
		if _, ok := doc.Get(name); ok && s.Sandbox != name {
			return Defaults(), at(file, doc.KeyLine(name), "%s is the options of sandbox: %s, and sandbox is %s", name, name, s.Sandbox)
		}
	}
	return s, nil
}

// repeated refuses the first key v writes twice in one table, at the line
// it is written again, naming the line before: a file that says two things
// is ambiguous, so neither is taken. prefix is the tables v is in.
func repeated(file string, v any, prefix string) *Problem {
	switch x := v.(type) {
	case *ordered.Map:
		if rs := x.Repeats(); len(rs) > 0 {
			return at(file, rs[0].Again, "%s is written twice, first on line %d: keep one", prefix+rs[0].Key, rs[0].First)
		}
		for _, k := range x.Keys() {
			val, _ := x.Get(k)
			if p := repeated(file, val, prefix+k+"."); p != nil {
				return p
			}
		}
	case []any:
		for _, e := range x {
			if p := repeated(file, e, prefix); p != nil {
				return p
			}
		}
	}
	return nil
}

// gone answers what went with key, a key a ruling took out, by its full
// name in lower case, and false for any other: a file that still holds one
// is refused with the ruling, not only told the key is unknown
// (cli.config.2).
func gone(key string) (string, bool) {
	switch key {
	case "signing-key":
		return "gragas signs no release and reads no key", true
	case "programs.signer":
		return "gragas runs no signing program", true
	}
	return "", false
}

// unknown refuses the key at p, which none of known is: a key a ruling took
// out is told so (gone), and a key spelled in another case is told its
// spelling (cli.config.2).
func unknown(p place, known []string) *Problem {
	if what, ok := gone(strings.ToLower(p.key)); ok {
		return at(p.file, p.line, "%s is gone: %s, since a release is pinned by its content hash (docs/reasons/release-pin-by-hash.md); remove it", p.key, what)
	}
	if i := slices.Index(known, strings.ToLower(p.key[strings.LastIndex(p.key, ".")+1:])); i >= 0 {
		return at(p.file, p.line, "unknown key %q: keys are lower case, so write %s", p.key, known[i])
	}
	return at(p.file, p.line, "unknown key %q: the keys are %s", p.key, strings.Join(known, ", "))
}

func names(k []Key) []string {
	out := make([]string, len(k))
	for i, x := range k {
		out[i] = x.Name
	}
	return out
}

func readVersion(s *Settings, p place, v any) *Problem {
	n, prob := p.version(v, Version, "settings", "the settings file")
	if prob == nil {
		s.Version = n
	}
	return prob
}

// version reads the version key of a file this gragas reads at version
// reads: kind names the versions ("settings version 2") and file the file
// ("the settings file is version 2"). A newer version is not ready, naming
// both; anything else not a whole number of at least 1 is refused. The rule is
// ordered.Version, the one for every versioned file (gate/shared.yaml, version);
// these are the settings and tools files' words for its verdicts.
func (p place) version(v any, reads int, kind, file string) (int, *Problem) {
	n, is := ordered.Version(v, reads)
	switch is {
	case ordered.VersionNotWhole:
		return 0, p.refuse("must be a whole number, %d", reads)
	case ordered.VersionNewer:
		return 0, &Problem{Kind: NotReady, Next: "install a gragas that reads " + kind + " version " + strconv.FormatInt(n, 10),
			What: fmt.Sprintf("%s:%d: %s is version %d, and this gragas reads version %d", p.file, p.line, file, n, reads)}
	case ordered.VersionBelowOne:
		return 0, p.refuse("%d is not a %s version: write version: %d", n, kind, reads)
	case ordered.VersionRead:
	}
	return int(n), nil
}

func oneOf(field func(*Settings) *string, allowed ...string) func(*Settings, place, any) *Problem {
	return func(s *Settings, p place, v any) *Problem {
		str, ok := v.(string)
		if !ok || !slices.Contains(allowed, str) {
			return p.refuse("must be %s, and it is %s", either(allowed), ordered.Repr(v))
		}
		*field(s) = str
		return nil
	}
}

// either is the allowed values as a person says them: a, b or c.
func either(allowed []string) string {
	if len(allowed) == 1 {
		return allowed[0]
	}
	return strings.Join(allowed[:len(allowed)-1], ", ") + " or " + allowed[len(allowed)-1]
}

func readPath(field func(*Settings) *string) func(*Settings, place, any) *Problem {
	return func(s *Settings, p place, v any) *Problem {
		str, prob := pathAt(p, v)
		if prob != nil {
			return prob
		}
		*field(s) = str
		return nil
	}
}

// pathAt is v as a path a setting names: a string, absolute and clean, with
// no control character, so it never starts with a dash, never climbs with
// .., and prints on one line.
func pathAt(p place, v any) (string, *Problem) {
	str, ok := v.(string)
	if !ok {
		return "", p.refuse("must be a path, and it is %s", ordered.Repr(v))
	}
	if why := unclean(str); why != "" {
		return "", p.refuse("%s", why)
	}
	return str, nil
}

// unclean says why s is not a path a setting may name, "" when it is one:
// absolute and clean, with no control character, so it never starts with a
// dash, never climbs with .., and prints on one line.
func unclean(s string) string {
	switch {
	case strings.IndexFunc(s, unicode.IsControl) >= 0:
		return "holds a control character"
	case !filepath.IsAbs(s) || strings.HasPrefix(s, "-"):
		return fmt.Sprintf("must be an absolute path, and it is %q", s)
	case filepath.Clean(s) != s && filepath.ToSlash(filepath.Clean(s)) != s:
		// A setting is in the model's slash form, an environment value in the system's own:
		// either is clean when it is its own clean form.
		return fmt.Sprintf("must be a clean path: write %q", filepath.Clean(s))
	}
	return ""
}

// unshown ends a refusal of the names file's path, which says why from a
// closed set and never the path: the path can hold a login, and the
// login can be on the list.
const unshown = " (the path is not shown: it can hold a name on the list)"

// uncleanNames is unclean for the names file's path: the same three reasons,
// in words that hold no part of s.
func uncleanNames(s string) string {
	switch {
	case strings.IndexFunc(s, unicode.IsControl) >= 0:
		return "holds a control character" + unshown
	case !filepath.IsAbs(s) || strings.HasPrefix(s, "-"):
		return "must be an absolute path" + unshown
	case filepath.Clean(s) != s && filepath.ToSlash(filepath.Clean(s)) != s:
		return "must be a clean path, with no . or .. part and no doubled or trailing separator" + unshown
	}
	return ""
}

// readNamesPath is the names-file key: readPath's checks, refused in
// uncleanNames' words, so the settings refusal holds no part of the path.
func readNamesPath(s *Settings, p place, v any) *Problem {
	str, ok := v.(string)
	if !ok {
		return p.refuse("must be a path, and it is not a string%s", unshown)
	}
	if why := uncleanNames(str); why != "" {
		return p.refuse("%s", why)
	}
	s.NamesFile = str
	return nil
}

// refRefused is what the base-ref key and GRAGAS_BASE_REF refuse, as
// docs/settings.md says it.
const refRefused = "a name git could read as an option or as more than one revision: empty, longer than 255 bytes, a space or control byte, one of \\~^:?*[, a start of -, / or ., a .., @{, // or /., or an end of /, . or .lock"

// badRev says why s is not a revision the git adapter runs, "" when it is
// one. It is the adapter's own check, made where the value is read, so a
// bad one ends the run before git is found.
func badRev(s string) string {
	if s == "" || len(s) > 255 {
		return fmt.Sprintf("must be a revision of 1 to 255 bytes, and it is %d", len(s))
	}
	for i := range len(s) {
		if c := s[i]; c <= ' ' || c >= 0x7f || strings.IndexByte(`\~^:?*[`, c) >= 0 {
			return fmt.Sprintf("must be a revision, and it holds the byte %q", c)
		}
	}
	switch {
	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "/"), strings.HasPrefix(s, "."):
		return fmt.Sprintf("must be a revision, and it starts with %q", s[0])
	case strings.Contains(s, ".."), strings.Contains(s, "@{"), strings.Contains(s, "//"), strings.Contains(s, "/."):
		return fmt.Sprintf("must be one revision, and it is %q", s)
	case strings.HasSuffix(s, "/"), strings.HasSuffix(s, "."), strings.HasSuffix(s, ".lock"):
		return fmt.Sprintf("must be a revision, and %q ends as no name does", s)
	}
	return ""
}

// readRev reads the base-ref key: a string that is one revision.
func readRev(s *Settings, p place, v any) *Problem {
	str, ok := v.(string)
	if !ok {
		return p.refuse("must be a revision, and it is %s", ordered.Repr(v))
	}
	if why := badRev(str); why != "" {
		return p.refuse("%s", why)
	}
	s.BaseRef = str
	return nil
}

// readMounts reads the mounts key: a list of paths, each as pathAt holds a
// setting's path. An empty list is none.
func readMounts(s *Settings, p place, v any) *Problem {
	list, ok := v.([]any)
	if !ok {
		return p.refuse("must be a list of paths, and it is %s", ordered.Repr(v))
	}
	out := make([]string, 0, len(list))
	for i, e := range list {
		str, prob := pathAt(place{file: p.file, line: p.line, key: fmt.Sprintf("%s[%d]", p.key, i)}, e)
		if prob != nil {
			return prob
		}
		out = append(out, str)
	}
	s.Mounts = out
	return nil
}

// table is v as a table, refused at p when it is not one.
func table(p place, v any) (*ordered.Map, *Problem) {
	m, ok := v.(*ordered.Map)
	if !ok {
		return nil, p.refuse("must be a table, and it is %s", ordered.Repr(v))
	}
	return m, nil
}

// within is the place of key k of table m, at p.
func within(p place, m *ordered.Map, k string) place {
	return place{file: p.file, line: m.KeyLine(k), key: p.key + "." + k}
}

func readPrograms(s *Settings, p place, v any) *Problem {
	m, prob := table(p, v)
	if prob != nil {
		return prob
	}
	fields := map[string]*string{"git": &s.Programs.Git,
		"container": &s.Programs.Container, "userns": &s.Programs.Userns, "macos": &s.Programs.Macos,
		"ssh": &s.Programs.SSH, "credential": &s.Programs.Credential}
	known := []string{"git", "container", "userns", "macos", "ssh", "credential"}
	for _, k := range m.Keys() {
		q := within(p, m, k)
		field, ok := fields[k]
		if !ok {
			return unknown(q, known)
		}
		val, _ := m.Get(k)
		str, prob := pathAt(q, val)
		if prob != nil {
			return prob
		}
		// git hands these to its shell in one pair of single quotes, which a
		// single quote in the path would end.
		if (k == "ssh" || k == "credential") && strings.ContainsRune(str, '\'') {
			return q.refuse("holds a single quote, and git hands this path to its shell in single quotes, which it would end: name the program at a path without one, and it is %q", str)
		}
		*field = str
	}
	return nil
}

func readOptions(name string) func(*Settings, place, any) *Problem {
	return func(_ *Settings, p place, v any) *Problem {
		m, prob := table(p, v)
		if prob != nil {
			return prob
		}
		if len(m.Keys()) > 0 {
			k := m.Keys()[0]
			return at(p.file, m.KeyLine(k), "unknown key %q in %s: the %s sandbox has no options in this build", k, name, name)
		}
		return nil
	}
}

func readToolPaths(s *Settings, p place, v any) *Problem {
	roots, prob := table(p, v)
	if prob != nil {
		return prob
	}
	out := map[string]map[string]string{}
	for _, root := range roots.Keys() {
		q := within(p, roots, root)
		if _, prob := pathAt(q, root); prob != nil {
			return prob
		}
		val, _ := roots.Get(root)
		tools, prob := table(q, val)
		if prob != nil {
			return prob
		}
		out[filepath.ToSlash(root)] = map[string]string{}
		for _, tool := range tools.Keys() {
			r := within(q, tools, tool)
			if !toolAt(tool) {
				return r.refuse("is not tool@version")
			}
			val, _ := tools.Get(tool)
			str, prob := pathAt(r, val)
			if prob != nil {
				return prob
			}
			out[filepath.ToSlash(root)][tool] = filepath.ToSlash(str)
		}
	}
	s.ToolPaths = out
	return nil
}

// toolAt is whether s is tool@version: a name of letters, digits and . _ -
// that starts with a letter or digit, an @, and a version of letters,
// digits and . + -.
func toolAt(s string) bool {
	name, ver, ok := strings.Cut(s, "@")
	word := func(w, more string) bool {
		if w == "" || !isAlnum(rune(w[0])) {
			return false
		}
		for _, r := range w {
			if !isAlnum(r) && !strings.ContainsRune(more, r) {
				return false
			}
		}
		return true
	}
	return ok && word(name, "._-") && word(ver, ".+-")
}

func isAlnum(r rune) bool { return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) }

func readMirrors(s *Settings, p place, v any) *Problem {
	m, prob := table(p, v)
	if prob != nil {
		return prob
	}
	out := map[string]string{}
	for _, k := range m.Keys() {
		q := within(p, m, k)
		if prob := where(q, k); prob != nil {
			return prob
		}
		val, _ := m.Get(k)
		str, ok := val.(string)
		if !ok {
			return q.refuse("must be a path or a URL, and it is %s", ordered.Repr(val))
		}
		if prob := where(q, str); prob != nil {
			return prob
		}
		out[k] = str
	}
	s.Mirrors = out
	return nil
}

// where holds a rulebook source or mirror, as parsed: an absolute, clean
// path, or an https, ssh or file URL with a host or a path and no password,
// where only ssh names a user, and no host or user starts with a dash, which
// ssh would read as an option.
// A password in a URL would be a secret in the settings (supply.secrets.2).
func where(p place, s string) *Problem {
	if filepath.IsAbs(s) {
		_, prob := pathAt(p, s)
		return prob
	}
	if strings.HasPrefix(s, "-") || strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return p.refuse("must be an absolute path or a URL, and it is %q", s)
	}
	u, err := url.Parse(s)
	switch {
	case err != nil:
		return p.refuse("is not a URL: %v", err)
	case !slices.Contains([]string{"https", "ssh", "file"}, u.Scheme):
		return p.refuse("must be an absolute path or an https, ssh or file URL, and it is %q", s)
	case u.User != nil && func() bool { _, has := u.User.Password(); return has }():
		return p.refuse("holds a password: the settings name where a secret is, never the secret")
	case u.User != nil && u.Scheme != "ssh":
		return p.refuse("names a user, which only an ssh URL may: write %s://%s%s", u.Scheme, u.Host, u.EscapedPath())
	case u.User != nil && (u.User.Username() == "" || strings.HasPrefix(u.User.Username(), "-")):
		return p.refuse("names the user %q, and ssh would read one that is empty or starts with - wrongly", u.User.Username())
	case strings.HasPrefix(u.Hostname(), "-"):
		return p.refuse("names the host %q, and ssh would read one that starts with - as an option", u.Hostname())
	case u.Host == "" && u.Path == "":
		return p.refuse("names no host and no path")
	}
	return nil
}

func readKeep(s *Settings, p place, v any) *Problem {
	str, ok := v.(string)
	n := strings.TrimRight(str, "dh")
	if !ok || len(str) < 2 || len(str)-len(n) != 1 || len(n) > 4 || n[0] == '0' || strings.Trim(n, "0123456789") != "" {
		return p.refuse("must be a whole number from 1 to 9999 and d or h, such as 30d, and it is %s", ordered.Repr(v))
	}
	s.Keep = str
	return nil
}
