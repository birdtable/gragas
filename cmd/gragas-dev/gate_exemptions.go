package main

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/host/clock"
)

// heldExemptions is what the gate holds of gate/exemptions.yaml: the line the
// tier summary prints of it, and each finding, escaped as every line the
// registry prints is.
type heldExemptions struct {
	line     string
	findings []string
}

// holdExemptions holds the project's exemptions file, read through fsys (the
// repository's root), to the rulebook's active rules and to today with
// project.CheckExemptions, the one comparison of an expiry with the day
// (gates.exemptions.2, gates.exemptions.4). It is gragas-dev's own call, with
// gragas-dev's own clock. The registry requires the file, so a file that cannot be
// read is a finding here as well; a rulebook whose rule files cannot be read
// is a finding here too, and every row is then unused.
func holdExemptions(fsys fs.FS, today string) heldExemptions {
	data, err := fs.ReadFile(fsys, project.ExemptionsFile)
	if err != nil {
		return heldExemptions{findings: []string{gate.Printable(fmt.Sprintf("%s: cannot be read, so no expiry is judged: %v", project.ExemptionsFile, err))}}
	}
	var active map[string]bool
	var unread []string
	if book, err := fs.Sub(fsys, "rulebook"); err == nil {
		if rules, err := gate.ReadRules(book); err == nil {
			active = gate.ActiveRules(rules)
		} else {
			first := err.Error()
			if u, ok := errors.AsType[*gate.Unread](err); ok {
				first = u.Lines[0]
			}
			unread = append(unread, gate.Printable("rulebook: the rule files are not read, so every exemption is unused: "+first))
		}
	}
	var stat fs.StatFS // nil names no exit file, so each is a finding
	if s, ok := fsys.(fs.StatFS); ok {
		stat = s
	}
	got := project.CheckExemptions(data, active, today, stat)
	next := got.NextExpiry
	if next == "" {
		next = "none"
	}
	out := heldExemptions{line: gate.Printable(fmt.Sprintf("exemptions: %d, next expiry %s", got.Rows, next))}
	out.findings = append(out.findings, unread...)
	for _, f := range got.Findings {
		out.findings = append(out.findings, gate.Printable(f.String()))
	}
	return out
}

// clockToday is the day gragas-dev judges an expiry on, YYYY-MM-DD, from the
// host's clock: the base's clock at landing.
func clockToday() string { return clock.Clock{}.Now().Format(time.DateOnly) }
