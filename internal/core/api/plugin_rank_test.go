package coreapi_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// valueBook is conflictBook with go.errors.1 and .2 setting wrap at one
// severity and two values, warn and strict, with lint-a tested at 2.5.0: what
// the two values are worth against each other is the plugin's word alone.
func valueBook() map[string]string {
	files := conflictBook()
	rules := strings.Replace(conflictRules, "key: wrap, severity: error,", "key: wrap, value: warn, severity: error,", 1)
	rules = strings.Replace(rules, "key: wrap, severity: warning,", "key: wrap, value: strict, severity: error,", 1)
	files["language/go/rules/errors.yaml"] = rules
	files["rulebook.yaml"] = bookIdentity + "tested:\n  tools:\n    - {name: lint-a, version: 2.5.0}\n  min_engine: 0.1.0\n"
	return files
}

// ranking installs a plugin of lint-a, serving 2.x, that answers each rank
// with word and keeps the requests it was asked.
func ranking(w *porttest.World, root model.Root, word plugin.Word) (*porttest.StandinPlugin, func() []plugin.RankRequest) {
	p := servedBy(w, root, "a-lint", "1.0.0", "lint-a")
	p.Facts.ToolVersions = ">=2.0.0 <3.0.0"
	var mu sync.Mutex
	var asked []plugin.RankRequest
	p.OnRank = func(in plugin.RankRequest) (plugin.RankResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, in)
		return plugin.RankResponse{Words: []plugin.Word{word}}, nil
	}
	return p, func() []plugin.RankRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
}

// TestListConflictsAsksThePluginOfTheToolWhetherTwoValuesAreTheSame: two values that differ as text are one value where the plugin that
// serves the tool ranks them the same, and a conflict where it ranks one
// weaker or cannot rank them; it is asked once, at the tested
// version, for the one pair, and opened and closed once. conflictBook, whose
// values are the same as text, lists with no plugin installed: a read with
// nothing to rank asks for none. The plant is the weaker word, which keeps
// the conflict.
func TestListConflictsAsksThePluginOfTheToolWhetherTwoValuesAreTheSame(t *testing.T) {
	t.Parallel()
	warn, err := link.Canon("warn")
	if err != nil {
		t.Fatal(err)
	}
	strict, err := link.Canon("strict")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		word      plugin.Word
		conflicts int
	}{
		{plugin.WordSame, 0},
		{plugin.WordWeaker, 1},
		{plugin.WordTighter, 1},
		{plugin.WordUnknown, 1},
	} {
		w, root := listWorld(t, valueBook())
		p, asked := ranking(w, root, tc.word)
		l, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
		if err != nil || len(l.Conflicts) != tc.conflicts {
			t.Fatalf("%s: ListConflicts = %+v, %v; want %d conflicts", tc.word, l, err, tc.conflicts)
		}
		if tc.conflicts == 1 && !slices.Equal(l.Conflicts[0].Differs, []string{model.DiffValue}) {
			t.Errorf("%s: conflict = %+v; want wrap, differing in value", tc.word, l.Conflicts[0])
		}
		got := asked()
		if len(got) != 1 || got[0].ToolVersion != "2.5.0" || len(got[0].Pairs) != 1 {
			t.Fatalf("%s: the plugin was asked %+v; want one pair at 2.5.0", tc.word, got)
		}
		pair := got[0].Pairs[0]
		pv := []any{pair.Preset, pair.Found}
		if pair.Key != "wrap" || pair.Aspect != plugin.AspectValue ||
			!reflect.DeepEqual(pv, []any{warn, strict}) && !reflect.DeepEqual(pv, []any{strict, warn}) {
			t.Errorf("%s: the pair asked = %+v; want wrap's values warn and strict", tc.word, pair)
		}
		if opened, closed := p.Opened(); opened != 1 || closed != 1 {
			t.Errorf("%s: opened %d, closed %d; want once each", tc.word, opened, closed)
		}
	}
	w, root := listWorld(t, conflictBook())
	if l, err := coreapi.ListConflicts(context.Background(), w.Ports(), root); err != nil || len(l.Conflicts) != 1 {
		t.Errorf("no value to rank, no plugin: %+v, %v; want the severity conflict and no error", l, err)
	}
}

// TestListConflictsEndsNotReadyWhereNoPluginCanRankAValue: a value of a
// tool no plugin serves ends the read not ready, naming the tool and, as Next,
// the install; a plugin Open refuses as not approved names the approval; a
// plugin that fails the rank, or answers with other than one word, is the
// error, never the same value (fail closed).
func TestListConflictsEndsNotReadyWhereNoPluginCanRankAValue(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, valueBook())
	servedBy(w, root, "z-lint", "1.0.0", "lint-z")
	_, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
	u, ok := errors.AsType[*coreapi.UnservedTools](err)
	if !ok || !slices.Equal(u.Tools, []string{"lint-a"}) || !errors.Is(err, coreapi.ErrNotReady) ||
		!strings.Contains(err.Error(), "no plugin is installed for lint-a") {
		t.Fatalf("no plugin of lint-a: %v; want lint-a unserved, not ready", err)
	}
	if next, ok := coreapi.PluginNext(err); !ok || next != "gragas plugin add <folder>" {
		t.Errorf("the Next of an unserved tool = %q, %v", next, ok)
	}

	w, root = listWorld(t, valueBook())
	p, _ := ranking(w, root, plugin.WordSame)
	p.OpenErr = port.ErrNotApproved
	_, err = coreapi.ListConflicts(context.Background(), w.Ports(), root)
	if _, ok := errors.AsType[*coreapi.UnapprovedPlugin](err); !ok {
		t.Fatalf("an unapproved plugin: %v; want UnapprovedPlugin", err)
	}
	if next, ok := coreapi.PluginNext(err); !ok || next != "gragas plugin approve a-lint@1.0.0" {
		t.Errorf("the Next of an unapproved plugin = %q, %v", next, ok)
	}

	for name, answer := range map[string]func(plugin.RankRequest) (plugin.RankResponse, error){
		"a failed rank": func(plugin.RankRequest) (plugin.RankResponse, error) {
			return plugin.RankResponse{}, &plugin.CallError{Code: plugin.CodeUnsupported, Message: "no"}
		},
		"two words for one pair": func(plugin.RankRequest) (plugin.RankResponse, error) {
			return plugin.RankResponse{Words: []plugin.Word{plugin.WordSame, plugin.WordSame}}, nil
		},
	} {
		w, root := listWorld(t, valueBook())
		p, _ := ranking(w, root, plugin.WordSame)
		p.OnRank = answer
		_, err := coreapi.ListConflicts(context.Background(), w.Ports(), root)
		if err == nil || !strings.Contains(err.Error(), "the plugin a-lint 1.0.0 could not rank lint-a at wrap") {
			t.Errorf("%s: %v; want the rank's error", name, err)
		}
		if _, ok := coreapi.PluginNext(err); ok {
			t.Errorf("%s: a Next of the plugins for %v", name, err)
		}
		if opened, closed := p.Opened(); opened != 1 || closed != 1 {
			t.Errorf("%s: opened %d, closed %d; want once each", name, opened, closed)
		}
	}
}
