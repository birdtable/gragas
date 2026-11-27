package main

import "os/exec"

// withArgs adds the words to the command's arguments after the call, so the program stays a constant.
func withArgs(cmd *exec.Cmd, words []string) *exec.Cmd {
	cmd.Args = append(cmd.Args, words...)
	return cmd
}

// host is the system the toolchain builds for here, as go env says it.
type host struct{ goos, goarch string }

func (h host) platform() string { return h.goos + "/" + h.goarch }

// skipDir is whether a folder is no package of the table: tooling, build
// output, fixtures.
func skipDir(name string) bool {
	switch name {
	case ".git", ".tools", "node_modules", "bin", "dist", "times", "testdata", "vendor":
		return true
	}
	return false
}

// pluralWord is one or many, by n.
func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
