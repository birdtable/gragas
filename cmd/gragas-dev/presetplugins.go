package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/dev/say"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

const presetPluginsUsage = `usage: gragas-dev preset-plugins [-C root]
`

// runPresetPlugins is `preset-plugins` (gragas#2340): the plugins the tree
// installs, held to the pins of tools.yaml's plugins: block, approved by
// those digests in a state folder made for this run alone, and then gragas
// preset --check over root through them. gragas is built from the module in
// the folder the command runs in, and run in root. Any step that cannot be
// done is red. The one thing skipped is the preset of a pinned plugin no rule
// binds, which preset --check does not list, and the member names each.
func runPresetPlugins(ctx context.Context, environ []string, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("preset-plugins", flag.ContinueOnError)
	fl.SetOutput(stderr)
	root := fl.String("C", ".", "the root holding tools.yaml, plugins/ and the rulebook")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		talk.Text(presetPluginsUsage)
		return exitUsage
	}
	n, marked, said, err := presetPlugins(ctx, environ, ".", *root)
	if err != nil {
		talk.Text("gragas-dev preset-plugins: " + err.Error() + "\n")
		return exitFailed
	}
	talk.Text(said + "\n")
	(&say.Speaker{W: stdout}).Text(fmt.Sprintf("preset-plugins: %d pinned plugin(s) approved in a state folder of this run, gragas preset --check holds the presets through them, and %d marked preset(s) open with the mark\n", n, marked))
	return exitDone
}

// pluginPin is what tools.yaml pins of one plugin: its name and version, the
// tag of the release it came from, that tag's object and commit, and the
// digest of its bundle the release recorded, which is the lock's.
type pluginPin struct {
	name, version, tag, object, commit, digest string
}

// ref is the pin as gragas names a plugin, name@version.
func (p pluginPin) ref() string { return p.name + "@" + p.version }

