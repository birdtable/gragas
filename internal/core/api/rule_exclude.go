package coreapi

// rule exclude and rule include: an overlay drops a rule of its parent on a
// decision, and lifts the exclusion on another (S4). The rules of what may be
// done are the lifecycle context's. This file reads the parent's rules for it,
// from the parent's bundle in the releases cache through contract.ReadBundle
// (A50), and hands them in as an argument (arch.ports.8). Only the rules'
// ids and statuses reach the plan, the fields the contract marks deciding
// (arch.ports.19); a bundle's prose is never read here.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	mc "github.com/birdtable/gragas/internal/model/contract"
	"github.com/birdtable/gragas/internal/port"
)

// OverlayAsk is a change asked of an overlay: the ask, whose Cache is the
// releases cache the parent is read from, and the contract's schemas its
// bundle is read by. A nil Contracts reads no bundle.
type OverlayAsk struct {
	model.ChangeAsk
	// Contracts is the Schema port over the contract's schemas.
	Contracts port.Schema
}

// RuleExclude is the change that excludes a parent's rule from the overlay
// in ask.Root. A parent whose bundle is not in the cache at its pin is
// ErrNotReady, "parent not fetched", naming the pin.
func RuleExclude(ctx context.Context, p port.Ports, ask OverlayAsk, req model.ExcludeRule) (*Change, *model.Refusal, error) {
	return overParent(ctx, p, ask, &req, &req.Files, lifecycle.PlanRuleExclude)
}

// RuleInclude is the change that lifts the overlay's exclusion of a parent's
// rule, with the same answers as RuleExclude.
func RuleInclude(ctx context.Context, p port.Ports, ask OverlayAsk, req model.IncludeRule) (*Change, *model.Refusal, error) {
	return overParent(ctx, p, ask, &req, &req.Files, lifecycle.PlanRuleInclude)
}

// overParent is the change plan makes of req, once the parent is read from
// its bundle in the cache; like withFiles, each reading sets the files the
// plan reads, at files.
func overParent[R any](ctx context.Context, p port.Ports, ask OverlayAsk, req *R, files *model.PlanFiles,
	plan func(model.Book, lifecycle.Parent, R, lifecycle.Moment) (lifecycle.Plan, error),
) (*Change, *model.Refusal, error) {
	return composeTo(ctx, p, ask.ChangeAsk, exclusionFiles, func(_ context.Context, b model.Book, f model.PlanFiles, m lifecycle.Moment) (lifecycle.Plan, error) {
		parent, err := parentOf(p, ask.Contracts, ask.Cache, b)
		if err != nil {
			return lifecycle.Plan{}, err
		}
		*files = f
		return plan(b, parent, *req, m)
	})
}

// ParentBundle is where the bundle of a parent at its pin is read, relative
// to the cache folder. The rulebook is free text, a short name or a full
// location, so its folder is named by the first sixteen hex digits of its
// digest and never by the text; the version is a semantic version. This is
// the placement A50 gives it, which only a fixture fills: a fetch, when there
// is one, keeps it or moves it here, and nowhere else.
func ParentBundle(e model.Extends) string {
	sum := string(apply.DigestBytes([]byte(e.Rulebook)))
	const head = len("sha256:")
	return path.Join("releases", "pinned", sum[head:head+16], e.Version, "bundle.json")
}

// ParentError is a parent the plan cannot read at its pin: one not in the
// releases cache, a pin that is not a version, or a bundle in a contract
// newer than this engine. It matches ErrNotReady, and its message is the
// sentence alone, without the closing word.
type ParentError struct {
	// Msg names the pin and what is not ready about it.
	Msg string
	// Err is what the bundle's reader answered, if it was read.
	Err error
}

func (e *ParentError) Error() string { return e.Msg }

// Is matches ErrNotReady.
func (e *ParentError) Is(target error) bool { return target == ErrNotReady }

func (e *ParentError) Unwrap() error { return e.Err }

// parentOf reads the parent of the book from its bundle in the cache. A book
// that is not an overlay with a parent has none, and the plan refuses it.
func parentOf(p port.Ports, contracts port.Schema, cache string, b model.Book) (lifecycle.Parent, error) {
	doc, err := parentBundle(p, contracts, cache, b)
	if err != nil || doc == nil {
		return lifecycle.Parent{}, err
	}
	e := b.Identity.Extends
	rules := make(map[model.RuleID]bool, len(doc.Rules))
	for _, r := range doc.Rules {
		rules[model.RuleID(r.ID)] = r.Status == mc.StatusRetired
	}
	return lifecycle.Parent{Rulebook: e.Rulebook, Version: e.Version, Rules: rules}, nil
}

// versionText says v is a semantic version as the identity schema writes it,
// so it is one path segment that names no folder above it.
func versionText(v string) bool {
	dots := 0
	for i := range len(v) {
		switch c := v[i]; {
		case c == '.' && i > 0 && v[i-1] != '.' && i < len(v)-1:
			dots++
		case c < '0' || c > '9':
			return false
		}
	}
	return dots == 2
}

// parentBundle reads the bundle of the book's parent from the releases cache at
// its pin: nil for a book that is no overlay with a parent, a *ParentError for
// a parent that is not there, not a version, or in a newer contract.
func parentBundle(p port.Ports, contracts port.Schema, cache string, b model.Book) (*mc.BundleDocument, error) {
	e := b.Identity.Extends
	if b.Identity.Kind != model.KindOverlay || e == nil {
		return nil, nil
	}
	pin := e.Rulebook + " " + e.Version
	if !versionText(e.Version) {
		return nil, &ParentError{Msg: fmt.Sprintf("rulebook.yaml pins the parent %s at %q, which is not a version; gragas validate lists it", e.Rulebook, e.Version)}
	}
	notFetched := &ParentError{Msg: "parent not fetched: " + pin + " is not in the releases cache, and this build fetches no parent (A50)"}
	if cache == "" {
		return nil, notFetched
	}
	root, err := p.Tree.Open(cache)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, notFetched
	}
	if err != nil {
		return nil, err
	}
	data, err := p.Tree.Read(root, ParentBundle(*e))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, notFetched
	}
	if err != nil {
		return nil, err
	}
	if contracts == nil {
		return nil, errors.New("this build reads no contract document, so the parent's bundle cannot be read")
	}
	doc, err := contract.ReadBundle(data, contract.Schemas{Check: contracts, Files: p.Assets})
	if _, ok := errors.AsType[*contract.NotReady](err); ok {
		return nil, &ParentError{Msg: "the bundle of the parent " + pin + ": " + err.Error(), Err: err}
	}
	if err != nil {
		return nil, fmt.Errorf("the bundle of the parent %s in the releases cache: %w", pin, err)
	}
	return &doc, nil
}
