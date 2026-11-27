package apply

import (
	"encoding/hex"
	"fmt"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
)

// DigestBytes is the digest of a file's own bytes: "sha256:" and the SHA-256
// sum in lower-case hex. It is the digest an Input and a Target.Before hold.
// It is the gate's kind of fingerprint, not canon's: canon digests a value's
// canonical encoding and nothing else (A59), so a file's bytes are summed
// here, in the one function that does it.
func DigestBytes(b []byte) model.Digest {
	return model.Digest(digest.Prefixed(b))
}

// Digest is the plan digest: the canonical digest (canon) of the command, the
// delta, every file with the digest of its bytes, the sections, the input set,
// the goal and the yes without its plan digest. The record's bytes are left
// out, since they hold the digest itself; its path, its mode and what it
// replaces are in. The plan's own Digest field and the yes's Plan field are
// not read.
func Digest(p model.Plan) (model.Digest, error) {
	d, err := canon.DigestOf(planValue(p, false))
	if err != nil {
		return "", fmt.Errorf("the plan has no canonical form: %w", err)
	}
	return model.Digest(d.String()), nil
}

// planValue is the plan as a canon value. With data it holds every file's
// bytes, hex-encoded: that is the intent a journal keeps, so recovery needs
// nothing else (architecture 5, step 4). Without, it holds each file's digest
// instead: that is what the plan digest covers, the same bytes in a form a
// person can compare.
func planValue(p model.Plan, data bool) canon.Value {
	o := canon.Object{
		"command":  canon.String(p.Command),
		"delta":    deltaValue(p.Delta),
		"files":    targetsValue(p.Files, data),
		"sections": sectionsValue(p.Sections),
		"inputs":   inputsValue(p.Inputs),
		"goal":     inputsValue(p.Goal.Files),
		"yes":      canon.Null{},
		"record":   canon.Null{},
	}
	if p.Yes != nil {
		y := canon.Object{
			"who":     canon.String(p.Yes.Who),
			"command": canon.String(p.Yes.Command),
		}
		if data {
			// the time of the answer is kept with the intent a journal holds and
			// never in the plan digest: that moves with the clock, not the plan
			y["at"] = canon.Int(p.Yes.At)
		}
		o["yes"] = y
	}
	if p.Record != nil {
		r := *p.Record
		if !data {
			r.Data = nil
		}
		o["record"] = targetValue(r, data)
	}
	return o
}

func deltaValue(d model.Delta) canon.Value {
	out := canon.Array{}
	for _, c := range d.Changes {
		out = append(out, canon.Object{
			"entity": canon.String(c.Entity), "field": canon.String(c.Field),
			"before": canon.String(c.Before), "after": canon.String(c.After),
		})
	}
	return out
}

func targetsValue(ts []model.Target, data bool) canon.Value {
	out := canon.Array{}
	for _, t := range ts {
		out = append(out, targetValue(t, data))
	}
	return out
}

func targetValue(t model.Target, data bool) canon.Value {
	o := canon.Object{
		"path":   canon.String(t.Path),
		"before": canon.String(string(t.Before)),
		"append": canon.Bool(t.Append),
	}
	if t.Delete {
		o["delete"] = canon.Bool(true) // absent for a write, so a plan digest of a write holds
	}
	if data {
		o["data"] = canon.String(hex.EncodeToString(t.Data))
	} else {
		o["data_digest"] = canon.String(string(DigestBytes(t.Data)))
	}
	return o
}

func sectionsValue(ss []model.Section) canon.Value {
	out := canon.Array{}
	for _, s := range ss {
		lines := canon.Array{}
		for _, l := range s.Lines {
			lines = append(lines, canon.String(l))
		}
		out = append(out, canon.Object{"title": canon.String(s.Title), "lines": lines})
	}
	return out
}

func inputsValue(in []model.Input) canon.Value {
	out := canon.Array{}
	for _, i := range in {
		out = append(out, canon.Object{"path": canon.String(i.Path), "digest": canon.String(string(i.Digest))})
	}
	return out
}