var (
	// pluginPinLine's version is the manifest schema's and the lock's: three
	// whole numbers with no leading zero, so semver orders it (PR 2394).
	pluginPinLine = regexp.MustCompile(`^  ([a-z0-9]+(?:-[a-z0-9]+)*)@((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)): \{(.*)\}$`)
	hex40         = regexp.MustCompile(`^[0-9a-f]{40}$`)
	bundleDigest  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// pinKeys are the keys of a plugin's pin, each exactly once, in this order.
func pinKeys() []string { return []string{"tag", "object", "commit", "digest"} }

// readPluginPins is the plugins: block of tools.yaml, one line a plugin,
//
//	<name>@<version>: {tag: <name>/v<version>, object: <40 hex>, commit: <40 hex>, digest: "sha256:<64 hex>"}
//
// indented two spaces and read by lines, as the browsers: block is. A line of the block that is not
// that, a plugin pinned twice, and a block with no plugin are refused, naming
// the line: a pin that cannot be read pins nothing, and the member is red.
func readPluginPins(yaml []byte) ([]pluginPin, error) {
	in, seen := false, false
	var pins []pluginPin
	for i, line := range strings.Split(string(yaml), "\n") {
		switch {
		case line == "plugins:":
			if seen {
				return nil, fmt.Errorf("tools.yaml line %d: a second plugins: block", i+1)
			}
			in, seen = true, true
			continue
		case !in:
			continue
		case line == "" || strings.HasPrefix(strings.TrimSpace(line), "#"):
			continue
		case line[0] != ' ':
			in = false
			continue
		}
		p, err := pluginPinOf(line)
		if err != nil {
			return nil, fmt.Errorf("tools.yaml line %d: %w", i+1, err)
		}
		if slices.ContainsFunc(pins, func(q pluginPin) bool { return q.ref() == p.ref() }) {
			return nil, fmt.Errorf("tools.yaml line %d: %s is pinned twice", i+1, p.ref())
		}
		pins = append(pins, p)
	}
	if len(pins) == 0 {
		return nil, errors.New("tools.yaml pins no plugin under plugins:, so the presets would be held through none")
	}
	return pins, nil
}

// pluginPinOf is one line of the plugins: block.
func pluginPinOf(line string) (pluginPin, error) {
	m := pluginPinLine.FindStringSubmatch(line)
	if m == nil {
		return pluginPin{}, fmt.Errorf("%q is not \"  <name>@<version>: {tag: ..., object: ..., commit: ..., digest: ...}\"", line)
	}
	p := pluginPin{name: m[1], version: m[2]}
	fields := strings.Split(m[3], ", ")
	if len(fields) != len(pinKeys()) {
		return pluginPin{}, fmt.Errorf("%s's pin has %d fields, and a pin is %s", p.ref(), len(fields), strings.Join(pinKeys(), ", "))
	}
	for i, f := range fields {
		k, v, ok := strings.Cut(f, ": ")
		if !ok || k != pinKeys()[i] {
			return pluginPin{}, fmt.Errorf("%s's pin field %d is %q, and a pin is %s, in that order", p.ref(), i+1, f, strings.Join(pinKeys(), ", "))
		}
		switch k {
		case "tag":
			p.tag = v
		case "object":
			p.object = v
		case "commit":
			p.commit = v
		case "digest":
			p.digest = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
		}
	}
	switch {
	case p.tag != p.name+"/v"+p.version:
		return pluginPin{}, fmt.Errorf("%s's tag is %q, and a plugin's release is tagged %s/v%s", p.ref(), p.tag, p.name, p.version)
	case !hex40.MatchString(p.object):
		return pluginPin{}, fmt.Errorf("%s's object is %q, not 40 lowercase hex", p.ref(), p.object)
	case !hex40.MatchString(p.commit):
		return pluginPin{}, fmt.Errorf("%s's commit is %q, not 40 lowercase hex", p.ref(), p.commit)
	case !bundleDigest.MatchString(p.digest):
		return pluginPin{}, fmt.Errorf("%s's digest is %q, not sha256: and 64 lowercase hex", p.ref(), p.digest)
	}
	return p, nil
}

// lockAgainstPins is what differs between the plugin lock of root and pins:
// a plugin the lock holds that nothing pins, one pinned that the lock does
// not hold, and one whose lock digest is not its pin, each named. A plugin
// installed and not pinned would be approved by nobody, and one pinned and
// not installed would hold nothing.
func lockAgainstPins(lock []hostfs.LockEntry, pins []pluginPin) []string {
	var problems []string
	for _, e := range lock {
		ref := e.Ref.Name + "@" + e.Ref.Version
		i := slices.IndexFunc(pins, func(p pluginPin) bool { return p.ref() == ref })
		switch {
		case i < 0:
			problems = append(problems, "plugins/lock.yaml holds "+ref+", and tools.yaml's plugins: block pins no such plugin")
		case string(e.Digest) != pins[i].digest:
			problems = append(problems, "plugins/lock.yaml holds "+ref+" at "+string(e.Digest)+", and tools.yaml pins "+pins[i].digest+", the digest its release "+pins[i].tag+" recorded")
		}
	}
	for _, p := range pins {
		if !slices.ContainsFunc(lock, func(e hostfs.LockEntry) bool { return e.Ref.Name+"@"+e.Ref.Version == p.ref() }) {
			problems = append(problems, "tools.yaml pins "+p.ref()+", and plugins/lock.yaml does not hold it: add it with gragas plugin add from "+p.tag+" at "+p.commit)
		}
	}
	return problems
}

// presetPlugins is the member: the pins, the lock held to them, a state
// folder and a settings file made for the run, gragas built, each pinned
// plugin approved by its pin in that state folder, then preset --check under
// --json, then each marked preset of a tool its report lists held to the
// mark. gragas is built from the module at src, the run's own folder in the
// member. It answers how many plugins it approved, how many marked presets
// open with the mark, and the member's account of the check: what it did not
// compare, each pinned plugin no rule binds, and its sentence.
func presetPlugins(ctx context.Context, environ []string, src, root string) (n, marked int, said string, err error) {
	raw, err := os.ReadFile(filepath.Join(root, "tools.yaml"))
	if err != nil {
		return 0, 0, "", fmt.Errorf("the plugin pins could not be read: %w", err)
	}
	pins, err := readPluginPins(raw)
	if err != nil {
		return 0, 0, "", err
	}
	rawLock, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(hostfs.PluginLockPath)))
	if err != nil {
		return 0, 0, "", fmt.Errorf("the plugin lock could not be read, so no pinned plugin is installed: %w", err)
	}
	lock, err := hostfs.DecodePluginLock(rawLock)
	if err != nil {
		return 0, 0, "", fmt.Errorf("%s: %w", hostfs.PluginLockPath, err)
	}
	if problems := lockAgainstPins(lock, pins); len(problems) > 0 {
		return 0, 0, "", errors.New(strings.Join(problems, "\n"))
	}
	stateVar := config.StateVar()
	if stateVar == "" {
		return 0, 0, "", errors.New("no variable moves gragas's state folder on this system, so an approval could not be kept apart from the account's own")
	}
	tmp, err := os.MkdirTemp("", "gragas-preset-plugins-")
	if err != nil {
		return 0, 0, "", err
	}
	defer func() {
		if rerr := os.RemoveAll(tmp); rerr != nil && err == nil {
			err = rerr
		}
	}()
	state := filepath.Join(tmp, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		return 0, 0, "", err
	}
	// The run's settings name no file of the account's: its own, empty.
	settings := filepath.Join(tmp, "settings.yaml")
	if err := os.WriteFile(settings, []byte("version: 1\n"), 0o600); err != nil {
		return 0, 0, "", err
	}
	gragas, err := buildGragas(ctx, environ, src, tmp)
	if err != nil {
		return 0, 0, "", err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return 0, 0, "", err
	}
	// proc keeps a variable named twice, so the run's own two replace the
	// caller's rather than follow them.
	env := overVars(environ, []string{config.ConfigVar + "=" + settings, stateVar + "=" + state})
	for _, p := range pins {
		if _, err := outputIn(ctx, abs, env, gragas, "plugin", "approve", p.ref(), "--digest", p.digest, "--apply"); err != nil {
			return 0, 0, "", fmt.Errorf("%s was not approved by its pin %s: %w", p.ref(), p.digest, err)
		}
	}
	rep, err := checkPresets(ctx, gragas, abs, env)
	if err != nil {
		return 0, 0, "", err
	}
	marked, skipped, err := markedPresets(root, pins, rep.Data)
	if err != nil {
		return 0, 0, "", err
	}
	return len(pins), marked, rep.said(skipped), nil
}

