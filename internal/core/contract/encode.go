package contract

import (
	"fmt"

	"github.com/birdtable/gragas/internal/canon"
	mc "github.com/birdtable/gragas/internal/model/contract"
)

// Encode is the one encoder of every document (A20, cli.output.1): v, the
// model value of a document of kind k, in canon's Document layout, so the
// same value is always the same bytes. The contract field names the contract
// this engine writes: Encode fills it when it is empty and refuses a value
// that names another. v is the document's own generated type, or a pointer
// to one; any other value is refused.
func Encode(k Kind, v any) ([]byte, error) {
	if !k.known() {
		return nil, fmt.Errorf("%q is not a document of the contract", clip(string(k)))
	}
	tree, got := treeOf(v)
	if got != k {
		return nil, fmt.Errorf("encode a %s: the value is %s", k, describe(got))
	}
	obj, isObject := tree.(canon.Object)
	if !isObject {
		return nil, fmt.Errorf("encode the %s: the value is not a JSON object", k) // a nil pointer
	}
	cur := Current().String()
	named, isString := obj["contract"].(canon.String)
	switch {
	case !isString || named == "":
		obj["contract"] = canon.String(cur)
	case named == canon.String(cur):
	default:
		return nil, fmt.Errorf("encode the %s: it names contract %q, and this engine writes contract %s", k, clip(string(named)), cur)
	}
	return canon.Encode(obj, canon.Document)
}

// treeOf is v as canon's value tree, by its generated ToCanon, and the
// document v is the generated type of; "" for any other value. A nil pointer
// to a document's type is no tree.
func treeOf(v any) (canon.Value, Kind) {
	switch x := v.(type) {
	case mc.BundleDocument:
		return x.ToCanon(), Bundle
	case *mc.BundleDocument:
		return deref(x), Bundle
	case mc.ReportDocument:
		return x.ToCanon(), Report
	case *mc.ReportDocument:
		return deref(x), Report
	case mc.ChangelogDocument:
		return x.ToCanon(), Changelog
	case *mc.ChangelogDocument:
		return deref(x), Changelog
	case mc.SarifDocument:
		return x.ToCanon(), SARIF
	case *mc.SarifDocument:
		return deref(x), SARIF
	case mc.PlanDocument:
		return x.ToCanon(), Plan
	case *mc.PlanDocument:
		return deref(x), Plan
	case mc.ProposalDocument:
		return x.ToCanon(), Proposal
	case *mc.ProposalDocument:
		return deref(x), Proposal
	case mc.GridDocument:
		return x.ToCanon(), Grid
	case *mc.GridDocument:
		return deref(x), Grid
	}
	return nil, ""
}

// deref is ToCanon of *p, and no tree for a nil p.
func deref[T interface{ ToCanon() canon.Value }](p *T) canon.Value {
	if p == nil {
		return nil
	}
	return (*p).ToCanon()
}

func describe(k Kind) string {
	if k == "" {
		return "not a document of the contract"
	}
	return "a " + string(k)
}

// EncodeProposal is Encode of a proposal document (S65): the bytes propose
// prints and an overlay's proposals file carries. Its evidence holds only a
// kind and a text, so a project's evidence has no field that names a
// project (spec 16.1).
func EncodeProposal(d mc.ProposalDocument) ([]byte, error) { return Encode(Proposal, d) }
