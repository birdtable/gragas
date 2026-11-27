package format

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"unicode"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// InitFiles are the files init writes, in the order they are written.
func InitFiles() []string { return []string{IdentityFile, StructureFile} }

// InitProblem is the first thing wrong with the form, "" when there is none.
// It is a sentence for a person that names the flag to change.
func InitProblem(f model.InitForm) string {
	switch {
	case !slugRE.MatchString(f.Name):
		return "the name " + fmt.Sprintf("%q", f.Name) + " is not a slug such as my-rules (--name)"
	case f.Kind != model.KindBase && f.Kind != model.KindOverlay && f.Kind != model.KindCompanion && f.Kind != model.KindFork:
		return "the kind " + fmt.Sprintf("%q", string(f.Kind)) + " is not base, overlay, companion or fork (--rulebook-kind)"
	case !semverRE.MatchString(f.Version):
		return "the starting version " + fmt.Sprintf("%q", f.Version) + " is not MAJOR.MINOR.PATCH (--version)"
	case f.Language == "" || !langRE.MatchString(f.Language):
		return "the language " + fmt.Sprintf("%q", f.Language) + " is not a language tag such as en (--language)"
	case f.Licences.Rules == "":
		return "the licence of the rules is empty (--licence)"
	case len(f.Owners) == 0:
		return "a rulebook has at least one owner (--owner)"
	case f.Kind == model.KindOverlay && (f.Extends == nil || f.Extends.Rulebook == "" || f.Extends.Version == ""):
		return "an overlay names its parent and the pin (--parent, --parent-version)"
	case f.Kind == model.KindOverlay && f.Extends.ContentHash == "":
		return "an overlay names the content hash of its parent's release (--parent-hash)"
	case f.Kind != model.KindOverlay && f.Extends != nil:
		return "only an overlay has a parent (--parent)"
	case f.Extends != nil && hasControl(f.Extends.Rulebook):
		return "the parent's name holds a control character (--parent)"
	case f.Extends != nil && !semverRE.MatchString(f.Extends.Version):
		return "the parent's version " + fmt.Sprintf("%q", f.Extends.Version) + " is not MAJOR.MINOR.PATCH (--parent-version)"
	case f.Extends != nil && !digestRE.MatchString(string(f.Extends.ContentHash)):
		return "the parent's content hash " + fmt.Sprintf("%q", string(f.Extends.ContentHash)) + " is not sha256: and 64 lowercase hex digits (--parent-hash)"
	case importProblem(f.Imports) != "":
		return importProblem(f.Imports)
	case !slugRE.MatchString(f.Layer.Slug) || !partRE.MatchString(f.Part.Slug):
		return "the first layer is a slug and the first part lowercase letters only (--layer, --part)"
	}
	return ""
}

// PlanRulebookInit is the init of a rulebook: the identity and the structure
// the form makes, with the rulebook as no rule yet. The files hold what a
// reader of the format reads back, and the same form gives the same bytes.
// It reads nothing and decides nothing of what is already in the folder.
func PlanRulebookInit(f model.InitForm) ([]model.Target, error) {
	if p := InitProblem(f); p != "" {
		return nil, errors.New(p)
	}
	return []model.Target{
		{Path: IdentityFile, Data: []byte(identityText(f))},
		{Path: StructureFile, Data: []byte(structureText(f))},
	}, nil
}

// InitEntry is the first history entry of a rulebook: the entity names the
// rulebook at its starting version (S68), and the entry holds the first
// owner.
func InitEntry(f model.InitForm) Entry {
	return Entry{Kind: "init", Entity: "rulebook:" + f.Name + "@" + f.Version, After: model.Lifecycle{Owner: f.Owners[0]}}
}

// InitRecord is the record function of the init: the history line of the
// first yes, read against the history file as it is.
func InitRecord(tree port.Tree, schema port.Schema, root model.Root, f model.InitForm) func(model.Yes) (model.Target, error) {
	return HistoryRecord(tree, schema, root, Loaded{}, InitEntry(f))
}

// InitInputs reads the three files init could meet, for the plan's input
// set: the digest of each, the empty digest for one that is not there.
func InitInputs(tree port.Tree, root model.Root) ([]model.Input, error) {
	var out []model.Input
	for _, p := range append(InitFiles(), HistoryFile) {
		b, err := tree.Read(root, p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			out = append(out, model.Input{Path: p})
		case err != nil:
			return nil, fmt.Errorf("%s could not be read: %w", p, err)
		default:
			out = append(out, model.Input{Path: p, Digest: digestOf(b)})
		}
	}
	return out, nil
}

