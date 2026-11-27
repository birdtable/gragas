package coreapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/model"
)

// A tracked path with a name no system can hold is not compared for a twin
// that differs only by case. These are that row in the run the
// project check makes: what it is, that it says no name of the
// list, and that the project check asks the session's FileNames port.

// notCompared is what every such row says after its path.
const notCompared = "was not checked for a twin that differs only by case"

// TestAPathNotComparedIsAPartOfTheTrackedCheckAndEndsNothing: aux is a name
// the port refuses, so aux/a.md is not compared. Its row is a part of the
// tracked-files check, in both shapes: the links and every other path were
// judged, so it says the path and never that the rules were not checked, it
// carries no tool, no cause and no bindings, and the run ends done as the
// same tree without the name does. The pair beside it is still found.
// Risk: a row that reads as the whole check unmade, or that ends a run for a
// cause none of model.GateCauses names; or the path dropped, a clean answer.
// Planted failure: checkRepo drops CheckTracked's rows.
func TestAPathNotComparedIsAPartOfTheTrackedCheckAndEndsNothing(t *testing.T) {
	t.Parallel()
	files := map[string]string{"aux/a.md": "a", "notes/B.md": "b", "notes/b.md": "b"}
	r := coreapi.CheckRepo(context.Background(), trackedAudit(t, files, nil))
	rows := notChecked(r, audit.KeyTracked)
	if len(rows) != 1 || !rows[0].Partial || !strings.HasPrefix(rows[0].Why, "aux/a.md "+notCompared+": ") {
		t.Fatalf("tracked rows %+v; want the one path, as a part of the check, with the port's reason", rows)
	}
	if s := rows[0].String(); strings.Contains(s, "rules were not checked") {
		t.Errorf("the row reads as the whole check unmade: %q", s)
	}
	if got := trackedOnly(r); len(got) != 1 || got[0].File != "notes/B.md" {
		t.Errorf("findings %+v; want the pair beside the refused name still found", got)
	}
	p := repoCheck(r)
	said := rowSaying(p, notCompared)
	if len(said) != 1 || said[0] != (model.NotChecked{Msg: rows[0].String()}) {
		t.Errorf("the project check's rows %+v; want the one row, with no tool, no cause and no bindings", said)
	}
	if p.Word() != model.Done || len(p.Unjudged()) != 0 {
		t.Errorf("ended %q with %+v left unjudged; want done: a part not compared ends nothing", p.Word(), p.Unjudged())
	}

	control := repoCheck(coreapi.CheckRepo(context.Background(), trackedAudit(t, map[string]string{"docs/a.md": "a", "notes/B.md": "b", "notes/b.md": "b"}, nil)))
	if len(rowSaying(control, notCompared)) != 0 || control.Word() != model.Done {
		t.Errorf("with no refused name: rows %+v, ended %q; want no such row", control.NotChecked, control.Word())
	}
}

// TestAPathNotComparedSaysNoNameOfTheList: the row holds a tracked path and
// the port's reason, and here the refused name is a name of the list with a
// dot at its end. Neither shape of the run says the name, and the row is
// still there, the name hidden.
// Risk: a names list read back off the screen, a path at a time, by tracking
// files no system can hold.
// Planted failure: the rows go to the result past masked.
func TestAPathNotComparedSaysNoNameOfTheList(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"team/zorbquux./a.md": "a", "notes/b.md": "b"}, nil)
	a.Names = listOf(t, "zorbquux")
	r := coreapi.CheckRepo(context.Background(), a)
	rows := notChecked(r, audit.KeyTracked)
	if len(rows) != 1 || !strings.Contains(rows[0].Why, notCompared) || !strings.Contains(rows[0].Why, "/a.md") {
		t.Fatalf("tracked rows %+v; want the one path not compared", rows)
	}
	p := repoCheck(r)
	if len(rowSaying(p, notCompared)) != 1 {
		t.Fatalf("the project check's rows %+v; want the one path not compared", p.NotChecked)
	}
	for shape, s := range map[string]string{"the rows": fmt.Sprintf("%+v", r.NotChecked), "the findings": fmt.Sprintf("%+v", r.Findings),
		"the project check": fmt.Sprintf("%+v %+v", p.Findings, p.NotChecked)} {
		if strings.Contains(strings.ToLower(s), "zorbquux") {
			t.Errorf("%s say the name:\n%s", shape, s)
		}
	}
}

// TestCheckProjectAsksTheSessionsFileNames: below a pin, with a tracked path
// that holds a name no system can hold and a name of the list, the project
// check gives each tracked path its key through the session's FileNames port
// and says the one row: a part of the tracked-files check, the name hidden on
// screen and in the document, the run ending done.
// Risk: the project check calls a port it was never handed, on the first
// tracked path of every project; or the row says a name of the list.
// Planted failure: checkProject hands repoChecks no FileNames.
func TestCheckProjectAsksTheSessionsFileNames(t *testing.T) {
	t.Parallel()
	files := map[string]string{"Taskfile.yml": "version: '3'\ntasks:\n  gate: {cmds: [echo]}\n", "a.go": "package p\n",
		"team/zorbquux./a.md": "a", "notes/B.md": "b", "notes/b.md": "b"}
	w, l := projectOf(t, repoRulesOfGo(), files, "go")
	got, err := checkIn(t, w, l, tracked(t, w, l, files))
	if err != nil {
		t.Fatal(err)
	}
	said := rowSaying(got, notCompared)
	if len(said) != 1 || said[0].Tool != "" || said[0].Cause != "" || !strings.HasPrefix(said[0].Msg, "gragas tracked: ") {
		t.Fatalf("rows %+v; want the one path not compared, a part of the tracked-files check with no tool and no cause", got.NotChecked)
	}
	if fold := ofKind(got, model.GateTrackedFiles); len(fold) != 1 || fold[0].Entity != "notes/B.md" {
		t.Errorf("tracked-files findings %+v; want the pair, found by the port's keys", fold)
	}
	if got.Word() != model.Done {
		t.Errorf("ended %q, want done\n%+v", got.Word(), got.NotChecked)
	}
	doc, err := json.Marshal(coreapi.ProjectDocOf(got, got.Word()))
	if err != nil {
		t.Fatal(err)
	}
	for shape, s := range map[string]string{"the check": fmt.Sprintf("%+v %+v", got.Findings, got.NotChecked), "the document": string(doc)} {
		if strings.Contains(strings.ToLower(s), "zorbquux") {
			t.Errorf("%s says the name:\n%s", shape, s)
		}
	}
	if !strings.Contains(string(doc), notCompared) {
		t.Errorf("the document does not hold the row:\n%s", doc)
	}
}