// checkPresets runs gragas --json preset --check in dir under env and answers
// its report. A check that does not end done is red, saying what it found, and
// so is one that ends done with an answer the member cannot read: a list it
// cannot read lists nothing, and every pin would be skipped.
func checkPresets(ctx context.Context, gragas, dir string, env []string) (presetCheck, error) {
	res, err := progRun{name: gragas, dir: dir, args: []string{"--json", "preset", "--check"}, env: env, timeout: outputTimeout}.do(ctx)
	if err == nil && res.Code != 0 {
		err = errors.New(exitWords(res.Code))
	}
	return judgePresetCheck(res.Stdout, err, res.Stderr)
}

// judgePresetCheck is the member's verdict on preset --check's run: out is
// its stdout, err how it exited, stderr what it said there. A failed run is
// red with what it said; a run that exited 0 is read strictly, and is red
// unless it is a report that ended done.
func judgePresetCheck(out []byte, err error, stderr []byte) (presetCheck, error) {
	rep, derr := readPresetCheck(out)
	if err != nil {
		return presetCheck{}, fmt.Errorf("the presets are not what the installed plugins make: %w:\n%s", err, rep.failed(derr, out, stderr))
	}
	if derr == nil && rep.Ended != "done" {
		derr = fmt.Errorf("it ended %q with exit status 0", rep.Ended)
	}
	if derr != nil {
		return presetCheck{}, fmt.Errorf("gragas --json preset --check exited 0, and its answer is not a report the member reads, so no preset is held to the mark: %w", derr)
	}
	return rep, nil
}

// presetCheck is what gragas --json preset --check answers, as much of it as
// the member reads: the closing word, the sentence beside it, and the report
// under data. Its fields are coreapi.Result's and coreapi.PresetReport's by
// their JSON names; cmd/gragas-dev may not import internal/core/api
// (layers.yaml, dev-entry), so it reads the document the command writes, the
// contract a project's own gate reads too.
type presetCheck struct {
	Ended string         `json:"ended"`
	Msg   string         `json:"msg"`
	Data  *presetChecked `json:"data"`
}

