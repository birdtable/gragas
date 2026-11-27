package generate

import (
	"fmt"
	"slices"
)

// defaultProblems lists each field whose default is no value of the field's
// own shape, reached through its references: a string shorter than the
// shape's minLength, a value outside its enum, an empty object where the
// object has required fields. Writing such a default would hand the reader a
// value the schema itself refuses, so the generators stop at it by field name.
func (t *tree) defaultProblems() []string {
	var out []string
	for _, d := range t.Decls {
		for _, f := range d.T.Fields {
			if !f.HasDefault {
				continue
			}
			if why := t.rejects(f.T, f.Default); why != "" {
				out = append(out, fmt.Sprintf("%s: %s: the default %s %s", d.File, fieldPath(d, f), jsonOf(f.Default), why))
			}
		}
	}
	return out
}

func fieldPath(d *decl, f field) string { return d.Name + "." + f.Key }

// rejects says why the shape refuses v, or is empty when it takes it.
func (t *tree) rejects(ty *typ, v any) string {
	if v == nil {
		if ty.Nullable {
			return ""
		}
		return "is null and the field does not take null"
	}
	r := t.resolve(ty)
	switch r.Kind {
	case kEnum:
		return rejectsEnum(r, v)
	case kConst:
		if s, ok := v.(string); !ok || s != r.Const {
			return "is not the constant of the field"
		}
	case kString:
		return rejectsString(r, v)
	case kInteger, kNumber:
		if !isNumber(v) {
			return "is not a number"
		}
	case kBool:
		if _, ok := v.(bool); !ok {
			return "is not a boolean"
		}
	case kArray:
		return rejectsArray(r, v)
	case kMap:
		if _, ok := v.(map[string]any); !ok {
			return "is not an object"
		}
	case kObject:
		return rejectsObject(r, v)
	}
	return ""
}

func rejectsEnum(r *typ, v any) string {
	if s, ok := v.(string); !ok || !slices.Contains(r.Enum, s) {
		return "is outside the enum it refers to"
	}
	return ""
}

func rejectsString(r *typ, v any) string {
	s, ok := v.(string)
	if !ok {
		return "is not a string"
	}
	if r.MinLen != nil && float64(len([]rune(s))) < *r.MinLen {
		return fmt.Sprintf("is shorter than the %g characters the shape needs", *r.MinLen)
	}
	return ""
}

func rejectsArray(r *typ, v any) string {
	l, ok := v.([]any)
	if !ok {
		return "is not an array"
	}
	if r.MinItems != nil && float64(len(l)) < *r.MinItems {
		return fmt.Sprintf("has fewer than the %g items the shape needs", *r.MinItems)
	}
	return ""
}

func rejectsObject(r *typ, v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return "is not an object"
	}
	for _, f := range r.Fields {
		if _, has := m[f.Key]; f.Required && !has {
			return "lacks the required field " + f.Key
		}
	}
	return ""
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int64, float64:
		return true
	}
	return false
}
