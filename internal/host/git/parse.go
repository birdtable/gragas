package git

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/model"
)

// maxRev is the longest revision this adapter runs.
const maxRev = 255

// parseRev checks a caller's revision on its parsed form and answers it as a
// value for a slot: 1 to 255 bytes of printable ASCII, no space, not
// starting with - or / or ., and none of what makes a name more than a
// name: .., @{, //, /., a backslash, ~ ^ : ? * [, or an end in /, . or
// .lock. A tag, a branch name and a full object name pass. It is run again
// by run at the slot.
func parseRev(s string) (value, error) {
	if s == "" || len(s) > maxRev {
		return value{}, fmt.Errorf("%w: %d bytes", errBadRev, len(s))
	}
	for i := range len(s) {
		if c := s[i]; c <= ' ' || c >= 0x7f || strings.IndexByte(`\~^:?*[`, c) >= 0 {
			return value{}, fmt.Errorf("%w: %q holds %q", errBadRev, s, c)
		}
	}
	switch {
	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "/"), strings.HasPrefix(s, "."):
		return value{}, fmt.Errorf("%w: %q starts with %q", errBadRev, s, s[0])
	case strings.Contains(s, ".."), strings.Contains(s, "@{"), strings.Contains(s, "//"), strings.Contains(s, "/."):
		return value{}, fmt.Errorf("%w: %q", errBadRev, s)
	case strings.HasSuffix(s, "/"), strings.HasSuffix(s, "."), strings.HasSuffix(s, ".lock"):
		return value{}, fmt.Errorf("%w: %q ends as no name does", errBadRev, s)
	}
	return value{kind: kindRev, text: s}, nil
}

// parseOID checks an object name: 40 or 64 lower-case hexadecimal digits,
// SHA-1's or SHA-256's.
func parseOID(s string) (value, error) {
	if len(s) != 40 && len(s) != 64 {
		return value{}, fmt.Errorf("%q is no object name: %d digits", clip([]byte(s)), len(s))
	}
	for i := range len(s) {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return value{}, fmt.Errorf("%q is no object name", clip([]byte(s)))
		}
	}
	return value{kind: kindOID, text: s}, nil
}

// parseOIDLine reads one object name ended by a newline, and nothing else.
func parseOIDLine(out []byte) (value, error) {
	line, ok := bytes.CutSuffix(out, []byte{'\n'})
	if !ok || bytes.IndexByte(line, '\n') >= 0 {
		return value{}, fmt.Errorf("%q is not one line", clip(out))
	}
	return parseOID(string(line))
}

// parseBases reads what merge-base --all prints: object names of one length,
// each ended by a newline. None is an empty list.
func parseBases(out []byte) ([]value, error) {
	return parseOIDLines(out, "a merge base", "the merge bases")
}

// parseOIDLines reads object names of one length, each ended by a newline,
// and nothing else; one and many name a line and the lines in an error.
func parseOIDLines(out []byte, one, many string) ([]value, error) {
	var ids []value
	for len(out) > 0 {
		end := bytes.IndexByte(out, '\n')
		if end < 0 {
			return nil, errors.New(one + " is not ended by a newline")
		}
		v, err := parseOID(string(out[:end]))
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 && len(v.text) != len(ids[0].text) {
			return nil, errors.New(many + " are object names of two lengths")
		}
		ids = append(ids, v)
		out = out[end+1:]
	}
	return ids, nil
}

// checkPath holds a path of a tree to what model.Entry promises, and more:
// relative and slash-separated, UTF-8, no empty, "." or ".." part, no part
// that is .git in any case, and no control character or backslash, which
// a terminal or another system would read as more than a name.
func checkPath(p string) error {
	if p == "" || len(p) > 4096 || !utf8.ValidString(p) {
		return fmt.Errorf("%w: %q", errBadPath, clip([]byte(p)))
	}
	for _, r := range p {
		if unicode.IsControl(r) || r == '\\' {
			return fmt.Errorf("%w: %q holds %q", errBadPath, p, r)
		}
	}
	for part := range strings.SplitSeq(p, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return fmt.Errorf("%w: %q", errBadPath, p)
		}
	}
	return nil
}

// entry is one regular file, symbolic link or submodule of a tree or the
// index.
type entry struct {
	path string
	oid  value
	size int64
	exec bool
	// link is mode 120000, git's own mode field: a symbolic link, whose
	// size is the length of the path it names.
	link bool
	// gitlink is mode 160000: a submodule, the commit it names in oid and
	// no size. Only Commit reads one, to refuse a file written over it or
	// below it; every listing leaves it out.
	gitlink bool
}

