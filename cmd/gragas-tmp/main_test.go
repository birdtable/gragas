package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// TestTheCommandPrintsTheFolderOrTheWalksMessage: a folder
// is printed as one line; the walk's refusal prints nothing on stdout, its
// message with its fix on stderr, and exits 1, and so does any other error.
// Planted: the refusal is the one a 775 ~/.cache gives on a host with a group
// for each user. internal/host/trust holds the walk's own rows, run against a
// real group-writable folder.
func TestTheCommandPrintsTheFolderOrTheWalksMessage(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name       string
		make       func() (string, error)
		code       int
		out, errIn string
	}{
		{"a trusted folder", func() (string, error) { return "/t/gragas-1/tmp", nil }, 0, "/t/gragas-1/tmp\n", ""},
		{"a folder the walk refuses", func() (string, error) {
			return "", &hosttrust.Unsafe{Observed: "the folder /h/.cache can be written by its group", Fix: "chmod g-w /h/.cache"}
		}, 1, "", "the temp folder is not trusted: the folder /h/.cache can be written by its group\n  fix: chmod g-w /h/.cache"},
		{"another error", func() (string, error) { return "", errors.New("disk full") }, 1, "", "gragas-tmp: disk full"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var out, errb bytes.Buffer
			code := run(&out, &errb, c.make)
			if code != c.code || out.String() != c.out || !strings.Contains(errb.String(), c.errIn) || (c.errIn == "") != (errb.Len() == 0) {
				t.Errorf("exit %d, stdout %q, stderr %q; want %d, %q, stderr holding %q", code, out.String(), errb.String(), c.code, c.out, c.errIn)
			}
		})
	}
}
