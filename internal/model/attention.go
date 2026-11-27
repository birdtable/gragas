package model

// AttentionKind is one way a rule needs its maintainer, read from the book
// alone. The proof-mark kinds are added with the bindings, and the kinds that
// need a git history or a count of releases with the tickets that supply them.
type AttentionKind string

const (
	// AttentionOverdue is an active rule whose review-by date has passed.
	AttentionOverdue AttentionKind = "overdue review-by"
	// AttentionNoSource is a rule that cites no source.
	AttentionNoSource AttentionKind = "missing source"
	// AttentionNoPair is a rule that has no example pair where it needs one.
	AttentionNoPair AttentionKind = "missing example pair"
	// AttentionNoDecision is a rule whose reasoning names no decision where the
	// governance asks for one.
	AttentionNoDecision AttentionKind = "missing decision"
	// AttentionNoOwner is a rule with no owner.
	AttentionNoOwner AttentionKind = "owner missing"
	// AttentionComplete is a rule that misses nothing; only a book of proposed
	// rules lists it, so that the whole book is listed.
	AttentionComplete AttentionKind = "nothing missing"
)

// AttentionKinds are the kinds a listing is made of, in the order it groups
// them.
func AttentionKinds() []AttentionKind {
	return []AttentionKind{AttentionOverdue, AttentionNoSource, AttentionNoPair, AttentionNoDecision, AttentionNoOwner, AttentionComplete}
}

// AttentionItem is one rule in one kind of attention.
type AttentionItem struct {
	Rule   RuleID        `json:"rule"`
	Kind   AttentionKind `json:"kind"`
	Status Status        `json:"status"`
	// Why says what is wrong, in words; Fix is the command that mends it.
	Why string `json:"why"`
	Fix string `json:"fix"`
}

// AttentionListing is what needs the maintainer, as of one day.
type AttentionListing struct {
	// Today is the day the dates were judged against, YYYY-MM-DD.
	Today string `json:"today"`
	// AllProposed says every rule of the book is proposed, so the whole book is
	// listed.
	AllProposed bool            `json:"all_proposed"`
	Items       []AttentionItem `json:"items"`
	// Findings are the problems that stopped rules from being read.
	Findings []Finding `json:"findings"`
}
