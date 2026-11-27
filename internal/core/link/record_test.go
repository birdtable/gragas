package link_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// aRecord is a whole binding record of a parent's rule, as bind writes one.
const aRecord = `subject: std.errors.1
tool: lint-a
target: source
key: wrapcheck
value: true
options:
  ignore: [fmt]
severity: error
rung: 1
run_point: commit
remedy: wrap the error with the call that failed
`

func TestABindingRecordIsReadWhole(t *testing.T) {
	got, refused := link.ReadRecord("bindings/std-errors-1.yaml", []byte(aRecord))
	if len(refused) > 0 {
		t.Fatalf("refused: %v", refused)
	}
	want := model.Tightening{
		Path: "bindings/std-errors-1.yaml", Subject: "std.errors.1", Text: aRecord,
		Binding: model.Binding{
			Tool: "lint-a", Target: model.TargetSource, Key: "wrapcheck", Value: true,
			Options: map[string]any{"ignore": []any{"fmt"}}, Severity: model.SeverityError, Rung: 1,
			RunPoint: model.RunCommit, Remedy: "wrap the error with the call that failed",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read\n%#v\nwant\n%#v", got, want)
	}
}

func TestABindingRecordThatCannotBeReadIsRefusedSayingWhy(t *testing.T) {
	without := func(k string) string {
		var keep []string
		for l := range strings.SplitSeq(aRecord, "\n") {
			if !strings.HasPrefix(l, k+":") {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n")
	}
	for name, c := range map[string]struct{ doc, says string }{
		"the subject not first":             {"tool: lint-a\n" + strings.Replace(aRecord, "tool: lint-a\n", "", 1), "the first line is subject"},
		"no subject":                        {without("subject"), "subject is missing"},
		"a subject not an id":               {strings.Replace(aRecord, "std.errors.1", "std.errors", 1), "a rule id"},
		"no target":                         {without("target"), "target is missing"},
		"no rung":                           {without("rung"), "rung is missing"},
		"no run point":                      {without("run_point"), "run_point is missing"},
		"no severity":                       {without("severity"), "severity is missing"},
		"a rung past the top":               {strings.Replace(aRecord, "rung: 1", "rung: 7", 1), "from 0 to 6"},
		"a rung not a number":               {strings.Replace(aRecord, "rung: 1", "rung: one", 1), "whole number"},
		"a severity unknown":                {strings.Replace(aRecord, "severity: error", "severity: fatal", 1), "one of error, warning, info"},
		"a run point unknown":               {strings.Replace(aRecord, "run_point: commit", "run_point: nightly", 1), "must be one of"},
		"tightens, which the subject says":  {aRecord + "tightens: std.errors.2\n", "tightens is not a field of a binding record"},
		"a key no binding has":              {aRecord + "weaker: true\n", "weaker is not a field"},
		"a conditional":                     {aRecord + "when: ci\n", "conditional"},
		"options not a mapping":             {strings.Replace(aRecord, "options:\n  ignore: [fmt]\n", "options: [fmt]\n", 1), "options must be a mapping"},
		"an option's name with a newline":   {strings.Replace(aRecord, "  ignore: [fmt]", "  \"ig\\nnore\": [fmt]", 1), `the option "ig\nnore"`},
		"a remedy with a control character": {strings.Replace(aRecord, "remedy: wrap the error with the call that failed", "remedy: \"\\e[31mwrap\"", 1), "remedy holds a control character"},
		"a list, not a mapping":             {"- subject: std.errors.1\n", "must be a mapping"},
		"not YAML":                          {"subject: [\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, refused := link.ReadRecord("bindings/std-errors-1.yaml", []byte(c.doc))
			if len(refused) == 0 {
				t.Fatalf("read %#v, want it refused saying %q", got, c.says)
			}
			if !reflect.DeepEqual(got, model.Tightening{}) {
				t.Errorf("refused and also read %#v", got)
			}
			var all []string
			for _, r := range refused {
				if r.Path != "bindings/std-errors-1.yaml" {
					t.Errorf("refusal at %q, want the record's path", r.Path)
				}
				all = append(all, r.String())
			}
			if !strings.Contains(strings.Join(all, "\n"), c.says) {
				t.Errorf("refused %q, want one saying %q", all, c.says)
			}
		})
	}
}

func TestEachRecordIsReadOrRefusedAndNeverBoth(t *testing.T) {
	got, refused := link.ReadRecords([]model.Opaque{
		{Path: "bindings/a.yaml", Data: []byte(aRecord)},
		{Path: "bindings/b.yaml", Data: []byte("subject: std.errors.2\n")},
	})
	if len(got) != 1 || got[0].Path != "bindings/a.yaml" {
		t.Errorf("read %v, want bindings/a.yaml alone", got)
	}
	if len(refused) == 0 || refused[0].Path != "bindings/b.yaml" {
		t.Errorf("refused %v, want bindings/b.yaml", refused)
	}
}

// FuzzReadRecord holds that a record is read or refused and never both, that
// a refusal names the record, and that what is read is a binding a borrower
// can hold: its entry named and its rung in range (tests.generated.1).
func FuzzReadRecord(f *testing.F) {
	f.Add([]byte(aRecord))
	f.Add([]byte("subject: std.errors.1\ntool: a\ntarget: source\nkey: k\nseverity: info\nrung: 0\nrun_point: edit\n"))
	f.Add([]byte(strings.Replace(aRecord, "rung: 1", "rung: -1", 1)))
	f.Add([]byte(strings.Replace(aRecord, "value: true", "value: {nested: [1, 2]}", 1)))
	f.Add([]byte(strings.Replace(aRecord, "  ignore: [fmt]", "  \"\\u200b\": 1", 1)))
	f.Add([]byte(aRecord + "subject: std.errors.2\n"))
	f.Add([]byte("subject: &a std.errors.1\ntool: *a\n"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		got, refused := link.ReadRecord("bindings/x.yaml", data)
		if len(refused) > 0 {
			if !reflect.DeepEqual(got, model.Tightening{}) {
				t.Fatalf("refused and read %#v", got)
			}
			for _, r := range refused {
				if r.Path != "bindings/x.yaml" || r.Msg == "" {
					t.Fatalf("a refusal that names no record or says nothing: %#v", r)
				}
			}
			return
		}
		b := got.Binding
		if got.Subject == "" || b.Tool == "" || b.Target == "" || b.Key == "" || b.Severity == "" || b.RunPoint == "" {
			t.Fatalf("read a record that leaves out its entry: %#v", got)
		}
		if b.Rung < 0 || b.Rung > model.MaxRung {
			t.Fatalf("read a rung out of range: %d", b.Rung)
		}
		if got.Text != string(data) {
			t.Fatalf("the text is not the record as written")
		}
	})
}
