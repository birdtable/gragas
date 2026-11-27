package porttest

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// rawOp is how a raw call OnRaw answers is recorded among the calls: it is
// read by no operation.
const rawOp plugin.Op = "raw"

// Raw is a trial's raw call (port.Trial.Raw), answered as a module of the
// wire answers: the bytes OnRaw scripts, where it is set; else the document
// the operation's own scripted answer is, in canonical form and ended by a
// newline, as the wasm stand-in writes it. Before any operation is reached,
// a request that is not one JSON document of the request schema is answered
// bug, and an operation of no name unsupported. A plugin's own error
// (plugin.CallError) is answered as the wire's error document; any other
// error of the scripted answer, a limit or a crash, is Raw's error.
func (o *opened) Raw(ctx context.Context, request []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.p.OnRaw != nil {
		if err := o.call(rawOp); err != nil {
			return nil, err
		}
		return o.p.OnRaw(slices.Clone(request))
	}
	v, err := canon.Decode(request)
	if err != nil {
		return failed(plugin.CodeBug, "the request is not one JSON document")
	}
	obj, ok := v.(canon.Object)
	if !ok {
		return failed(plugin.CodeBug, "the request is not an object")
	}
	if s, ok := obj["op"].(canon.String); ok && !plugin.Op(s).Valid() {
		return failed(plugin.CodeUnsupported, "no such operation")
	}
	var r plugin.Request
	if err := r.FromCanon(v); err != nil {
		return failed(plugin.CodeBug, err.Error())
	}
	answer, err := o.typed(ctx, r)
	var ce *plugin.CallError
	switch {
	case errors.As(err, &ce):
		e := plugin.Error{Code: ce.Code, Message: ce.Message}
		if ce.Path != "" {
			e.Path = &ce.Path
		}
		return document(plugin.Response{Error: &e})
	case err != nil:
		return nil, err
	}
	body, ok := answer.(canon.Object)
	if !ok {
		return nil, fmt.Errorf("the scripted answer of %s is not an object", r.Op)
	}
	return document(plugin.Response{Ok: body})
}

// typed is the scripted answer of r's operation to its body, as canon's
// tree; a body the operation's schema refuses is answered bug.
func (o *opened) typed(ctx context.Context, r plugin.Request) (canon.Value, error) {
	bad := func(err error) error {
		return &plugin.CallError{Code: plugin.CodeBug, Message: "the body of " + string(r.Op) + " " + err.Error()}
	}
	switch r.Op {
	case plugin.OpPlan:
		var in plugin.PlanRequest
		if err := in.FromCanon(r.Body); err != nil {
			return nil, bad(err)
		}
		a, err := o.Plan(ctx, in)
		return a.ToCanon(), err
	case plugin.OpEntries:
		var in plugin.EntriesRequest
		if err := in.FromCanon(r.Body); err != nil {
			return nil, bad(err)
		}
		a, err := o.Entries(ctx, in)
		return a.ToCanon(), err
	case plugin.OpFindings:
		var in plugin.FindingsRequest
		if err := in.FromCanon(r.Body); err != nil {
			return nil, bad(err)
		}
		a, err := o.Findings(ctx, in)
		return a.ToCanon(), err
	case plugin.OpRank:
		var in plugin.RankRequest
		if err := in.FromCanon(r.Body); err != nil {
			return nil, bad(err)
		}
		a, err := o.Rank(ctx, in)
		return a.ToCanon(), err
	case plugin.OpPreset:
		var in plugin.PresetRequest
		if err := in.FromCanon(r.Body); err != nil {
			return nil, bad(err)
		}
		a, err := o.Preset(ctx, in)
		return a.ToCanon(), err
	}
	return nil, &plugin.CallError{Code: plugin.CodeUnsupported, Message: "no such operation"}
}

// failed is the wire's error document of code, with message.
func failed(code plugin.Code, message string) ([]byte, error) {
	return document(plugin.Response{Error: &plugin.Error{Code: code, Message: message}})
}

// document is r in canonical form, ended by a newline.
func document(r plugin.Response) ([]byte, error) {
	out, err := canon.Encode(r.ToCanon(), canon.Canonical)
	if err != nil {
		return nil, fmt.Errorf("%w: the stand-in cannot write its answer: %w", port.ErrTrap, err)
	}
	return append(out, '\n'), nil
}
