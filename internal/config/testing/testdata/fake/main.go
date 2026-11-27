// Command fake is the stand-in program of a test (gragas#1630, gragas#2282): go-testkit's stand-in, built
// from its fake.Main with the steps only gragas has as handlers, so a test puts a program under a name in a
// folder with fake.From(...).Program, on every system, and no test writes a shell script. It is built by
// configtest.Fake and by nothing else; it sits under testdata so no tool of the tree lists it as a program
// of gragas.
//
// The library's steps (out, err, exit, write, append, args, env, pid, exec, flood, spin, hold and the cases)
// are the library's. A step of these names is gragas's own and is a Custom step of the library:
//
//	version     with "--version" as its first argument, it says the line and ends with 0 (after writing "x"
//	            to the path in mark, when there is one); with another first argument it goes on
//	cat-last    a path it appends "config <last argument>" to, then the lines of the file the last argument names
//	script      the line script=<first argument>, then the other arguments one to a line
//	empty-cwd   ends it with code 7 when the folder it runs in holds anything
//	stdin       its standard input copied to stdout, to its end
//	under       the name of an environment variable holding a folder: with a file name and text, it makes the
//	            folder and writes the file there (exit 9 when the variable is empty or the write fails); with
//	            a lock, it makes that folder there, waits and removes it, and exits 3 saying so when the folder
//	            is already there; with stuck, it makes a folder with a read-only file in it
//	only-first  ends it with code 0 unless its first argument is this word, so later steps are for it alone
//	write-args  a path it writes its arguments, its first argument or its environment to
//	run-path    a program it starts by name through PATH, its output dropped
//	zero-err    that many zero bytes on stderr
//	env-err     its environment on stderr, sorted, each variable ended by the byte 1
//	vars-to     a path and variable names (JSON): the values of the variables joined by |, "unset" for one it has not, written to the path
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/birdtable/go-testkit/fake"
)

func main() {
	fake.Main(map[string]func(string, []string) (int, bool){
		"version":    version,
		"cat-last":   catLast,
		"script":     script,
		"empty-cwd":  emptyCwd,
		"stdin":      stdin,
		"under":      under,
		"only-first": onlyFirst,
		"write-args": writeArgs,
		"run-path":   runPath,
		"zero-err":   zeroErr,
		"env-err":    envErr,
		"vars-to":    varsTo,
	})
}

func fail(what string, err error) (int, bool) {
	fmt.Fprintln(os.Stderr, "fake:", what+":", err)
	return 125, true
}

func version(arg string, args []string) (int, bool) {
	if len(args) == 0 || args[0] != "--version" {
		return 0, false
	}
	var v struct{ Line, Mark string }
	if err := json.Unmarshal([]byte(arg), &v); err != nil {
		return fail("its version step", err)
	}
	if v.Mark != "" {
		if err := os.WriteFile(v.Mark, []byte("x"), 0o600); err != nil {
			return fail("its mark "+v.Mark, err)
		}
	}
	fmt.Fprintln(os.Stdout, v.Line)
	return 0, true
}

func catLast(path string, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	last := args[len(args)-1]
	body, err := os.ReadFile(last) //nolint:gosec // G304: the file the call names
	if err != nil {
		return fail("its last argument "+last, err)
	}
	text := "config " + last + "\n" + string(body)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := appendTo(path, text); err != nil {
		return fail("its log "+path, err)
	}
	return 0, false
}

func script(_ string, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	fmt.Fprintln(os.Stdout, "script="+args[0])
	for _, a := range args[1:] {
		fmt.Fprintln(os.Stdout, a)
	}
	return 0, false
}

func emptyCwd(string, []string) (int, bool) {
	if entries, err := os.ReadDir("."); err != nil || len(entries) > 0 {
		return 7, true
	}
	return 0, false
}

func stdin(string, []string) (int, bool) {
	if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
		return fail("its stdin", err)
	}
	return 0, false
}

func onlyFirst(word string, args []string) (int, bool) {
	if len(args) == 0 || args[0] != word {
		return 0, true
	}
	return 0, false
}

