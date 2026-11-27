package coreapi_test

import (
	"reflect"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/audit"
)

// leaves is the exported leaf fields of a struct type, an embedded struct
// flattened, as "Name type json-tag" in order.
func leaves(t reflect.Type) []string {
	var out []string
	for f := range t.Fields() {
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			out = append(out, leaves(f.Type)...)
			continue
		}
		if f.IsExported() {
			out = append(out, f.Name+" "+f.Type.String()+" "+f.Tag.Get("json"))
		}
	}
	return out
}

// TestEveryFieldOfTheAuditResultIsConvertedToTheMirror holds the one
// conversion (conformanceOf) to what the audit context says: each result type
// of audit has a mirror in this layer whose fields are the same in name, type
// and JSON name and in the same order, so a field added in audit and not here
// fails this test before it is dropped from a command's output.
func TestEveryFieldOfTheAuditResultIsConvertedToTheMirror(t *testing.T) {
	t.Parallel()
	for _, p := range []struct {
		name          string
		audit, mirror reflect.Type
	}{
		{"Finding", reflect.TypeFor[audit.Finding](), reflect.TypeFor[coreapi.ConfigFinding]()},
		{"Exempt", reflect.TypeFor[audit.Exempt](), reflect.TypeFor[coreapi.ConfigExempt]()},
		{"NotChecked", reflect.TypeFor[audit.NotChecked](), reflect.TypeFor[coreapi.ConfigNotChecked]()},
	} {
		a, m := leaves(p.audit), leaves(p.mirror)
		if len(a) == 0 || !reflect.DeepEqual(a, m) {
			t.Errorf("%s: audit has %q, the mirror has %q", p.name, a, m)
		}
	}
	res, conf := reflect.TypeFor[audit.Result](), reflect.TypeFor[coreapi.Conformance]()
	for f := range res.Fields() {
		cf, ok := conf.FieldByName(f.Name)
		// Halt is an error, which is carried as it is, so AsRefusal reads the
		// adapter's own refusal from it.
		if f.Name == "Halt" {
			if !ok || cf.Type != reflect.TypeFor[error]() || f.Type != reflect.TypeFor[error]() {
				t.Errorf("audit.Result.Halt is not carried as an error in Conformance")
			}
			continue
		}
		if !ok || cf.Type.Kind() != reflect.Slice || cf.Type.Elem().Kind() != reflect.Struct || cf.Type.Elem().NumField() == 0 {
			t.Errorf("audit.Result.%s has no slice of the mirror in Conformance", f.Name)
		}
	}
	if res.NumField() != 4 {
		t.Errorf("audit.Result has %d fields, the conversion handles 4: Findings, Exempt, NotChecked, Halt", res.NumField())
	}
}
