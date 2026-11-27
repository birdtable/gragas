package config

import (
	"errors"
	"fmt"
)

// Kind is how a run ends on a settings problem.
type Kind int

// The three endings a settings problem has.
const (
	// Refused: the file says something gragas will not run with. It is
	// fixed in the file.
	Refused Kind = iota
	// NotReady: something is missing before gragas can start, and Next
	// says what to do.
	NotReady
	// Failed: the file is there and could not be read.
	Failed
)

// Problem is what is wrong with the settings, and how the run ends on it.
// What is the sentence, with no full stop; it may hold a path a person
// chose, which the command tree prints through ui.Name.
type Problem struct {
	Kind Kind
	What string
	// Next is the line that makes it ready, for NotReady.
	Next string
}

func (p *Problem) Error() string { return p.What }

func refused(format string, args ...any) *Problem {
	return &Problem{Kind: Refused, What: fmt.Sprintf(format, args...)}
}

func notReady(next, format string, args ...any) *Problem {
	return &Problem{Kind: NotReady, What: fmt.Sprintf(format, args...), Next: next}
}

// at is a refusal at a line of the settings file: file:line: what.
func at(file string, line int, format string, args ...any) *Problem {
	where := file
	if line > 0 {
		where = fmt.Sprintf("%s:%d", file, line)
	}
	return refused("%s: %s", where, fmt.Sprintf(format, args...))
}

// refusal is what the reader returns for a file it will not read (host/fs's
// Unsafe): another account owns it, its group or others can write it, or it
// is not a regular file (cli.config.12). why ends "the settings file
// <path> ...", and fix is the line that fixes it, "" when there is none.
type refusal interface {
	error
	Refusal() (why, fix string)
}

// readProblem is how a run ends when read failed on the file what (such as
// "the settings file") at path. An error that matches errors.ErrUnsupported
// says this system gives no owner or mode to check (Windows), so the file is
// not read and the run is not ready: the check fails closed.
func readProblem(what, path string, err error) *Problem {
	var u refusal
	switch {
	case errors.As(err, &u):
		why, fix := u.Refusal()
		p := refused("%s %s %s, and it names the programs gragas runs, so it is not read", what, path, why)
		if fix != "" {
			p.What += ". Fix it with `" + fix + "`"
		}
		return p
	case errors.Is(err, errors.ErrUnsupported):
		return notReady("move "+path+" away to run with the defaults",
			"%s %s exists, and its owner and mode cannot be checked on this system, so it is not read", what, path)
	}
	return &Problem{Kind: Failed, What: fmt.Sprintf("%s %s could not be read: %v", what, path, err)}
}
