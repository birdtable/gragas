package coreapi

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/birdtable/gragas/internal/core/project"
)

// Exemptions is what check says of a project's exceptions: the findings,
// each as a person reads it, and the line of the listing the gate prints.
// Present is false when the project has no exemptions file, which is no
// finding.
type Exemptions struct {
	Present    bool
	File       string // the file read, from the project's root
	Findings   []string
	Rows       int
	NextExpiry string
}

// CheckExemptions reads the exemptions file of the repository l was loaded
// from, when there is one, and holds each row to the rulebook's active rules
// and to today, YYYY-MM-DD, asked only when the file is there (nil gives no
// date, which is a finding). A file that is there and cannot be read is a
// finding; a file that is not there is none.
func CheckExemptions(l Loaded, today func() string) Exemptions {
	if l.src == nil {
		return Exemptions{}
	}
	data, err := l.src.ReadFile(project.ExemptionsFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Exemptions{}
	case err != nil:
		return Exemptions{Present: true, File: project.ExemptionsFile, Findings: []string{fmt.Sprintf("%s: cannot be read: %v", project.ExemptionsFile, err)}}
	}
	active := map[string]bool{}
	if l.book != nil {
		for id, r := range l.book.Rules {
			if r.Data != nil && r.Data.Str("status") == "active" {
				active[id] = true
			}
		}
	}
	date := ""
	if today != nil {
		date = today()
	}
	got := project.CheckExemptions(data, active, date, l.src)
	out := Exemptions{Present: true, File: project.ExemptionsFile, Rows: got.Rows, NextExpiry: got.NextExpiry, Findings: make([]string, len(got.Findings))}
	for i, f := range got.Findings {
		out.Findings[i] = f.String()
	}
	return out
}
