package trust

import (
	"testing"

	shell "github.com/birdtable/go-console/shell"
	gotrust "github.com/birdtable/go-files/trust"
)

// TestNoFixLineRunsPartOfAPathAPersonPastes: a
// fix line, and a hand-over line, is printed only as go-console quotes it, so a
// path's own `&`, `|`, `;`, `$(` or `${` never reaches a shell bare. Beside a
// character go-console quotes for no shell (a per-cent cmd expands, a tab,
// U+180E drawn as nothing, U+202E that turns the screen around) the step is no
// line at all, and the caller says what to do in words; with none beside it,
// the line is exactly go-console's for the step, which reads back as its argv,
// or no line where go-console refuses. Planted: the step printed with its argv
// bare, as a "type by hand" line was.
//
// What it costs when this fails: a fix block a person pastes into cmd,
// PowerShell or sh, from an elevated prompt, runs a command a folder name chose.
func TestNoFixLineRunsPartOfAPathAPersonPastes(t *testing.T) {
	t.Parallel()
	const sid = "S-1-5-21-1-2-3-1001"
	separators := []string{"&calc", "|calc", ";calc", "$(calc)", "${IFS}calc"}
	winBad := []string{"%", "\u180e", "\t"}
	posixBad := []string{"\u180e", "\t", "\u202e"}
	refused, quoted := 0, 0
	for _, m := range separators {
		for _, b := range winBad {
			p := `C:\w\a` + b + m + `\x`
			if got := fixLine(gotrust.Fix{Path: p, Reset: true}); got != "" {
				t.Errorf("icacls fix of %q = %q, want no line", p, got)
			}
			if got := fixLine(gotrust.Fix{Path: p, Owner: sid}); got != "" {
				t.Errorf("icacls hand-over of %q = %q, want no line", p, got)
			}
			refused++
		}
		for _, b := range posixBad {
			p := "/x/a" + b + m
			if got := fixLine(gotrust.Fix{Path: p, Clear: 0o022}); got != "" {
				t.Errorf("chmod fix of %q = %q, want no line", p, got)
			}
			if got := fixLine(gotrust.Fix{Path: p, Owner: "uid:501"}); got != "" {
				t.Errorf("chown hand-over of %q = %q, want no line", p, got)
			}
			refused++
		}
		wp := `C:\w\a` + m + `\x`
		pp := "/x/a" + m
		for _, c := range []struct {
			what, got string
			d         shell.Dialect
			argv      []string
			prefix    string
		}{
			{"icacls fix", fixLine(gotrust.Fix{Path: wp, Reset: true}), shell.Windows, []string{"icacls", wp, "/reset", "/L"}, ""},
			{"icacls hand-over", fixLine(gotrust.Fix{Path: wp, Owner: sid}), shell.Windows, []string{"icacls", wp, "/setowner", "*" + sid, "/L"}, ""},
			{"chown hand-over", fixLine(gotrust.Fix{Path: pp, Owner: "uid:501"}), shell.POSIX, []string{"chown", "-h", "501", pp}, "sudo "},
		} {
			want, err := shell.Quote(c.d, c.argv)
			if err != nil {
				want = ""
			} else {
				want = c.prefix + want
			}
			if c.got != want {
				t.Errorf("%s of %q = %q, want go-console's own line %q", c.what, c.argv[1], c.got, want)
			}
			quoted++
		}
	}
	if refused != len(separators)*(len(winBad)+len(posixBad)) || quoted != len(separators)*3 {
		t.Fatalf("ran %d refused and %d quoted cases, want %d and %d", refused, quoted, len(separators)*(len(winBad)+len(posixBad)), len(separators)*3)
	}
}

