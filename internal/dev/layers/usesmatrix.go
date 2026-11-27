package layers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// The published matrix of a repo is generated from its uses file, and a check compares the two as data. The
// uses file holds every fact the matrix holds, so nothing of it stands only in the matrix.

// textList is a field the matrix writes as one text or as a list of texts; it reads as a list.
type textList []string

func (t *textList) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*t = textList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("a text or a list of texts: %w", err)
	}
	*t = many
	return nil
}

type matrixPick struct {
	Item        string   `json:"item"`
	RegisterKey string   `json:"register_key"`
	Job         string   `json:"job"`
	Scope       string   `json:"scope"`
	Ban         string   `json:"ban"`
	BannedCalls []string `json:"banned_calls"`
	Pattern     string   `json:"pattern,omitempty"`
	Sources     textList `json:"sources"`
	AdoptedBy   []string `json:"adopted_by,omitempty"`
	Conditional string   `json:"conditional,omitempty"`
}

type matrixNotFor struct {
	Item    string   `json:"item"`
	Why     string   `json:"why"`
	Sources textList `json:"sources"`
}

type matrixFinding struct {
	Where      string   `json:"where"`
	What       string   `json:"what"`
	Lines      any      `json:"lines"`
	ReplacedBy []string `json:"replaced_by,omitempty"`
	RemovedBy  []string `json:"removed_by,omitempty"`
	Carried    bool     `json:"carried,omitempty"`
	Cut        bool     `json:"cut,omitempty"`
	Later      bool     `json:"later,omitempty"`
	KeptIssue  string   `json:"kept_issue,omitempty"`
	Sources    textList `json:"sources"`
	Note       string   `json:"note,omitempty"`
	Symbol     string   `json:"symbol,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
}

type matrixBlocker struct {
	Ticket string `json:"ticket"`
	Why    string `json:"why"`
}

type matrixDoc struct {
	Repo          string          `json:"repo"`
	PlanKeyPrefix string          `json:"plan_key_prefix,omitempty"`
	IssueKey      string          `json:"issue_key,omitempty"`
	IsLibrary     bool            `json:"is_library"`
	Related       []string        `json:"related,omitempty"`
	Picks         []matrixPick    `json:"picks"`
	NotFor        []matrixNotFor  `json:"not_for_this_repo"`
	Findings      []matrixFinding `json:"findings"`
	Blockers      []matrixBlocker `json:"blockers"`
	Notes         textList        `json:"notes"`
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// projection is the matrix the uses file generates.
func (u *Uses) projection() matrixDoc {
	d := matrixDoc{Repo: u.Repo, PlanKeyPrefix: u.PlanKeyPrefix, IssueKey: u.IssueKey, IsLibrary: u.IsLibrary, Related: u.Related,
		Picks: []matrixPick{}, NotFor: []matrixNotFor{}, Findings: []matrixFinding{}, Blockers: []matrixBlocker{}, Notes: textList(orEmpty(u.Notes))}
	for _, pk := range u.Picks {
		d.Picks = append(d.Picks, matrixPick{Item: pk.Item, RegisterKey: pk.RegisterKey, Job: pk.Job, Scope: pk.Scope, Ban: pk.Ban,
			BannedCalls: orEmpty(pk.BannedCalls), Pattern: pk.Pattern, Sources: orEmpty(pk.Sources), AdoptedBy: pk.AdoptedBy, Conditional: pk.Conditional})
	}
	for _, n := range u.NotFor {
		d.NotFor = append(d.NotFor, matrixNotFor{Item: n.Item, Why: n.Why, Sources: orEmpty(n.Sources)})
	}
	for _, f := range u.Findings {
		d.Findings = append(d.Findings, matrixFinding{Where: f.Where, What: f.What, Lines: f.Lines, ReplacedBy: f.ReplacedBy, RemovedBy: f.RemovedBy,
			Carried: f.Carried, Cut: f.Cut, Later: f.State == FindingLater, KeptIssue: f.KeptIssue, Sources: orEmpty(f.Sources), Note: f.Note,
			Symbol: f.Symbol, Pattern: f.Pattern})
	}
	for _, b := range u.Blockers {
		d.Blockers = append(d.Blockers, matrixBlocker(b))
	}
	return d
}

// Matrix is the published matrix JSON the uses file generates, one document per repo.
func (u *Uses) Matrix() ([]byte, error) {
	return json.MarshalIndent(u.projection(), "", " ")
}

// CompareMatrix compares a published matrix with the uses file as data: each pick, job that is not for the repo
// and finding by its key, and each field at the top. A text where the file has a list of one, or the reverse, is
// the same fact. A field the uses file has no place for is a difference: the file must hold every fact of the
// matrix. It answers each difference a line.
func (u *Uses) CompareMatrix(published []byte) []string {
	var got matrixDoc
	dec := json.NewDecoder(bytes.NewReader(published))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&got); err != nil {
		return []string{"the published matrix does not read as the uses file's form: " + err.Error()}
	}
	want := u.projection()
	var out []string
	if got.Repo != want.Repo || got.IsLibrary != want.IsLibrary {
		out = append(out, fmt.Sprintf("the matrix is for %s (library %v), the uses file for %s (library %v)", got.Repo, got.IsLibrary, want.Repo, want.IsLibrary))
	}
	for name, pair := range map[string][2]any{
		"plan_key_prefix": {got.PlanKeyPrefix, want.PlanKeyPrefix}, "issue_key": {got.IssueKey, want.IssueKey},
		"related": {orEmpty(got.Related), orEmpty(want.Related)}, "blockers": {got.Blockers, want.Blockers}, "notes": {orEmpty(got.Notes), orEmpty(want.Notes)},
	} {
		if !sameJSON(pair[0], pair[1]) {
			out = append(out, fmt.Sprintf("%s differs between the uses file and the matrix", name))
		}
	}
	out = append(out, diffRows("pick", pickKeys(want.Picks), pickKeys(got.Picks), want.Picks, got.Picks)...)
	out = append(out, diffRows("not_for_this_repo", notKeys(want.NotFor), notKeys(got.NotFor), want.NotFor, got.NotFor)...)
	out = append(out, diffRows("finding", findKeys(want.Findings), findKeys(got.Findings), want.Findings, got.Findings)...)
	slices.Sort(out)
	return out
}

func pickKeys(rows []matrixPick) []string {
	return mapKeys(rows, func(r matrixPick) string { return r.Item })
}
func notKeys(rows []matrixNotFor) []string {
	return mapKeys(rows, func(r matrixNotFor) string { return r.Item })
}
func findKeys(rows []matrixFinding) []string {
	return mapKeys(rows, func(r matrixFinding) string { return r.Where + "|" + r.Symbol + r.Pattern })
}

func mapKeys[T any](rows []T, key func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, key(r))
	}
	return out
}

// diffRows says each row the uses file has and the matrix lacks, each row the matrix has and the file lacks, and
// each row of the same key whose fields differ.
func diffRows[T any](kind string, wantKeys, gotKeys []string, want, got []T) []string {
	var out []string
	gotAt := map[string]T{}
	for i, k := range gotKeys {
		gotAt[k] = got[i]
	}
	wantAt := map[string]T{}
	for i, k := range wantKeys {
		wantAt[k] = want[i]
		g, ok := gotAt[k]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s %q is in the uses file and not in the matrix", kind, k))
		case !sameJSON(want[i], g):
			out = append(out, fmt.Sprintf("%s %q differs between the uses file and the matrix", kind, k))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(gotAt)) {
		if _, ok := wantAt[k]; !ok {
			out = append(out, fmt.Sprintf("%s %q is in the matrix and not in the uses file", kind, k))
		}
	}
	return out
}

// sameJSON is whether two values are the same as data: the same JSON.
func sameJSON(a, b any) bool {
	ja, ea := json.Marshal(a)
	jb, eb := json.Marshal(b)
	return ea == nil && eb == nil && string(ja) == string(jb)
}
