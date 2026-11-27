package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/host/treepin"
)

// browsersFolder is where Playwright's own install places the browsers, under
// the repository's .tools (task tools sets PLAYWRIGHT_BROWSERS_PATH to it).
const browsersFolder = ".tools/playwright"

// runBrowsers is `browsers`: the folder Playwright's own install placed held to
// the tree digest tools.yaml pins for the platform, before a test starts a
// browser from it (supply.provenance.7). With --print it prints the digest of
// the folder and checks nothing, for a person who bumps the pin.
func runBrowsers(args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("browsers", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the repository holding tools.yaml and .tools/")
	platform := fl.String("platform", "", "the platform whose pin is read, as GOOS/GOARCH")
	print := fl.Bool("print", false, "print the folder's digest and check nothing")
	unmeasuredOK := fl.Bool("unmeasured-ok", false, "end green, saying so, on a system whose row says unmeasured (task tools places no browser there)")
	rest, err := parseInterspersed(fl, args)
	if err != nil || len(rest) != 0 || *platform == "" {
		talk.Text(usage)
		return exitUsage
	}
	folder := filepath.Join(*dir, filepath.FromSlash(browsersFolder))
	if *print {
		sum, err := treepin.Sum(folder)
		if err != nil {
			talk.Text("gragas-dev browsers: " + err.Error() + "\n")
			return exitFailed
		}
		(&say.Speaker{W: stdout}).Text(sum + "\n")
		return exitDone
	}
	yaml, err := os.ReadFile(filepath.Join(*dir, "tools.yaml"))
	if err != nil {
		talk.Text("gragas-dev browsers: " + err.Error() + "\n")
		return exitFailed
	}
	pin, err := readBrowsersPin(yaml, *platform)
	if err == nil {
		err = treepin.Check(folder, pin)
	}
	if un := (*unmeasuredError)(nil); *unmeasuredOK && errors.As(err, &un) {
		(&say.Speaker{W: stdout}).Text("browsers: " + un.Error() + ": no browser is placed, and the members that run one refuse here\n")
		return exitDone
	}
	if err != nil {
		talk.Text("gragas-dev browsers: " + err.Error() + "; run task tools, and never run a browser from the folder\n")
		return exitFailed
	}
	(&say.Speaker{W: stdout}).Text("browsers: " + browsersFolder + " is the tree tools.yaml pins for " + *platform + "\n")
	return exitDone
}

// readBrowsersPin is the tree digest of the browsers: block of tools.yaml for
// platform, a line "    <platform>: {tree: browsers2:<hex>}" under "browsers:". A system
// whose browsers are not pinned has a row of its own, "    <platform>: {unmeasured: <why>}",
// and the step refuses it by name: a system is a key of the data, never a branch here.
func readBrowsersPin(yaml []byte, platform string) (string, error) {
	in := false
	for line := range strings.SplitSeq(string(yaml), "\n") {
		switch {
		case line == "browsers:":
			in = true
		case in && line != "" && line[0] != ' ' && line[0] != '#':
			in = false
		case in && strings.HasPrefix(line, "  platforms:"):
		case in && strings.HasPrefix(line, "    "+platform+":"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "    "+platform+":"))
			if why, ok := strings.CutPrefix(v, "{unmeasured:"); ok {
				return "", &unmeasuredError{platform: platform, why: strings.TrimSuffix(why, "}")}
			}
			v = strings.TrimSuffix(strings.TrimPrefix(v, "{tree:"), "}")
			// A pin of an older format is read too, and treepin.Check refuses
			// it by name: measure again (gragas#1451).
			v = strings.TrimSpace(v)
			for _, prefix := range append([]string{treepin.Prefix}, treepin.Older()...) {
				if len(v) == len(prefix)+64 && strings.HasPrefix(v, prefix) {
					return v, nil
				}
			}
			return "", fmt.Errorf("tools.yaml's browsers pin for %s is not a browsers digest (%s and 64 hex)", platform, treepin.Prefix)
		}
	}
	return "", fmt.Errorf("tools.yaml pins no browsers for %s", platform)
}

// unmeasuredError is a row that says its system's browsers are not pinned. Only task tools reads it as green
// (--unmeasured-ok); a member that runs a browser refuses with it by name.
type unmeasuredError struct{ platform, why string }

func (e *unmeasuredError) Error() string {
	return "tools.yaml's browsers row for " + e.platform + " says its tree is not measured:" + e.why + "; measure it on that system with gragas-dev browsers --print --platform " + e.platform + " and put the digest in the row"
}