func (e entry) model() model.Entry {
	return model.Entry{Path: e.path, Size: e.size, Exec: e.exec, Link: e.link, Object: e.oid.text}
}

// fileMode is the tree's mode field of e, as git writes it: 160000 for a
// submodule, 120000 for a link, 100755 for a file it runs, 100644 for any
// other.
func (e entry) fileMode() string {
	switch {
	case e.gitlink:
		return "160000"
	case e.link:
		return "120000"
	case e.exec:
		return "100755"
	}
	return "100644"
}

// parseSize reads a blob's size: decimal digits, no sign, no leading zero.
func parseSize(s string) (int64, error) {
	if s == "" || len(s) > 18 || s[0] == '0' && len(s) > 1 {
		return 0, fmt.Errorf("%q is no size", s)
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("%q is no size", s)
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// records splits NUL-ended records; output not ended by NUL is an error.
func records(out []byte) ([][]byte, error) {
	if len(out) == 0 {
		return nil, nil
	}
	if out[len(out)-1] != 0 {
		return nil, errors.New("a record is not ended by NUL")
	}
	return bytes.Split(out[:len(out)-1], []byte{0}), nil
}

// fileOf reads the fields every listing shares, a mode, an object name and a
// size, and answers the entry when the mode is a regular file's, a link's
// (120000) or a submodule's (160000), the link and the submodule marked so.
// Only a submodule's size is "-", and its entry's size is 0.
func fileOf(mode, oid, size, path string, width *int) (entry, error) {
	v, err := parseOID(oid)
	if err != nil {
		return entry{}, err
	}
	if *width == 0 {
		*width = len(v.text)
	}
	if len(v.text) != *width {
		return entry{}, errors.New("the object names are of two lengths")
	}
	if err := checkPath(path); err != nil {
		return entry{}, err
	}
	if mode == "160000" {
		if size != "-" {
			return entry{}, fmt.Errorf("the submodule %q has a size", path)
		}
		return entry{path: path, oid: v, gitlink: true}, nil
	}
	n, err := parseSize(size)
	if err != nil {
		return entry{}, err
	}
	switch mode {
	case "100644", "100755":
		return entry{path: path, oid: v, size: n, exec: mode == "100755"}, nil
	case "120000":
		return entry{path: path, oid: v, size: n, link: true}, nil
	}
	return entry{}, fmt.Errorf("the mode %q is no mode of a file", mode)
}

// parseTree reads what tree-files prints: records of "mode type object
// size" with the size padded by spaces, a tab and the path, each ended by
// NUL. -z is what leaves the path unquoted in this form; --format's %(path)
// quotes it whatever -z says, so the listing is not read with --format. It answers the regular
// files, the links and the submodules, in git's order.
//
// git's order is the paths' bytes, each path after the one before it, and
// every lookup in a listing searches it so. A listing out of that order, a
// hand-made tree's, is refused, naming the first path out of place: a search
// would miss what it holds, and a Commit would write over a link it never
// found.
func parseTree(out []byte) ([]entry, error) {
	recs, err := records(out)
	if err != nil {
		return nil, err
	}
	var files []entry
	width := 0
	for _, rec := range recs {
		head, path, ok := strings.Cut(string(rec), "\t")
		f := strings.Fields(head)
		if !ok || len(f) != 4 {
			return nil, fmt.Errorf("%q is no record of a tree", clip(rec))
		}
		if want := map[string]string{"100644": "blob", "100755": "blob", "120000": "blob", "160000": "commit"}[f[0]]; want == "" || f[1] != want {
			return nil, fmt.Errorf("the mode %q and the type %q are no file of a tree", f[0], f[1])
		}
		e, err := fileOf(f[0], f[2], f[3], path, &width)
		if err != nil {
			return nil, err
		}
		if n := len(files); n > 0 && files[n-1].path >= e.path {
			return nil, fmt.Errorf("the tree lists %q after %q, out of git's order", clip([]byte(e.path)), clip([]byte(files[n-1].path)))
		}
		files = append(files, e)
	}
	return files, nil
}

// parseIndex reads what index-files prints: records of "mode object size
// stage", a tab and the path, each ended by NUL. It answers the regular
// files, the links and the submodules at stage 0, and whether any path is at another stage, a merge in
// progress.
func parseIndex(out []byte) (files []entry, unmerged bool, err error) {
	recs, err := records(out)
	if err != nil {
		return nil, false, err
	}
	width := 0
	seen := map[string]bool{}
	for _, rec := range recs {
		head, path, ok := strings.Cut(string(rec), "\t")
		f := strings.Split(head, " ")
		if !ok || len(f) != 4 || len(f[3]) != 1 || f[3][0] < '0' || f[3][0] > '3' {
			return nil, false, fmt.Errorf("%q is no record of the index", clip(rec))
		}
		e, err := fileOf(f[0], f[1], f[2], path, &width)
		if err != nil {
			return nil, false, err
		}
		if f[3] != "0" {
			unmerged = true
			continue
		}
		if seen[path] {
			return nil, false, fmt.Errorf("the index holds %q twice", path)
		}
		seen[path] = true
		files = append(files, e)
	}
	return files, unmerged, nil
}

// parseRange reads what range-commits prints: one full object name of
// width digits a line, each once. None is an empty list.
func parseRange(out []byte, width int) ([]value, error) {
	var ids []value
	seen := map[string]bool{}
	for len(out) > 0 {
		end := bytes.IndexByte(out, '\n')
		if end < 0 {
			return nil, errors.New("a commit of the range is not ended by a newline")
		}
		c, err := parseOID(string(out[:end]))
		if err != nil {
			return nil, err
		}
		out = out[end+1:]
		if len(c.text) != width {
			return nil, fmt.Errorf("the commit %s is not of the range's length", c.text)
		}
		if seen[c.text] {
			return nil, fmt.Errorf("the commit %s is listed twice", c.text)
		}
		seen[c.text] = true
		ids = append(ids, c)
	}
	return ids, nil
}

// errHidden is a commit whose message cannot be read whole: git log would
// answer it re-coded from the encoding its author's header names, or cut
// at a NUL, and a reader of the text would read less than the object holds.
var errHidden = errors.New("a commit's message cannot be read whole")

// parseCommits reads what commit-objects prints for ids, in their order:
// for each a line of its name, "commit" and its size, then its object's
// bytes and a newline. A message is the bytes after the header's blank
// line, as stored. A message under a header that names an encoding other
// than UTF-8, or one holding a NUL, is errHidden naming the commit.
func parseCommits(out []byte, ids []value) ([]model.Message, error) {
	msgs := make([]model.Message, 0, len(ids))
	for _, id := range ids {
		end := bytes.IndexByte(out, '\n')
		if end < 0 {
			return nil, errors.New("an object is not headed by a line")
		}
		f := strings.Split(string(out[:end]), " ")
		if len(f) != 3 || f[0] != id.text || f[1] != "commit" {
			return nil, fmt.Errorf("%q does not head the commit %s", clip(out[:end]), id.text)
		}
		size, err := strconv.Atoi(f[2])
		if err != nil || size < 0 || size >= len(out)-end-1 || out[end+1+size] != '\n' {
			return nil, fmt.Errorf("the commit %s is not of the size its line gives", id.text)
		}
		text, err := stored(out[end+1 : end+1+size])
		if err != nil {
			return nil, fmt.Errorf("%w: the commit %s: %w", errHidden, id.text, err)
		}
		msgs = append(msgs, model.Message{Commit: id.text, Text: text})
		out = out[end+2+size:]
	}
	if len(out) > 0 {
		return nil, errors.New("more than the commits asked for was printed")
	}
	return msgs, nil
}

// parseBlobs reads what cat-file --batch prints for the files named on its
// stdin: for each, in order, a line of the object's name, "blob" and its
// size, the content, and a newline. A file git says is missing is an error,
// never an empty file; a line that names another object or type than the
// file's, a size other than the listing's, or a size the bytes that follow do
// not carry, is an error, and so are bytes left after the last file.
func parseBlobs(out []byte, files []entry) ([][]byte, error) {
	blobs := make([][]byte, 0, len(files))
	for _, f := range files {
		end := bytes.IndexByte(out, '\n')
		if end < 0 {
			return nil, errors.New("an object is not headed by a line")
		}
		fields := strings.Split(string(out[:end]), " ")
		if len(fields) == 2 && fields[0] == f.oid.text && fields[1] == "missing" {
			return nil, fmt.Errorf("git has not the object %s the tree lists for %q", f.oid.text, f.path)
		}
		if len(fields) != 3 || fields[0] != f.oid.text || fields[1] != "blob" {
			return nil, fmt.Errorf("%q does not head the blob %s", clip(out[:end]), f.oid.text)
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || fields[2] != strconv.FormatInt(size, 10) || size != f.size {
			return nil, fmt.Errorf("%q is %q bytes, and the listing says %d", f.path, fields[2], f.size)
		}
		if size >= int64(len(out)-end-1) || out[end+1+int(size)] != '\n' {
			return nil, fmt.Errorf("the blob %s is not of the size its line gives", f.oid.text)
		}
		// a copy, never nil: an empty file is an empty slice
		blobs = append(blobs, append([]byte{}, out[end+1:end+1+int(size)]...))
		out = out[end+2+int(size):]
	}
	if len(out) > 0 {
		return nil, errors.New("more than the blobs asked for was printed")
	}
	return blobs, nil
}

// stored is the message of a commit object: the bytes after the first
// blank line, as they are. A header that names an encoding other than
// UTF-8, which git reads as utf-8 or utf8 in any case, or a NUL in the
// message, is an error.
func stored(obj []byte) (string, error) {
	head, msg, _ := bytes.Cut(obj, []byte("\n\n"))
	for line := range bytes.SplitSeq(head, []byte("\n")) {
		enc, ok := bytes.CutPrefix(line, []byte("encoding "))
		if ok && !strings.EqualFold(string(enc), "utf-8") && !strings.EqualFold(string(enc), "utf8") {
			return "", fmt.Errorf("its header names the encoding %q, and its message is read only as stored, in UTF-8", clip(enc))
		}
	}
	if bytes.IndexByte(msg, 0) >= 0 {
		return "", errors.New("its message holds a NUL, at which git log stops")
	}
	return string(msg), nil
}

// tagsPrefix is where every tag's ref is.
const tagsPrefix = "refs/tags/"

// parseTags reads what tags prints: lines of the ref, its object's type and
// name, and the peeled type and name, separated by NUL. A tag of a commit is
// that commit; a tag of a tag is the commit git peels it to. A tag that
// peels to anything but a commit is an error naming it.
func parseTags(out []byte) ([]model.Tag, error) {
	var tags []model.Tag
	width := 0
	for len(out) > 0 {
		end := bytes.IndexByte(out, '\n')
		if end < 0 {
			return nil, errors.New("a tag is not ended by a newline")
		}
		f := strings.Split(string(out[:end]), "\x00")
		out = out[end+1:]
		if len(f) != 5 {
			return nil, fmt.Errorf("%q is no line of the tags", clip([]byte(f[0])))
		}
		name, ok := strings.CutPrefix(f[0], tagsPrefix)
		if !ok || checkPath(name) != nil {
			return nil, fmt.Errorf("%q is no tag's ref", clip([]byte(f[0])))
		}
		commit, err := peeled(name, f[1:], &width)
		if err != nil {
			return nil, err
		}
		tags = append(tags, model.Tag{Name: name, Commit: commit})
	}
	return tags, nil
}

// peeled is the commit of the tag name from its type, object, peeled type
// and peeled object.
func peeled(name string, f []string, width *int) (string, error) {
	typ, oid := f[0], f[1]
	switch {
	case typ == "commit" && f[2] == "" && f[3] == "":
	case typ == "tag" && f[2] == "commit":
		oid = f[3]
	case typ == "tag":
		return "", fmt.Errorf("the tag %q names a %s, not a commit", name, clip([]byte(f[2])))
	default:
		return "", fmt.Errorf("the tag %q names a %s, not a commit", name, clip([]byte(typ)))
	}
	v, err := parseOID(oid)
	if err != nil {
		return "", err
	}
	if *width == 0 {
		*width = len(v.text)
	}
	if len(v.text) != *width {
		return "", errors.New("the object names are of two lengths")
	}
	return v.text, nil
}

// parsePath checks a path for a slot: absolute and clean on this system,
// UTF-8, at most 4096 bytes, and no control character, which an argv or
// a config value would carry as more than a name.
func parsePath(s string) (value, error) {
	if s == "" || len(s) > 4096 || !utf8.ValidString(s) || !filepath.IsAbs(s) || filepath.Clean(s) != s {
		return value{}, fmt.Errorf("%w: %q is not an absolute, clean path", errBadPath, clip([]byte(s)))
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return value{}, fmt.Errorf("%w: %q holds %q", errBadPath, s, r)
		}
	}
	return value{kind: kindPath, text: s}, nil
}

// parseTag checks the name of a tag on its parsed form: a revision as
// parseRev holds one, that is not HEAD and does not start with refs/, so
// it names one tag and never a ref of another kind.
func parseTag(s string) (value, error) {
	r, err := parseRev(s)
	if err != nil {
		return value{}, err
	}
	if s == "HEAD" || strings.HasPrefix(s, "refs/") || s == "@" {
		return value{}, fmt.Errorf("%w: %q is no tag name", errBadRev, s)
	}
	return value{kind: kindTag, text: r.text}, nil
}