// presetChecked is the report: each file compared with the tool it is of, the
// paths a project owns (its mark removed), and the tools of
// hand_written_presets. The tools of files and the tools of owned paths are
// the tools preset --check lists: those a rule binds and a plugin serves.
type presetChecked struct {
	Files       []presetCheckedFile `json:"files"`
	Compared    int                 `json:"compared"`
	Owned       []string            `json:"owned"`
	HandWritten []string            `json:"hand_written"`
}

// presetCheckedFile is one file of the report.
type presetCheckedFile struct {
	Path  string `json:"path"`
	Tool  string `json:"tool"`
	State string `json:"state"`
	First string `json:"first"`
}

// readPresetCheck is the document of out. A document with no report under
// data is refused: a run that ended done answers one, and a list the member
// cannot read lists nothing, so no tool would be held.
func readPresetCheck(out []byte) (presetCheck, error) {
	var c presetCheck
	if err := json.Unmarshal(out, &c); err != nil {
		return presetCheck{}, fmt.Errorf("its answer is not one JSON document: %w", err)
	}
	if c.Data == nil || c.Data.Files == nil {
		return c, errors.New("its answer holds no report of files under data")
	}
	return c, nil
}

// lists is whether the report lists tool, whose plugin's preset is rel: a
// file of the tool was compared, or rel is a file the project owns. Both are
// files a rule binds the tool to; a tool no rule binds has neither.
func (r *presetChecked) lists(tool, rel string) bool {
	return slices.ContainsFunc(r.Files, func(f presetCheckedFile) bool { return f.Tool == tool }) || slices.Contains(r.Owned, rel)
}

// said is the member's account of a check that ended done: each tool of
// hand_written_presets, each file the project owns, each pinned plugin no
// rule binds, and the check's own sentence.
func (c presetCheck) said(skipped []string) string {
	var lines []string
	for _, tool := range c.Data.HandWritten {
		lines = append(lines, tool+": hand-written (hand_written_presets), so not compared")
	}
	for _, p := range c.Data.Owned {
		lines = append(lines, p+": the project's (its mark removed), so not compared")
	}
	for _, tool := range skipped {
		lines = append(lines, tool+": pinned, no rule binds it, no preset checked")
	}
	return strings.Join(append(lines, c.Msg), "\n")
}

