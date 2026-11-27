// Command standin is the stand-in enforcer the Steps adapter's tests run: a built program that prints what an
// enforcer prints. It reads its arguments and the one fixture they name, and nothing else (it was a
// shell script, which no system but Unix runs).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "--version":
		fmt.Println("standin version 1.4.2 (a stand-in)")
	case "print-config":
		fmt.Println(`{"rules":{"no-bad":["error",{}],"no-todo":["warn",{}]}}`)
	case "run":
		// run --out <output> <fixtures>
		if len(args) < 4 {
			return 2
		}
		f, err := os.Open(args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, "standin:", err)
			return 2
		}
		defer func() { _ = f.Close() }()
		var issues []string
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			if strings.Contains(sc.Text(), "BAD") {
				issues = append(issues, fmt.Sprintf(`{"rule":"no-bad","file":"%s","line":%d}`, filepath.Base(args[3]), n))
			}
		}
		fmt.Printf("{\"issues\":[%s]}\n", strings.Join(issues, ","))
		if len(issues) > 0 {
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "standin: no command %s\n", cmd)
		return 2
	}
	return 0
}
