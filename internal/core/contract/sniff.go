package contract

import "github.com/birdtable/gragas/internal/canon"

// own is the top-level field only one kind of document has, which names it: a
// document carries its contract version and no kind.
func own(k Kind) string {
	switch k {
	case Bundle:
		return "rules"
	case Report:
		return "sections"
	case Changelog:
		return "releases"
	case SARIF:
		return "runs"
	case Plan:
		return "plan"
	case Proposal:
		return "proposal"
	case Grid:
		return "cells"
	}
	return ""
}

// SniffKind is the kind of document doc is, from the one field only that
// kind has at its top level. It answers false when doc is not a JSON object
// or has the field of none of the kinds or of more than one: the caller then
// asks for the kind by name and lists Kinds.
func SniffKind(doc []byte) (Kind, bool) {
	if len(doc) > MaxDocument {
		return "", false
	}
	v, err := canon.Decode(doc)
	if err != nil {
		return "", false
	}
	obj, ok := v.(canon.Object)
	if !ok {
		return "", false
	}
	var found Kind
	for _, k := range Kinds() {
		if _, has := obj[own(k)]; has {
			if found != "" {
				return "", false
			}
			found = k
		}
	}
	return found, found != ""
}

// ParseKind is the kind named name, or false when the contract has no such
// document.
func ParseKind(name string) (Kind, bool) {
	k := Kind(name)
	return k, k.known()
}
