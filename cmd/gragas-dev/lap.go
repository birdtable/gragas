package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/dev/say"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

// A tier's laps are kept in the tools folder, in a folder of their own: one small file a begin or an end, named
// for the word and the member (a name's slash is escaped), holding the instant. Each file is made whole by
// internal/host/fs's Writer.Replace (gate/shared.yaml, the atomic-write owner), so members that begin and end
// together never share a file, and nothing is ever appended to (gragas#1710, G2). reset removes them, report
// reads them in the order of their instants.
const lapFolder = "lap"

// runLap is `lap [--dir folder] reset | begin name | end name | report`: the push tier times its own
// members through it. A member's first command is `lap begin <task>` and its last (a defer, so a red
// member is timed too) is `lap end <task>`; the tier's start is `lap reset` and its end, green or red,
// is `lap report`, the members slowest first. Timing is a note, never a check: every word but a missing
// or unknown one ends 0, and a log that cannot be read or written is said on stderr and changes
// nothing, so a lap can never fail a tier. A member that began and never ended (it failed, or the
// tier was killed) is reported as unfinished, with the time it had run.
func runLap(ctx context.Context, args []string, now func() time.Time, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("lap", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".tools", "the folder the laps are kept in")
	if err := fl.Parse(args); err != nil || fl.NArg() == 0 {
		talk.Text(usage)
		return exitUsage
	}
	word, rest := fl.Arg(0), fl.Args()[1:]
	if !slices.Contains([]string{"reset", "begin", "end", "report"}, word) ||
		(word == "begin" || word == "end") != (len(rest) == 1) || (word == "reset" || word == "report") && len(rest) != 0 {
		talk.Text(usage)
		return exitUsage
	}
	// A folder that is not there is a tier with no tools: nothing to time, and no failure.
	tree := &hostfs.Tree{}
	defer func() {
		if err := tree.Close(); err != nil {
			talk.Text("gragas-dev lap: not timed: " + err.Error() + "\n")
		}
	}()
	root, err := tree.Open(*dir)
	if err != nil {
		talk.Text("gragas-dev lap: not timed: " + err.Error() + "\n")
		return 0
	}
	w := hostfs.Writer{Tree: tree}
	var done error
	switch word {
	case "reset":
		done = lapReset(tree, w, root)
	case "begin", "end":
		if strings.ContainsAny(rest[0], " \t\n") || rest[0] == "" {
			talk.Text("gragas-dev lap: not timed: a member's name holds no space\n")
			return 0
		}
		done = w.Replace(root, lapFolder+"/"+word+"."+url.PathEscape(rest[0]), []byte(strconv.FormatInt(now().UnixNano(), 10)))
	case "report":
		var log string
		if log, done = lapLogOf(tree, root); done == nil {
			(&say.Speaker{W: stdout}).Text(lapReport(log, now()))
		}
	}
	if done != nil {
		talk.Text("gragas-dev lap: not timed: " + done.Error() + "\n")
	}
	return 0
}

// lap is one member's time: how long it ran, and whether it ended.
type lap struct {
	name     string
	ran      time.Duration
	finished bool
}

// lapTimes are the instants a log holds: when each member last began, when it last ended (a member
// that began again has no end until it ends again), the members in the order they first began, and the
// first begin and the last end.
type lapTimes struct {
	begun, ended map[string]int64
	order        []string
	first, last  int64
}

// lapRead reads the lines of a log. A line it cannot read (a torn write, a word it does not know, an
// end with no begin) is skipped.
func lapRead(log string) lapTimes {
	t := lapTimes{begun: map[string]int64{}, ended: map[string]int64{}}
	sc := bufio.NewScanner(strings.NewReader(log))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 {
			continue
		}
		at, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			continue
		}
		t.add(f[0], f[1], at)
	}
	return t
}

func (t *lapTimes) add(word, name string, at int64) {
	_, began := t.begun[name]
	switch {
	case word == "begin":
		if !began {
			t.order = append(t.order, name)
		}
		t.begun[name] = at
		delete(t.ended, name)
		if t.first == 0 || at < t.first {
			t.first = at
		}
	case word == "end" && began:
		t.ended[name] = at
		t.last = max(t.last, at)
	}
}

// lapReport is what `lap report` prints: each member's wall time, slowest first, an unfinished member
// marked, and the span from the first begin to the last end or to now. It prints nothing for a log with
// no member in it, so a tier that timed nothing says nothing.
func lapReport(log string, now time.Time) string {
	t := lapRead(log)
	if len(t.order) == 0 {
		return ""
	}
	laps := make([]lap, 0, len(t.order))
	for _, name := range t.order {
		l := lap{name: name, ran: max(0, now.Sub(time.Unix(0, t.begun[name])))}
		if end, ok := t.ended[name]; ok {
			l.ran, l.finished = time.Duration(end-t.begun[name]), true
		}
		laps = append(laps, l)
	}
	slices.SortStableFunc(laps, func(a, b lap) int { return int(b.ran - a.ran) })
	if t.last == 0 {
		t.last = now.UnixNano()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "lap: %d members, slowest first; the tier ran %s from the first begin\n", len(laps), roundLap(time.Duration(t.last-t.first)))
	for _, l := range laps {
		note := ""
		if !l.finished {
			note = "  (never ended: red, or killed)"
		}
		fmt.Fprintf(&b, "lap: %8s  %s%s\n", roundLap(l.ran), l.name, note)
	}
	return b.String()
}

// roundLap shows a time to a tenth of a second, which is what a member's wall time is good to.
func roundLap(d time.Duration) string { return d.Round(100 * time.Millisecond).String() }

// lapReset removes every file of the lap folder, so the tier starts with no member timed.
func lapReset(tree *hostfs.Tree, w hostfs.Writer, root model.Root) error {
	// The log of an earlier version was one file of this name: it goes, so a folder can take its place.
	if st, err := tree.Stat(root, lapFolder); err == nil && !st.Dir {
		return w.Remove(root, lapFolder)
	}
	entries, err := tree.List(root, lapFolder, model.Every)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Dir {
			continue
		}
		if err := w.Remove(root, e.Path); err != nil {
			return err
		}
	}
	return nil
}

// lapLogOf reads the lap folder as the lines lapRead reads, "word name instant", in the order of the instants (a
// file it cannot read or name it cannot split is left out: a lap is a note).
func lapLogOf(tree *hostfs.Tree, root model.Root) (string, error) {
	entries, err := tree.List(root, lapFolder, model.Every)
	if err != nil {
		return "", err
	}
	type event struct {
		line string
		at   int64
	}
	var events []event
	for _, e := range entries {
		word, name, ok := strings.Cut(strings.TrimPrefix(e.Path, lapFolder+"/"), ".")
		name, uerr := url.PathUnescape(name)
		if e.Dir || !ok || uerr != nil {
			continue
		}
		data, rerr := tree.Read(root, e.Path)
		at, perr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if rerr != nil || perr != nil {
			continue
		}
		events = append(events, event{fmt.Sprintf("%s %s %d\n", word, name, at), at})
	}
	slices.SortStableFunc(events, func(a, b event) int { return int(a.at - b.at) })
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.line)
	}
	return b.String(), nil
}
