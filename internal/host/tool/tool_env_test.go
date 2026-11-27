package tool_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// TestTheEnvironmentOfARunIsThisSystemsTable: the whole environment a run is
// given, by system, with no Home and with one. The system's part is
// go-proc's proc.SystemEnv, which names what a program on this system needs;
// nothing else of this process's environment is taken, a planted
// GRAGAS_PLANTED_SECRET, HOME and USER among them. Planted failure:
// proc.Bare() in place of proc.SystemEnv(), which brings PATH and HOME.
// Risk: a variable of this process's, or of go-proc's Bare, reaches a tool
// on one system and not the others, and no test on that system sees it.
func TestTheEnvironmentOfARunIsThisSystemsTable(t *testing.T) {
	// serial: it sets variables of the process, to prove a run is handed none of them
	t.Setenv("GRAGAS_PLANTED_SECRET", "s3cret-value")
	t.Setenv(homeName(), "/planted-home")
	t.Setenv("USER", "planted")
	// The system's part is go-proc's SystemEnv, empty off Windows, so the
	// table is this system's with no branch on the system, and
	// nothing else of Bare, PATH, HOME and USER among it, may come through.
	system := proc.SystemEnv()
	home := "/home-of-the-run"
	for name, tc := range map[string]struct {
		cfg  tool.Config
		path string
		want []string
	}{
		"path and lang":    {tool.Config{Lang: "C"}, pathFolder(), append([]string{"PATH=" + pathFolder(), "LANG=C"}, system...)},
		"no path, no lang": {tool.Config{}, "", system},
		"a home": {tool.Config{Lang: "C", Home: home}, pathFolder(), append(append(append([]string{"PATH=" + pathFolder(), "LANG=C"}, system...),
			proc.HomeEnv(filepath.FromSlash(home))...), "GOENV=off", "GOTOOLCHAIN=local", "GOFLAGS=-modcacherw")},
	} {
		got := tool.Env(tool.New(tc.cfg), tc.path)
		if !slices.Equal(got, tc.want) || got == nil {
			t.Errorf("%s: %q, want %q, and never nil", name, got, tc.want)
		}
	}
}

// reservedHas says whether tool.Reserved names name, compared without regard
// to case, as the plugin host compares them.
func reservedHas(name string) bool {
	return slices.ContainsFunc(tool.Reserved(), func(r string) bool { return strings.EqualFold(r, name) })
}

// TestReservedHoldsEveryNameTheEnvironmentSets: every name a
// run with a Home, a PATH and a LANG is given is on the one list the plugin
// host refuses a grant's cache_vars by, and each kind of name is there by
// itself: env's own (PATH, LANG and the three of the go command), go-proc's
// Needed, its HomeEnv (homeCacheName, the one name a cache_vars pattern
// lets through, among them) and its SystemEnv, empty off Windows. Planted
// failure: Reserved without HomeEnv's names. Risk: a plugin names a variable
// gragas points below the Home, and its step reads the cache another's wrote.
func TestReservedHoldsEveryNameTheEnvironmentSets(t *testing.T) {
	t.Parallel()
	home := "/home-of-the-run"
	for _, kv := range tool.Env(tool.New(tool.Config{Lang: "C", Home: home}), pathFolder()) {
		name, _, _ := strings.Cut(kv, "=")
		if !reservedHas(name) {
			t.Errorf("the run is given %s, and Reserved does not name it", name)
		}
	}
	kinds := map[string][]string{
		"env's own":         {"PATH", "LANG", "GOENV", "GOTOOLCHAIN", "GOFLAGS"},
		"go-proc's Needed":  proc.Needed(),
		"go-proc's HomeEnv": append(namesOf(proc.HomeEnv(filepath.FromSlash(home))), homeCacheName(t)),
		// Empty off Windows: the row logs what it holds.
		"go-proc's SystemEnv": namesOf(proc.SystemEnv()),
	}
	for kind, names := range kinds {
		t.Logf("%s: %q", kind, names)
		for _, name := range names {
			if !reservedHas(name) {
				t.Errorf("%s: Reserved does not name %s", kind, name)
			}
		}
	}
}

