package audit

import "fmt"

// Module is one module of the repository's build list, as `go list -m -json
// all` says it. Replace is the path or module a replace directive puts in its
// place, and Local says that replacement is a folder.
type Module struct {
	Path    string
	Replace string
	Local   bool
}

// ReplaceInput is what the replace check reads. Workspace is the go.work file
// the build uses, or "".
type ReplaceInput struct {
	Modules   []Module
	Workspace string
	Rules     RepoRules
}

// CheckReplace is the replace check (supply.age.3): a replace to a local path
// and a go workspace are findings by name, since each lets in code whose age
// nothing says. A replace by another module is not one.
func CheckReplace(in ReplaceInput) []Finding {
	var out []Finding
	if in.Workspace != "" {
		out = append(out, found(in.Rules, KeyReplace, in.Workspace, 0,
			"a go workspace builds its modules from their folders, so the age of what the build uses is not judged"))
	}
	for _, m := range in.Modules {
		if m.Local {
			out = append(out, found(in.Rules, KeyReplace, "go.mod", 0,
				fmt.Sprintf("module %s is replaced by the folder %s, so its age is not judged", m.Path, m.Replace)))
		}
	}
	return sorted(out)
}
