package contract

import (
	"fmt"

	"github.com/birdtable/gragas/internal/canon"
	mc "github.com/birdtable/gragas/internal/model/contract"
)

// ReadBundle reads data, a bundle from outside (supply.scans.5), into its
// model type. It reads no file and runs nothing: bytes in, model out. It
// reads in this order, and the first step that refuses ends it:
//
//  1. the size cap, MaxDocument;
//  2. canon's decode, which refuses a repeated key, bad UTF-8, a number two
//     readers could read apart, and nesting past canon.MaxDepth;
//  3. the contract version the bundle names, the schema of that version and
//     the deprecation windows;
//  4. the decode into the model type, by its generated FromCanon, which
//     refuses a member the type has no field for under its exact name, and a
//     member of another JSON type than its field;
//  5. the history's two copies of a status: a step that carries a status
//     and whose after.status is not the same string (one fact, contract 1.3) is refused,
//     naming the step's number and no other data.
//
// A bundle in a newer major is *NotReady, and so is one in a newer minor of
// this major: the model type holds the fields of the contract this engine
// writes, and a field a later minor adds would be refused as unknown rather
// than named. Anything else wrong is *Refused, with the schema's findings
// when it was the schema. An error is a port that failed.
func ReadBundle(data []byte, s Schemas) (mc.BundleDocument, error) {
	var out mc.BundleDocument
	obj, ver, findings, err := read(Bundle, data, s)
	cur := Current()
	switch {
	case err != nil:
		return out, err
	case ver.Major == cur.Major && ver.Minor > cur.Minor:
		return out, &NotReady{Kind: Bundle, Found: ver, Reads: cur}
	case len(findings) > 0:
		return out, &Refused{Kind: Bundle, Why: "it fails its contract", Findings: findings}
	}
	if err := strict(Bundle, obj, &out); err != nil {
		return mc.BundleDocument{}, err
	}
	if n, bad := splitStatus(out.History); bad {
		return mc.BundleDocument{}, &Refused{Kind: Bundle, Why: fmt.Sprintf("history step %d records two statuses", n)}
	}
	return out, nil
}

// splitStatus is the number of the first history step that carries a status
// and whose after.status is not the same string: one that differs, one that is
// no string (a number, a list), and none at all. after is an untyped record,
// so the schema cannot hold the two equal. A step with no status,
// or a null one, is not compared.
func splitStatus(steps []mc.HistoryEntry) (int, bool) {
	for _, h := range steps {
		if h.Status == nil {
			continue
		}
		moved, ok := h.After["status"].(canon.String)
		if !ok || string(*h.Status) != string(moved) {
			return h.Number, true
		}
	}
	return 0, false
}

// strict reads obj, a document of kind k, into into by its generated
// FromCanon, which matches a key to a field by its exact name and refuses a
// key no field has.
func strict(k Kind, obj canon.Object, into interface{ FromCanon(canon.Value) error }) error {
	if err := into.FromCanon(obj); err != nil {
		return &Refused{Kind: k, Why: "its fields are not the model's: " + clip(err.Error()), Err: err}
	}
	return nil
}