// TestEachFixGoFilesGivesIsTheLineItWroteBefore: each fix go-files gives as
// data (Fix.Commands) reads as the lines a person types, each step quoted for
// its shell with go-console's shell.Quote, /L on every icacls step and the
// owner set by icacls; and a fix no run of commands makes, or none at all, is
// no line. Planted: an icacls step without /L, a revoke from a macOS list
// rendered as a line it is not.
//
// An icacls step is quoted for shell.Windows (go-console v0.16.0): one command
// cmd and PowerShell both read the same, so a path holding a `;`, `&` or `|` is
// in double quotes, not bare where a shell would split it. A path go-console
// quotes for neither shell, a per-cent cmd expands or a newline, has no such
// line, and the whole fix is no line, so the caller says what to do in words,
// as on release-1: a step printed with its path's separators bare would run
// part of the path when the block is pasted. Two steps are two lines, joined by
// a newline, never " && " (an error in Windows PowerShell 5.1).
// TestNoFixLineRunsPartOfAPathAPersonPastes holds the shell separators one by
// one.
func TestEachFixGoFilesGivesIsTheLineItWroteBefore(t *testing.T) {
	t.Parallel()
	const p = "/opt/it's"
	seen := 0
	for name, c := range map[string]struct {
		fix  gotrust.Fix
		want string
	}{
		"others write":                 {gotrust.Fix{Path: p, Clear: 0o022}, `chmod go-w '/opt/it'\''s'`},
		"group write, a mask":          {gotrust.Fix{Path: p, Clear: 0o020}, `chmod g-w '/opt/it'\''s'`},
		"owner cannot run":             {gotrust.Fix{Path: p, Set: 0o100}, `chmod u+x '/opt/it'\''s'`},
		"a key others reach":           {gotrust.Fix{Path: p, Clear: 0o077}, `chmod go-rwx '/opt/it'\''s'`},
		"a private file":               {gotrust.Fix{Path: p, Clear: 0o066}, `chmod go-rw '/opt/it'\''s'`},
		"a private read":               {gotrust.Fix{Path: p, Clear: 0o044}, `chmod go-r '/opt/it'\''s'`},
		"both ways":                    {gotrust.Fix{Path: p, Clear: 0o022, Set: 0o100}, `chmod go-w,u+x '/opt/it'\''s'`},
		"root's":                       {gotrust.Fix{Path: p, Owner: "uid:0"}, `sudo chown -h root '/opt/it'\''s'`},
		"Administrators'":              {gotrust.Fix{Path: `C:\tools\`, Owner: administrators}, `icacls C:\tools\ /setowner "*S-1-5-32-544" /L`},
		"a null list":                  {gotrust.Fix{Path: `C:\t`, Reset: true}, `icacls C:\t /reset /L`},
		"a grant to revoke":            {gotrust.Fix{Path: `C:\t`, Revoke: "S-1-1-0"}, "icacls C:\\t /inheritance:d /L\nicacls C:\\t /remove:g \"*S-1-1-0\" /L"},
		"a path with a space":          {gotrust.Fix{Path: `C:\a b`, Reset: true}, `icacls "C:\a b" /reset /L`},
		"a semicolon quoted":           {gotrust.Fix{Path: `C:\w\a;b\x`, Reset: true}, `icacls "C:\w\a;b\x" /reset /L`},
		"an ampersand quoted":          {gotrust.Fix{Path: `C:\w\a&b\x`, Reset: true}, `icacls "C:\w\a&b\x" /reset /L`},
		"a pipe quoted":                {gotrust.Fix{Path: `C:\w\a|b\x`, Reset: true}, `icacls "C:\w\a|b\x" /reset /L`},
		"a per-cent is no line":        {gotrust.Fix{Path: `C:\a%b`, Reset: true}, ""},
		"a newline is no line":         {gotrust.Fix{Path: "C:\\w\\a\nb\\x", Reset: true}, ""},
		"a hand-over to a uid":         {gotrust.Fix{Path: "/opt/it's.yaml", Owner: "uid:1000"}, `sudo chown -h 1000 '/opt/it'\''s.yaml'`},
		"a hand-over to a SID":         {gotrust.Fix{Path: `C:\t\s.yaml`, Owner: "S-1-5-21-1-2-3-1001"}, `icacls C:\t\s.yaml /setowner "*S-1-5-21-1-2-3-1001" /L`},
		"an account of neither":        {gotrust.Fix{Path: p, Owner: "nobody"}, ""},
		"a SID with an operator":       {gotrust.Fix{Path: `C:\t`, Owner: "S-1-5-32-544 & calc"}, ""},
		"an owner SID cut short":       {gotrust.Fix{Path: `C:\t`, Owner: "S-1-"}, ""},
		"a SID of the wrong authority": {gotrust.Fix{Path: `C:\t`, Owner: "S-2-5-32-544"}, ""},
		"another owner":                {gotrust.Fix{Path: p, Owner: "uid:501"}, `sudo chown -h 501 '/opt/it'\''s'`},
		"a malformed SID":              {gotrust.Fix{Path: `C:\t`, Revoke: "S-1-5-32-544 & calc"}, ""},
		"a SID cut short":              {gotrust.Fix{Path: `C:\t`, Revoke: "S-1-"}, ""},
		"a macOS entry":                {gotrust.Fix{Path: p, Revoke: "uid:501"}, ""},
		"an NFSv4 entry":               {gotrust.Fix{Path: p, Revoke: "nfs4:x@y"}, ""},
		"a special bit":                {gotrust.Fix{Path: p, Clear: 0o022 | 1<<23}, ""},
		"none":                         {gotrust.Fix{}, ""},
		"no path":                      {gotrust.Fix{Clear: 0o022}, ""},
	} {
		seen++
		if got := fixLine(c.fix); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	if seen == 0 {
		t.Fatal("the table is empty")
	}
}