// failed is what a check that did not end done says: its sentence and each
// file that differs, where, when its answer was read, and what it wrote
// otherwise.
func (c presetCheck) failed(derr error, out, stderr []byte) string {
	if c.Msg == "" {
		raw := strings.TrimSpace(string(out) + "\n" + string(stderr))
		if derr != nil {
			raw += "\n(" + derr.Error() + ")"
		}
		return strings.TrimSpace(raw)
	}
	lines := []string{c.Ended + ": " + c.Msg}
	if c.Data != nil {
		for _, f := range c.Data.Files {
			if f.State != "same" {
				lines = append(lines, f.Path+": "+f.State+": "+f.First)
			}
		}
	}
	if s := strings.TrimSpace(string(stderr)); s != "" {
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}

// buildGragas builds gragas from the module at src into tmp, under the pin,
// with environ, the environment main read, and answers the path of the binary.
func buildGragas(ctx context.Context, environ []string, src, tmp string) (string, error) {
	pin, err := goToolchainPin(src)
	if err != nil {
		return "", err
	}
	exe, err := goPinned(ctx, environ, src, pin, "env", "GOEXE")
	if err != nil {
		return "", err
	}
	gragas := filepath.Join(tmp, "gragas"+strings.TrimSpace(string(exe)))
	// Built under the caller's own environment, before the state folder moves:
	// the build's cache is never the run's folder.
	if _, err := goPinned(ctx, environ, src, pin, "build", "-trimpath", "-o", gragas, "./cmd/gragas"); err != nil {
		return "", err
	}
	return gragas, nil
}

// goToolchainPin is the toolchain the module at src pins: the name on its
// go.mod's toolchain line, which the Taskfile's GOTOOLCHAIN is held to. A
// module with no such line pins none, and the answer is "local".
func goToolchainPin(src string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(src, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("the toolchain pin could not be read: %w", err)
	}
	for l := range strings.SplitSeq(string(raw), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "toolchain" {
			return f[1], nil
		}
	}
	return "local", nil
}

// goPinned runs a go command over the module at dir as goOver does, but under
// the toolchain pin and not the host's own: the machine's go may be older than
// the one go.mod asks for (Homebrew's 1.24 against 1.27), and the build of
// gragas is the one go command of the gate that must be the pinned one.
// It names GOTOOLCHAIN once, the pin, in place of goEnv's local.
func goPinned(ctx context.Context, environ []string, dir, pin string, args ...string) ([]byte, error) {
	return output(ctx, environ, dir, append(withoutVars(goEnv(), []string{"GOTOOLCHAIN"}), "GOTOOLCHAIN="+pin), "go", args...)
}

// pluginPreset is what a plugin's manifest says of its preset: the tool, the
// file's path in the tool's folder of presets/, and whether the plugin is
// asked for the mark.
type pluginPreset struct {
	tool, path string
	mark       bool
}

// markedPresets holds every preset a pinned plugin whose manifest says mark:
// true makes to the mark: the file's first line carries model.PresetMark, the
// text link.Mark answers. gragas preset --check holds a marked file byte for
// byte, and reads a copy-once file whose mark is taken away as the project's,
// whatever it holds; that is the engine's rule for a project, and gragas's own
// gate refuses the opt-out here, naming each file. A file that is not there is preset --check's to
// refuse, and it has, so this runs after it; it is refused here too.
//
// A pinned plugin whose tool rep does not list is skipped and answered by its
// tool: no rule binds the tool, so the book makes no file of it and preset
// --check compares none. The skip is decided by preset --check's own list
// alone, and is for a plugin with no file: anything at an unlisted tool's
// path is a preset nothing holds, and is red. For
// each tool the list names, a file that is not there and a file without the
// mark stay red. It answers how many marked presets it held, and the tools it
// skipped.
func markedPresets(root string, pins []pluginPin, rep *presetChecked) (int, []string, error) {
	n := 0
	var problems, skipped []string
	for _, p := range pins {
		manifest := path.Join("plugins", p.name, p.version, "plugin.yaml")
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifest)))
		if err != nil {
			return 0, nil, fmt.Errorf("%s's manifest could not be read, so its preset is held to nothing: %w", p.ref(), err)
		}
		pre, ok, err := readPluginPreset(raw)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", manifest, err)
		}
		if !ok {
			continue
		}
		rel := path.Join("presets", pre.tool, pre.path)
		switch {
		case slices.Contains(rep.HandWritten, pre.tool):
			// The book binds the tool and a person writes its preset; said
			// names it as hand-written, never as unbound.
			continue
		case !rep.lists(pre.tool, rel):
			// The skip is for a plugin with no file (the ruling's words): a file
			// at the path is one preset --check holds to nothing, so it is red.
			there, err := presetThere(root, rel)
			switch {
			case err != nil:
				return 0, nil, err
			case there:
				problems = append(problems, rel+" is there, and preset --check does not list "+pre.tool+" ("+p.ref()+" is pinned, and the book makes no preset of it), so nothing holds the file to what the plugin makes: take it away, or bind the tool in the rulebook")
			default:
				skipped = append(skipped, pre.tool)
			}
			continue
		}
		if !pre.mark {
			continue
		}
		have, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return 0, nil, fmt.Errorf("%s could not be read to hold it to the mark: %w", rel, err)
		}
		first, _, _ := strings.Cut(string(have), "\n")
		if !strings.Contains(first, model.PresetMark) {
			problems = append(problems, rel+" does not open with the mark, and "+p.ref()+"'s manifest asks for it (mark: true): this repository holds every preset a pinned plugin makes to what the plugin makes, so the mark is never taken away here; take the file away and run gragas preset --apply to write it again")
			continue
		}
		n++
	}
	if len(problems) > 0 {
		return 0, nil, errors.New(strings.Join(problems, "\n"))
	}
	return n, skipped, nil
}

// presetThere tells whether anything is at rel under root: a file, a folder
// or a link, each a preset nothing holds when its tool is not listed. Any
// answer but "not there" that cannot be read is an error, never a skip.
func presetThere(root, rel string) (bool, error) {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("%s could not be looked for, so the skip of its plugin is not decided: %w", rel, err)
	}
}