// under is the arg of an under step: env names the variable that holds the folder.
func under(arg string, _ []string) (int, bool) {
	var u struct {
		Env, Write, Text, Lock string
		MS                     int
		Stuck                  bool
	}
	if err := json.Unmarshal([]byte(arg), &u); err != nil {
		return fail("its under step", err)
	}
	dir := os.Getenv(u.Env)
	if dir == "" {
		return 9, true
	}
	switch {
	case u.Lock != "":
		d := filepath.Join(dir, u.Lock)
		if err := os.Mkdir(d, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, "parallel program is running: "+d)
			return 3, true
		}
		time.Sleep(time.Duration(u.MS) * time.Millisecond)
		_ = os.Remove(d)
	case u.Stuck:
		d := filepath.Join(dir, u.Write)
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fail("its folder "+d, err)
		}
		if err := os.WriteFile(filepath.Join(d, "x"), []byte(u.Text), 0o600); err != nil {
			return 9, true
		}
		if err := os.Chmod(d, 0o555); err != nil {
			return 9, true
		}
	default:
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fail("its folder "+dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, u.Write), []byte(u.Text), 0o600); err != nil {
			return 9, true
		}
	}
	return 0, false
}

// writeArgs is the arg of a write-args step: what names the text, one of args, first and env, and
// whether it is appended.
func writeArgs(arg string, args []string) (int, bool) {
	var w struct {
		Path, What string
		Append     bool
	}
	if err := json.Unmarshal([]byte(arg), &w); err != nil {
		return fail("its write step", err)
	}
	var text string
	switch w.What {
	case "env":
		text = strings.Join(os.Environ(), "\n") + "\n"
	case "args":
		text = strings.Join(args, "\n") + "\n"
	case "first":
		first := ""
		if len(args) > 0 {
			first = args[0]
		}
		text = first + "\n"
	}
	if w.Append {
		if err := appendTo(w.Path, text); err != nil {
			return fail("its file "+w.Path, err)
		}
		return 0, false
	}
	if err := os.WriteFile(w.Path, []byte(text), 0o600); err != nil { //nolint:gosec // G304: a path the test named
		return fail("its file "+w.Path, err)
	}
	return 0, false
}

func runPath(arg string, _ []string) (int, bool) {
	var argv []string
	if err := json.Unmarshal([]byte(arg), &argv); err != nil || len(argv) == 0 {
		return fail("its run step", fmt.Errorf("%v", err))
	}
	_ = exec.Command(argv[0], argv[1:]...).Run() //nolint:gosec // G204: the program a test named
	return 0, false
}

func zeroErr(arg string, _ []string) (int, bool) {
	n, err := strconv.Atoi(arg)
	if err != nil {
		return fail("its zero-err step", err)
	}
	block := make([]byte, 32*1024)
	for n > 0 {
		k := min(n, len(block))
		if _, err := os.Stderr.Write(block[:k]); err != nil {
			break
		}
		n -= k
	}
	return 0, false
}

func envErr(string, []string) (int, bool) {
	env := os.Environ()
	slices.Sort(env)
	for _, kv := range env {
		fmt.Fprint(os.Stderr, kv+"\x01")
	}
	return 0, false
}

func varsTo(arg string, _ []string) (int, bool) {
	var v struct {
		Path  string
		Names []string
	}
	if err := json.Unmarshal([]byte(arg), &v); err != nil {
		return fail("its vars-to argument", err)
	}
	vals := make([]string, len(v.Names))
	for i, name := range v.Names {
		x, ok := os.LookupEnv(name)
		if !ok {
			x = "unset"
		}
		vals[i] = x
	}
	if err := os.WriteFile(v.Path, []byte(strings.Join(vals, "|")), 0o600); err != nil {
		return fail("its file "+v.Path, err)
	}
	return 0, false
}

// appendTo adds text to the file at path, read and written back: the one open that appends is
// Writer.Append's (store.state-files.5), and this program has one writer at a time.
func appendTo(path, text string) error {
	old, err := os.ReadFile(path) //nolint:gosec // G304: a path the test named
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, append(old, text...), 0o600) //nolint:gosec // G304: a path the test named
}