// importProblem is the first thing wrong with the imports, "" when there is
// none: each names a rulebook, a version, at least one part and a content
// hash of the shape the digest has.
func importProblem(imports []model.Import) string {
	for _, i := range imports {
		switch {
		case i.Rulebook == "" || i.Version == "" || len(i.Parts) == 0:
			return "an import names its rulebook, its version and its parts (--import)"
		case hasControl(i.Rulebook):
			return "the name of the import of " + fmt.Sprintf("%q", i.Rulebook) + " holds a control character (--import)"
		case !semverRE.MatchString(i.Version):
			return "the version " + fmt.Sprintf("%q", i.Version) + " of the import of " + fmt.Sprintf("%q", i.Rulebook) + " is not MAJOR.MINOR.PATCH (--import)"
		case partProblem(i.Parts) != "":
			return "the parts of the import of " + fmt.Sprintf("%q", i.Rulebook) + " " + partProblem(i.Parts) + " (--import)"
		case i.ContentHash == "":
			return "the import of " + fmt.Sprintf("%q", i.Rulebook) + " names no content hash (--import)"
		case !digestRE.MatchString(string(i.ContentHash)):
			return "the content hash " + fmt.Sprintf("%q", string(i.ContentHash)) + " of the import of " + fmt.Sprintf("%q", i.Rulebook) + " is not sha256: and 64 lowercase hex digits (--import)"
		}
	}
	return ""
}

// partProblem is what is wrong with the parts of an import, "" when
// nothing: each is lowercase letters only, and none is named twice.
func partProblem(parts []string) string {
	seen := map[string]bool{}
	for _, p := range parts {
		switch {
		case !partRE.MatchString(p):
			return "hold " + fmt.Sprintf("%q", p) + ", which is not lowercase letters only"
		case seen[p]:
			return "name " + fmt.Sprintf("%q", p) + " twice"
		}
		seen[p] = true
	}
	return ""
}

// hasControl says s holds a control character, which a name is never given.
func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

func identityText(f model.InitForm) string {
	var b strings.Builder
	fmt.Fprintf(&b, "format: %d\nname: %s\nversion: %s\nkind: %s\nlicences:\n  rules: %s\n", model.FormatVersion, f.Name, f.Version, f.Kind, q(f.Licences.Rules))
	if f.Licences.CheckerCode != "" {
		fmt.Fprintf(&b, "  checker_code: %s\n", q(f.Licences.CheckerCode))
	}
	if f.Licences.Presets != "" {
		fmt.Fprintf(&b, "  presets: %s\n", q(f.Licences.Presets))
	}
	fmt.Fprintf(&b, "language: %s\n", f.Language)
	if f.DocsURL != "" {
		fmt.Fprintf(&b, "docs_url: %s\n", q(f.DocsURL))
	}
	if e := f.Extends; e != nil {
		fmt.Fprintf(&b, "extends:\n  rulebook: %s\n  version: %s\n", q(e.Rulebook), e.Version)
		if e.Source != "" {
			fmt.Fprintf(&b, "  source: %s\n", q(e.Source))
		}
		if e.ContentHash != "" {
			fmt.Fprintf(&b, "  content_hash: %s\n", q(string(e.ContentHash)))
		}
	}
	if len(f.Imports) > 0 {
		b.WriteString("imports:\n")
	}
	for _, i := range f.Imports {
		fmt.Fprintf(&b, "  - rulebook: %s\n    version: %s\n    parts:\n", q(i.Rulebook), i.Version)
		for _, p := range i.Parts {
			fmt.Fprintf(&b, "      - %s\n", q(p))
		}
		if i.Source != "" {
			fmt.Fprintf(&b, "    source: %s\n", q(i.Source))
		}
		fmt.Fprintf(&b, "    content_hash: %s\n", q(string(i.ContentHash)))
	}
	b.WriteString("owners:\n")
	for _, o := range f.Owners {
		fmt.Fprintf(&b, "  - %s\n", q(o))
	}
	g := f.Governance
	fmt.Fprintf(&b, "governance:\n  contribution: %s\n  decision_required: %t\n  trial_releases: %d\n  review_days: %d\n", g.Contribution, g.DecisionRequired, g.TrialReleases, g.ReviewDays)
	return b.String()
}

func structureText(f model.InitForm) string {
	return fmt.Sprintf("format: %d\nlayers:\n  - slug: %s\n    name: %s\n    parts:\n      - slug: %s\n        name: %s\n        concerns: []\nkinds: []\n",
		model.FormatVersion, f.Layer.Slug, q(f.Layer.Name), f.Part.Slug, q(f.Part.Name))
}

// q is s as a YAML double-quoted scalar, which a JSON string always is.
func q(s string) string { return fmt.Sprintf("%q", s) }

// The shapes the schemas hold a name, a version, a language and a part to.
var (
	slugRE   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	partRE   = regexp.MustCompile(`^[a-z]+$`)
	digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	semverRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	langRE   = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]+)*$`)
)
