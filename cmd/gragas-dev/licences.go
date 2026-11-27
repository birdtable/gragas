package main

import (
	"context"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"os"
	"regexp"
	"strings"
)

const licencesUsage = `usage: gragas-dev licences list allowlist | ignore sources allowlist
`

// licenceID is a line of gate/licences.txt that names an SPDX id (the rest are comments and blanks).
var licenceID = regexp.MustCompile(`^[A-Za-z0-9.+-]+$`)

// runLicences is the licences command. list prints the ids of the allowlist file as the one
// comma-joined value go-licenses takes, and is red when the file names none: an empty
// allowlist is no list. It replaces a grep piped into paste in the Taskfile, which is not the
// same program on every system (gragas#1228).
func runLicences(_ context.Context, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	listed := len(args) == 2 && args[0] == "list"
	ignored := len(args) == 3 && args[0] == "ignore"
	if !listed && !ignored {
		talk.Text(licencesUsage)
		return exitUsage
	}
	if args[0] == "ignore" {
		return runIgnore(args[1], args[2], stdout, talk)
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		talk.Text("gragas-dev licences: " + err.Error() + "\n")
		return exitFailed
	}
	var ids []string
	for l := range strings.SplitSeq(string(data), "\n") {
		if l = strings.TrimSpace(l); licenceID.MatchString(l) {
			ids = append(ids, l)
		}
	}
	if len(ids) == 0 {
		talk.Text("gragas-dev licences: " + args[1] + " names no licence, so the check is red\n")
		return exitFailed
	}
	(&say.Speaker{W: stdout}).Text(strings.Join(ids, ",") + "\n")
	return exitDone
}

// sourceRow is a row of gate/licence-sources.txt: a module go-licenses cannot find a licence file
// of, and where its licence is.
type sourceRow struct {
	module, version, commit, id, sum, issue string
}

// sourceRows reads the rows: six fields each, a line of any other shape is red.
func sourceRows(text string) ([]sourceRow, error) {
	var rows []sourceRow
	for i, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) != 6 || strings.ContainsAny(f[0], "*?[") {
			return nil, fmt.Errorf("line %d is not <module> <version> <commit> <id> <sha256> <issue>, with no wildcard: %q", i+1, l)
		}
		rows = append(rows, sourceRow{f[0], f[1], f[2], f[3], f[4], f[5]})
	}
	return rows, nil
}

// takesRows reads the "<module> takes <id>" rows of the allowlist: a module whose licence file
// offers a choice (an OR), and the side taken. The side must be a plain id of the same list, and
// the module is named with no wildcard; a takes line of any other shape is red.
func takesRows(text string) ([]string, error) {
	ids := map[string]bool{}
	type row struct {
		line       int
		module, id string
	}
	var rows []row
	for i, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if licenceID.MatchString(l) {
			ids[l] = true
			continue
		}
		f := strings.Fields(l)
		if len(f) < 2 || f[1] != "takes" {
			continue
		}
		if len(f) != 3 || strings.ContainsAny(f[0], "*?[") {
			return nil, fmt.Errorf("line %d is not <module> takes <id>, with no wildcard: %q", i+1, l)
		}
		rows = append(rows, row{i + 1, f[0], f[2]})
	}
	mods := make([]string, 0, len(rows))
	for _, r := range rows {
		if !ids[r.id] {
			return nil, fmt.Errorf("line %d: %s takes %s, which is not a plain id of the list", r.line, r.module, r.id)
		}
		mods = append(mods, r.module)
	}
	return mods, nil
}

// runIgnore prints the modules go-licenses passes over, comma-joined as its --ignore takes them:
// the modules of the sources file, then each module the allowlist takes one side of a choice for,
// since go-licenses names the other side (gragas#1455: libcap's psx, by go-landlock under
// go-proc's sandbox, is "BSD-3-Clause OR GPL-2.0-only" and is reported GPL-2.0, as go-proc's own
// check passes it over, go-proc#28). Joined here and never in the shell, so no empty entry, a
// prefix of every path, can reach --ignore. Two empty files print nothing and pass.
func runIgnore(sources, allowlist string, stdout io.Writer, talk *say.Speaker) int {
	data, err := os.ReadFile(sources)
	if err != nil {
		talk.Text("gragas-dev licences: " + err.Error() + "\n")
		return exitFailed
	}
	rows, err := sourceRows(string(data))
	if err != nil {
		talk.Text("gragas-dev licences: " + sources + ": " + err.Error() + "\n")
		return exitFailed
	}
	list, err := os.ReadFile(allowlist)
	if err != nil {
		talk.Text("gragas-dev licences: " + err.Error() + "\n")
		return exitFailed
	}
	taken, err := takesRows(string(list))
	if err != nil {
		talk.Text("gragas-dev licences: " + allowlist + ": " + err.Error() + "\n")
		return exitFailed
	}
	mods := make([]string, 0, len(rows)+len(taken))
	for _, r := range rows {
		mods = append(mods, r.module)
	}
	mods = append(mods, taken...)
	(&say.Speaker{W: stdout}).Text(strings.Join(mods, ",") + "\n")
	return exitDone
}