// homeCacheName is the name of go-proc's HomeEnv that holds CACHE, the one
// name of it a cache_vars pattern lets through, read from go-proc so no test
// names a variable of the system.
func homeCacheName(t *testing.T) string {
	t.Helper()
	for _, name := range namesOf(proc.HomeEnv(proc.NeutralDir())) {
		if strings.Contains(name, "CACHE") {
			return name
		}
	}
	t.Fatalf("go-proc's HomeEnv names no cache folder: %q", namesOf(proc.HomeEnv(proc.NeutralDir())))
	return ""
}

// TestAHomeCacheMovesOneNameAndIsHeldApartFromTheHome: with a Home cache, a run's environment is
// the one a fresh Home gives, name for name and in order, save homeCacheName,
// which names the kept folder; a kept folder that is the Home, lies beneath
// it or holds it is refused, and the run is given no environment. Planted
// failure: env ignoring homeCache (the moved row red); env appending
// homeCacheName=homeCache itself, with no check (the refusal rows red).
// Risk: a kept cache inside the Home is removed with it, or a Home inside the
// kept cache carries one step's config and state into the next.
func TestAHomeCacheMovesOneNameAndIsHeldApartFromTheHome(t *testing.T) {
	t.Parallel()
	base := private(t)
	home, kept := base+"/the-step-home", filepath.FromSlash(base+"/kept")
	tl := tool.New(tool.Config{Lang: "C", Home: home})
	name := homeCacheName(t)
	fresh := tool.Env(tl, pathFolder())
	got, err := tool.EnvHomeCached(tl, pathFolder(), kept)
	if err != nil || len(got) != len(fresh) {
		t.Fatalf("a kept Home cache apart from the Home: %q, %v; want the %d variables of a fresh Home", got, err, len(fresh))
	}
	moved := 0
	for i, kv := range fresh {
		if strings.HasPrefix(kv, name+"=") {
			kv = name + "=" + kept
			moved++
		}
		if got[i] != kv {
			t.Errorf("variable %d = %q; want %q", i, got[i], kv)
		}
	}
	if moved != 1 {
		t.Errorf("a fresh Home gives %s %d times; want once", name, moved)
	}
	for what, at := range map[string]string{
		"the Home itself":  home,
		"beneath the Home": home + "/cache",
		"holding the Home": base,
	} {
		if env, err := tool.EnvHomeCached(tl, pathFolder(), filepath.FromSlash(at)); !errors.Is(err, port.ErrRefused) || env != nil {
			t.Errorf("a kept Home cache %s (%s): %q, %v; want a refusal and no environment", what, at, env, err)
		}
	}
}

func namesOf(env []string) []string {
	names := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	return names
}

// TestACacheVariableComesFirstAndGivesWay: a step's cache
// variables open the environment, so a name of env's own given after one is
// the one a program reads, as go-proc and os/exec keep the last. Planted
// failure: the cache names appended last. Risk: a manifest the opener let
// through names homeCacheName, and its folder replaces the Home's.
func TestACacheVariableComesFirstAndGivesWay(t *testing.T) {
	t.Parallel()
	home := "/home-of-the-run"
	name := homeCacheName(t)
	cache := []string{"GOCACHE=/cache/a", name + "=/cache/planted"}
	got := tool.EnvCached(tool.New(tool.Config{Lang: "C", Home: home}), pathFolder(), cache)
	if !slices.Equal(got[:len(cache)], cache) {
		t.Fatalf("the environment starts %q, want the cache variables %q first", got[:min(len(got), len(cache))], cache)
	}
	last := ""
	for _, kv := range got {
		if value, ok := strings.CutPrefix(kv, name+"="); ok {
			last = value
		}
	}
	if last == "/cache/planted" || last == "" {
		t.Errorf("%s is last %q, and the Home's is the one a program reads", name, last)
	}
}
