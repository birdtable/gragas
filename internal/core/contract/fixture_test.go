package contract

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/canon"
)

// fixtureDir is the fixture's hand-authored documents (contract/fixture),
// read here as files: the contract's goldens.
const fixtureDir = "../../../contract/fixture"

// fixtureDocs is each fixture document by file name and its kind.
func fixtureDocs() map[string]Kind {
	return map[string]Kind{
		"bundle.json":          Bundle,
		"changelog.json":       Changelog,
		"report-clean.json":    Report,
		"report-weakened.json": Report,
		"plan.json":            Plan,
		"sarif.json":           SARIF,
		"proposal.json":        Proposal,
		"grid.json":            Grid,
	}
}

func fixtureBytes(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(path.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEveryFixtureDocumentIsItsOwnCanonicalBytes: decoding a document into
// the model and encoding it again gives the bytes on disk (A20).
func TestEveryFixtureDocumentIsItsOwnCanonicalBytes(t *testing.T) {
	t.Parallel()
	for name := range fixtureDocs() {
		want := fixtureBytes(t, name)
		v, err := canon.Decode(want)
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		got, err := canon.Encode(v, canon.Document)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: canon.Encode = %v, not the bytes on disk\n%s", name, err, firstDiff(got, want))
		}
	}
}

// firstDiff is the line where a and b first differ, with its neighbours.
func firstDiff(a, b []byte) string {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := range min(len(la), len(lb)) {
		if !bytes.Equal(la[i], lb[i]) {
			return "line " + itoa(i+1) + ":\n got  " + string(la[i]) + "\n want " + string(lb[i])
		}
	}
	return "lengths " + itoa(len(la)) + " and " + itoa(len(lb))
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// schemaProps is every property name the schemas of v1 declare, found by
// walking each schema as a schema: through properties, items, the any/all/one
// lists, additionalProperties and $defs, never through a field that is itself
// named "properties".
func schemaProps(t testing.TB) map[string]string {
	t.Helper()
	out := map[string]string{}
	var walk func(file string, v any)
	walk = func(file string, v any) {
		node, ok := v.(map[string]any)
		if !ok {
			return
		}
		if props, ok := node["properties"].(map[string]any); ok {
			for k, c := range props {
				out[k] = file
				walk(file, c)
			}
		}
		for _, key := range []string{"items", "additionalProperties"} {
			walk(file, node[key])
		}
		for _, key := range []string{"anyOf", "allOf", "oneOf"} {
			if list, ok := node[key].([]any); ok {
				for _, c := range list {
					walk(file, c)
				}
			}
		}
		if defs, ok := node["$defs"].(map[string]any); ok {
			for _, c := range defs {
				walk(file, c)
			}
		}
	}
	for p, body := range schemaFiles(t) {
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		walk(path.Base(p), v)
	}
	return out
}

// keysOf adds every object key of v to into.
func keysOf(v canon.Value, into map[string]bool) {
	switch x := v.(type) {
	case canon.Object:
		for k, c := range x {
			into[k] = true
			keysOf(c, into)
		}
	case canon.Array:
		for _, c := range x {
			keysOf(c, into)
		}
	}
}

// notInTheFixture is the fields of the schemas that no fixture document
// carries, each with why; the README says the same. A field leaves this list
// when a document carries it.
func notInTheFixture() map[string]string {
	return map[string]string{
		"since":      "no shape of v1.0 is deprecated, so no document carries a deprecation",
		"removed_in": "no shape of v1.0 is deprecated, so no document carries a deprecation",
		"preset":     "the flattened upstream set is a rulebook data file read by coverage, not a contract document; upstream_test.go holds its example",
		"sets":       "the flattened upstream set is a rulebook data file read by coverage, not a contract document; upstream_test.go holds its example",
		"observed":   "the flattened upstream set is a rulebook data file read by coverage, not a contract document; upstream_test.go holds its example",
	}
}

// TestEveryFieldOfTheSchemasIsInAFixtureDocument: the fixture is the one
// example of every field, so a field no document carries is named here with
// its reason, and a field named here that a document carries is stale.
func TestEveryFieldOfTheSchemasIsInAFixtureDocument(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for name := range fixtureDocs() {
		v, err := canon.Decode(fixtureBytes(t, name))
		if err != nil {
			t.Fatal(err)
		}
		keysOf(v, seen)
	}
	skip := notInTheFixture()
	for field, file := range schemaProps(t) {
		why, listed := skip[field]
		switch {
		case !seen[field] && !listed:
			t.Errorf("%s declares %q and no fixture document carries it", file, field)
		case seen[field] && listed:
			t.Errorf("%q is listed as absent (%s) and a fixture document carries it", field, why)
		}
	}
}

// bundleMap is the fixture bundle read as plain JSON, for the derivations.
func bundleMap(t testing.TB) map[string]any {
	t.Helper()
	var b map[string]any
	if err := json.Unmarshal(fixtureBytes(t, "bundle.json"), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func list(v any) []map[string]any {
	var out []map[string]any
	for _, x := range v.([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func strs(v any) []string {
	var out []string
	if v == nil {
		return nil
	}
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

// ratio is num/den to 4 places, rounded half to even, as S29 and the README
// write it; den is never 0 in the fixture, which holds an item of every set.
func ratio(t testing.TB, num, den int) float64 {
	t.Helper()
	if den == 0 {
		t.Fatal("an empty denominator: the fixture must hold an item of every set a ratio divides by")
	}
	r := new(big.Rat).SetFrac64(int64(num), int64(den))
	scaled := new(big.Rat).Mul(r, big.NewRat(10000, 1))
	q, rem := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	twice := new(big.Int).Mul(rem, big.NewInt(2))
	switch c := twice.Cmp(scaled.Denom()); {
	case c > 0, c == 0 && q.Bit(0) == 1:
		q.Add(q, big.NewInt(1))
	}
	f, _ := new(big.Rat).SetFrac(q, big.NewInt(10000)).Float64()
	return f
}

// liveRules splits the rules into the active and the live (active or trial)
// ones: the sets S29's ratios divide by.
func liveRules(rules []map[string]any) (active, live []map[string]any) {
	for _, r := range rules {
		switch r["status"] {
		case "active":
			active = append(active, r)
			live = append(live, r)
		case "trial":
			live = append(live, r)
		}
	}
	return active, live
}

// rulesHeld counts the live rules with an entry in the coverage.
func rulesHeld(b map[string]any, live []map[string]any) int {
	bound := map[string]bool{}
	for _, id := range strs(b["coverage"].(map[string]any)["rules_to_bindings"]) {
		bound[id] = true
	}
	n := 0
	for _, r := range live {
		if bound[r["id"].(string)] {
			n++
		}
	}
	return n
}

// entriesExplained counts the coverage's entries that map to a rule, and the
// entries.
func entriesExplained(b map[string]any) (explained, entries int) {
	all := strs(b["coverage"].(map[string]any)["bindings_to_rules"])
	for _, e := range all {
		for _, bd := range list(b["bindings"]) {
			if bd["tool"].(string)+"/"+bd["key"].(string) == e && bd["rule"] != nil {
				explained++
				break
			}
		}
	}
	return explained, len(all)
}

// proofRate counts the passes and the proofs that ran (pass and fail).
func proofRate(b map[string]any) (pass, ran int) {
	for _, p := range list(b["proof_grid"]) {
		switch p["result"] {
		case "pass":
			pass++
			ran++
		case "fail":
			ran++
		}
	}
	return pass, ran
}

// completeRules counts the active rules with a source, a pair and a decision
// (a history entry that names one).
func completeRules(b map[string]any, active []map[string]any) int {
	cited := map[string]bool{}
	for _, h := range list(b["history"]) {
		if d, ok := h["decision"].(string); ok && d != "" {
			cited[strings.TrimPrefix(h["entity"].(string), "rule:")] = true
		}
	}
	n := 0
	for _, r := range active {
		if len(strs(r["sources"])) > 0 && r["pairs"] != nil && len(r["pairs"].([]any)) > 0 && cited[r["id"].(string)] {
			n++
		}
	}
	return n
}

// reviewCurrent counts the live rules with a review_by on or after the
// release date, and those with one.
func reviewCurrent(date string, live []map[string]any) (current, dated int) {
	for _, r := range live {
		if by, ok := r["review_by"].(string); ok {
			dated++
			if by >= date {
				current++
			}
		}
	}
	return current, dated
}

// attentionKinds is, per rule, the kinds of S13 that apply to it among the
// seven health counts (a high override rate needs two report batches and S29
// keeps signals out of health): an unproven binding; a missing example pair,
// source or decision, of a rule that is active or trial; an overdue review-by;
// planned past its zero-by; a blocked binding. The two kinds the fixture holds
// no data for, a trial past its window and a rule a linter holds that still
// sits in review text, apply to no rule here.
func attentionKinds(b map[string]any, date string) map[string][]string {
	cited := map[string]bool{}
	for _, h := range list(b["history"]) {
		if d, ok := h["decision"].(string); ok && d != "" {
			cited[strings.TrimPrefix(h["entity"].(string), "rule:")] = true
		}
	}
	out := map[string][]string{}
	for _, r := range list(b["rules"]) {
		id := r["id"].(string)
		marks := strs(r["marks"])
		var kinds []string
		if slices.Contains(marks, "unproven") {
			kinds = append(kinds, "unproven binding")
		}
		if r["status"] == "active" || r["status"] == "trial" {
			pairs, _ := r["pairs"].([]any)
			if len(strs(r["sources"])) == 0 || len(pairs) == 0 || !cited[id] {
				kinds = append(kinds, "missing pair, source or decision")
			}
		}
		if by, ok := r["review_by"].(string); ok && by < date {
			kinds = append(kinds, "overdue review-by")
		}
		if zb, ok := r["zero_by"].(string); ok && zb < date && r["standing"] == "planned" {
			kinds = append(kinds, "planned past zero-by")
		}
		if slices.Contains(marks, "blocked") {
			kinds = append(kinds, "blocked binding")
		}
		if len(kinds) > 0 {
			out[id] = kinds
		}
	}
	return out
}

// daysToActiveMedian is the median, the lower middle of an even count, of the
// whole days from a rule's proposed to its active in the history.
func daysToActiveMedian(t testing.TB, b map[string]any) int {
	t.Helper()
	proposed := map[string]time.Time{}
	var days []int
	for _, h := range list(b["history"]) {
		when, err := time.Parse(time.RFC3339, h["when"].(string))
		if err != nil {
			t.Fatal(err)
		}
		e := h["entity"].(string)
		after, _ := h["after"].(map[string]any)
		switch {
		case h["kind"] == "init" && after["status"] == "proposed":
			proposed[e] = when
		case after["status"] == "active" && !proposed[e].IsZero():
			from := proposed[e].Truncate(24 * time.Hour)
			days = append(days, int(when.Truncate(24*time.Hour).Sub(from).Hours()/24))
			delete(proposed, e)
		}
	}
	if len(days) == 0 {
		t.Fatal("no rule went from proposed to active in the history")
	}
	slices.Sort(days)
	return days[(len(days)-1)/2]
}

// TestTheHealthNumbersAreWhatTheFixtureDataGives: each of the seven numbers
// of the 1.2.0 release is recomputed from the bundle's own rules, coverage,
// proof grid, history and bindings, by the derivations the README gives line
// by line, and equals the number the release carries.
func TestTheHealthNumbersAreWhatTheFixtureDataGives(t *testing.T) {
	t.Parallel()
	b := bundleMap(t)
	rules := list(b["rules"])
	release := b["release"].(map[string]any)
	active, live := liveRules(rules)
	explained, entries := entriesExplained(b)
	pass, ran := proofRate(b)
	current, dated := reviewCurrent(release["date"].(string), live)

	want := map[string]float64{
		"rules-held":            ratio(t, rulesHeld(b, live), len(live)),
		"entries-explained":     ratio(t, explained, entries),
		"proof-pass-rate":       ratio(t, pass, ran),
		"completeness":          ratio(t, completeRules(b, active), len(active)),
		"review-currency":       ratio(t, current, dated),
		"attention-size":        float64(len(attentionKinds(b, release["date"].(string)))),
		"days-to-active-median": float64(daysToActiveMedian(t, b)),
	}
	nums := release["health"].(map[string]any)["numbers"].([]any)
	order := []string{"rules-held", "entries-explained", "proof-pass-rate", "completeness", "review-currency", "attention-size", "days-to-active-median"}
	if len(nums) != len(order) {
		t.Fatalf("the release carries %d health numbers, want %d", len(nums), len(order))
	}
	for i, n := range nums {
		m := n.(map[string]any)
		name := m["name"].(string)
		if name != order[i] {
			t.Errorf("health number %d is %q, want %q (S29's order)", i, name, order[i])
		}
		if got := m["value"].(float64); got != want[name] {
			t.Errorf("%s = %v in the release, %v from the fixture data", name, got, want[name])
		}
	}
}

// TestTheBumpOfTheReleaseIsTheLargestChangeItHolds: S11's table over the
// entries of the 1.2.0 release: a rule added, tightened or replaced with a
// successor is a minor, a term or an explanation a patch, and a rule removed,
// retired without a successor or weakened a major.
func TestTheBumpOfTheReleaseIsTheLargestChangeItHolds(t *testing.T) {
	t.Parallel()
	b := bundleMap(t)
	release := b["release"].(map[string]any)
	rank := map[string]int{"patch": 1, "minor": 2, "major": 3}
	got := 0
	for _, e := range list(b["changelog"]) {
		if e["release"] != release["version"] {
			continue
		}
		var bump string
		switch kind, _, _ := strings.Cut(e["entity"].(string), ":"); {
		case e["change"] == "added" && kind == "rule":
			bump = "minor"
		case e["change"] == "replaced" && kind == "rule":
			bump = "minor"
		case e["change"] == "removed" || e["change"] == "retired":
			bump = "major"
		case kind == "term":
			bump = "patch"
		default:
			t.Fatalf("no row of S11's table for %v", e)
		}
		got = max(got, rank[bump])
	}
	if want := rank[release["bump"].(string)]; got != want {
		t.Errorf("the entries give rank %d, the release says %s", got, release["bump"])
	}
}