var (
	// presetTool is a tool's name as a plugin's manifest gives it, and so the
	// folder of presets/ its preset is written in.
	presetTool = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	// presetFile is the preset's path in the tool's folder as the manifest
	// schema holds it: slash-separated steps of letters, digits, ., _ and -,
	// and no step . or .. (presetPath).
	presetFile = regexp.MustCompile(`^[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*$`)
	// bareKey is a line that is a bare key and its value, the one form of a key
	// the member reads; a quoted key is the same key to YAML, and the member
	// would pass over it.
	bareKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*:(?:[ \t]|$)`)
)

// presetPath is whether p is a path of the tool's folder: presetFile, and no
// step that is . or .., so it names no file outside the folder.
func presetPath(p string) bool {
	return presetFile.MatchString(p) && !slices.ContainsFunc(strings.Split(p, "/"), func(s string) bool { return s == "." || s == ".." })
}

// readPluginPreset reads plugin.yaml by lines, as tools.yaml is read: the
// top-level tool: line, and the path: and mark: lines of the preset: block,
// each indented two spaces; a deeper line of the block is not its own. It
// answers false where the manifest has no preset: block. A preset: block in
// another form, a key given twice, a tool that is not one name, a path that
// leaves the tool's folder, and a mark that is not true or false are
// refused, naming the line: a manifest the member cannot read holds nothing,
// and the member is red.
func readPluginPreset(manifest []byte) (pluginPreset, bool, error) {
	var r presetReader
	for i, line := range strings.Split(string(manifest), "\n") {
		if err := r.line(strings.TrimSuffix(line, "\r")); err != nil {
			return pluginPreset{}, false, fmt.Errorf("line %d: %w", i+1, err)
		}
	}
	return r.done()
}

// presetReader is readPluginPreset's state over the lines of one manifest:
// what it has read, whether a line is in the preset: block, and which keys
// it has seen.
type presetReader struct {
	pre                        pluginPreset
	in, seen, tool, file, mark bool
}

// line is one line of the manifest: a comment or a blank is nothing, a
// top-level line is top's, and a line two spaces into the preset: block is
// key's. Either is a bare key, or an item of a list in the block; any other
// form is refused, so no key the member passes over can hold the preset.
func (r *presetReader) line(line string) error {
	k, v, _ := strings.Cut(line, ":")
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	switch {
	case strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#"):
		return nil
	case line[0] != ' ':
		r.in = false
		if !bareKey.MatchString(line) {
			return fmt.Errorf("%q is not a bare key and its value, the one form of a top-level line the member reads", line)
		}
		return r.top(line, k, v)
	case !r.in || strings.HasPrefix(line, "   "):
		return nil
	case !bareKey.MatchString(line[2:]) && !strings.HasPrefix(line[2:], "-"):
		return fmt.Errorf("%q is not a bare key and its value, the one form of a line of the preset block the member reads", line)
	}
	return r.key(line, strings.TrimSpace(k), v)
}

// top is a top-level line: tool: once, and preset: once with nothing after it.
func (r *presetReader) top(line, k, v string) error {
	switch k {
	case "tool":
		if r.tool {
			return errors.New("a second tool key")
		}
		r.tool, r.pre.tool = true, v
	case "preset":
		if r.seen || v != "" {
			return fmt.Errorf("%q is not the one preset block, its keys on the lines below it", line)
		}
		r.in, r.seen = true, true
	}
	return nil
}

// key is a line of the preset: block: path: once, and mark: once, true or
// false.
func (r *presetReader) key(line, k, v string) error {
	switch k {
	case "path":
		if r.file {
			return errors.New("a second path key in the preset block")
		}
		r.file, r.pre.path = true, v
	case "mark":
		if r.mark || (v != "true" && v != "false") {
			return fmt.Errorf("%q is not the one mark of the preset block, true or false", line)
		}
		r.mark, r.pre.mark = true, v == "true"
	}
	return nil
}

// done is what the lines read: no preset where there was no preset: block,
// and the tool and the path held where there was one.
func (r *presetReader) done() (pluginPreset, bool, error) {
	switch {
	case !r.seen:
		return pluginPreset{}, false, nil
	case !presetTool.MatchString(r.pre.tool):
		return pluginPreset{}, false, fmt.Errorf("the tool is %q, and a manifest with a preset block names its tool by one name", r.pre.tool)
	case !presetPath(r.pre.path):
		return pluginPreset{}, false, fmt.Errorf("the preset's path is %q, and it is a path in the tool's folder: slash-separated steps, none of them . or .. alone", r.pre.path)
	}
	return r.pre, true, nil
}
