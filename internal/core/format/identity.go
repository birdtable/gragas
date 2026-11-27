package format

import (
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// identityOf reads a document the identity schema accepted. Nothing here
// checks: the schema has, so a value of another type reads as its zero value.
func identityOf(m *ordered.Map) model.RulebookIdentity {
	id := model.RulebookIdentity{
		Format:      model.FormatVersion,
		Name:        m.Str("name"),
		Version:     m.Str("version"),
		Kind:        model.IdentityKind(m.Str("kind")),
		Language:    m.Str("language"),
		DocsURL:     m.Str("docs_url"),
		ExplorerURL: m.Str("explorer_url"),
		Owners:      ordered.Strings(m.List("owners")),

		HandWrittenPresets: ordered.Strings(m.List("hand_written_presets")),
	}
	if l := m.Sub("licences"); l != nil {
		id.Licences = model.Licences{Rules: l.Str("rules"), CheckerCode: l.Str("checker_code"), Presets: l.Str("presets")}
	}
	if e := m.Sub("extends"); e != nil {
		id.Extends = &model.Extends{Rulebook: e.Str("rulebook"), Version: e.Str("version"), Source: e.Str("source"), ContentHash: model.Digest(e.Str("content_hash"))}
	}
	for _, i := range ordered.Maps(m.List("imports")) {
		id.Imports = append(id.Imports, model.Import{
			Rulebook: i.Str("rulebook"), Version: i.Str("version"), Source: i.Str("source"),
			Parts: ordered.Strings(i.List("parts")), ContentHash: model.Digest(i.Str("content_hash")),
		})
	}
	if g := m.Sub("governance"); g != nil {
		id.Governance = model.Governance{
			Contribution:          model.Contribution(g.Str("contribution")),
			CheckerPublishers:     ordered.Strings(g.List("checker_publishers")),
			CheckerSecondReviewer: g.Val("checker_second_reviewer") == true,
			CheckerProof:          model.ProofLevel(g.Str("checker_proof")),
			DecisionRequired:      g.Val("decision_required") == true,
			TrialReleases:         intOf(g.Val("trial_releases")),
			ReviewDays:            intOf(g.Val("review_days")),
		}
	}
	if t := m.Sub("tested"); t != nil {
		id.Tested = &model.Tested{
			Tools: versionsOf(t.List("tools")), Plugins: versionsOf(t.List("plugins")),
			Kits: versionsOf(t.List("kits")), Adapters: versionsOf(t.List("adapters")),
			MinEngine: t.Str("min_engine"),
		}
	}
	if b := m.Sub("budgets"); b != nil {
		id.Budgets = &model.Budgets{StatementWords: intOf(b.Val("statement_words")), ReasoningWords: intOf(b.Val("reasoning_words"))}
	}
	return id
}

func versionsOf(l []any) []model.Version {
	var out []model.Version
	for _, v := range ordered.Maps(l) {
		out = append(out, model.Version{Name: v.Str("name"), Version: v.Str("version")})
	}
	return out
}

func intOf(v any) int {
	if n, ok := v.(int64); ok {
		return int(n)
	}
	return 0
}
