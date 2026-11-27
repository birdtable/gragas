package proof_test

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
)

// inputsJob is a job with a config set of two files, of a case, a neighbour
// placed beside it, and a review; the case is answered.
func inputsJob() (proof.Job, proof.Case) {
	c := proof.Case{Rule: "go.errors.2",
		Binding: model.Binding{Tool: "lint", Target: "code", Key: "no-x", Rung: 3, Pair: 1, PairPath: "a.go"},
		Pair:    proof.Pair{Digest: "sha256:aa"}}
	n := proof.Case{Rule: "go.errors.3",
		Binding: model.Binding{Tool: "lint", Target: "code", Key: "unused", Rung: 3, Pair: 1, PairPath: "b.go"},
		Pair:    proof.Pair{Digest: "sha256:bb"}}
	rv := proof.Case{Rule: "go.errors.4",
		Binding: model.Binding{Tool: "lint", Target: "code", Key: "doc", Rung: model.MaxRung, Pair: 1, PairPath: "c.go"},
		Pair:    proof.Pair{Digest: "sha256:cc"}}
	j := proof.Job{Tool: "lint", Version: "1.2.3", Adapter: "1.0.0", ConfigPath: "lint.yaml",
		Config: []model.File{{Path: "lint.yaml", Data: []byte("a: 1\n")}, {Path: "extra.yaml", Data: []byte("b: 2\n")}},
		Cases:  []proof.Case{c, n, rv}}
	return j, c
}

// TestEachInputOfACellChangesItsInputs is the staleness of one cell:
// a change to any one input is another digest, so a cell proven before the
// change is never read as fresh after it. A key that left one out would
// serve a stale pass; each row here is the plant for one.
func TestEachInputOfACellChangesItsInputs(t *testing.T) {
	t.Parallel()
	j0, c0 := inputsJob()
	before := proof.Inputs(j0, c0)
	for name, change := range map[string]func(*proof.Job, *proof.Case){
		"the pair":              func(_ *proof.Job, c *proof.Case) { c.Pair.Digest = "sha256:ab" },
		"the binding's key":     func(_ *proof.Job, c *proof.Case) { c.Binding.Key = "no-y" },
		"the binding's rung":    func(_ *proof.Job, c *proof.Case) { c.Binding.Rung = 4 },
		"the rule":              func(_ *proof.Job, c *proof.Case) { c.Rule = "go.errors.3" },
		"the tool":              func(j *proof.Job, _ *proof.Case) { j.Tool = "lint2" },
		"the tool version":      func(j *proof.Job, _ *proof.Case) { j.Version = "1.2.4" },
		"the adapter version":   func(j *proof.Job, _ *proof.Case) { j.Adapter = "1.0.1" },
		"no adapter":            func(j *proof.Job, _ *proof.Case) { j.Adapter = "" },
		"the config path":       func(j *proof.Job, _ *proof.Case) { j.ConfigPath = "extra.yaml" },
		"a config file's bytes": func(j *proof.Job, _ *proof.Case) { j.Config[1].Data = []byte("b: 3\n") },
		"a config file's path":  func(j *proof.Job, _ *proof.Case) { j.Config[1].Path = "other.yaml" },
		"a config file added": func(j *proof.Job, _ *proof.Case) {
			j.Config = append(j.Config, model.File{Path: "z.yaml"})
		},
		"a config file removed": func(j *proof.Job, _ *proof.Case) { j.Config = j.Config[:1] },
		// The cases placed beside it: their examples share its folder.
		"a neighbour's pair":   func(j *proof.Job, _ *proof.Case) { j.Cases[1].Pair.Digest = "sha256:bc" },
		"a neighbour's path":   func(j *proof.Job, _ *proof.Case) { j.Cases[1].Binding.PairPath = "pkg/b.go" },
		"a neighbour reviewed": func(j *proof.Job, _ *proof.Case) { j.Cases[1].Binding.Rung = model.MaxRung },
		"a neighbour added": func(j *proof.Job, _ *proof.Case) {
			j.Cases = append(j.Cases, proof.Case{Binding: model.Binding{Rung: 3, PairPath: "d.go"}, Pair: proof.Pair{Digest: "sha256:dd"}})
		},
		"a neighbour taken out":       func(j *proof.Job, _ *proof.Case) { j.Cases = slices.Delete(j.Cases, 1, 2) },
		"the neighbours in new order": func(j *proof.Job, _ *proof.Case) { j.Cases[0], j.Cases[1] = j.Cases[1], j.Cases[0] },
	} {
		j, c := inputsJob()
		change(&j, &c)
		if got := proof.Inputs(j, c); got == before {
			t.Errorf("%s changed and the inputs did not: %s", name, got)
		}
	}
}

// TestTheConfigSetsOrderIsNotAnInput: the set is the same set in any order,
// so listing it again in another order stales nothing.
func TestTheConfigSetsOrderIsNotAnInput(t *testing.T) {
	t.Parallel()
	j, c := inputsJob()
	before := proof.Inputs(j, c)
	j.Config[0], j.Config[1] = j.Config[1], j.Config[0]
	if got := proof.Inputs(j, c); got != before {
		t.Fatalf("the config set's order changed the inputs: %s, was %s", got, before)
	}
}

// TestWhatPlacesNothingStandsInNoKey: a review and a case with no pair place
// no file, so a change to their pair stales no neighbour, and their own cells,
// run in no folder, hold no neighbour.
func TestWhatPlacesNothingStandsInNoKey(t *testing.T) {
	t.Parallel()
	j, c := inputsJob()
	placed, review := proof.Inputs(j, c), proof.Inputs(j, j.Cases[2])
	j.Cases[2].Pair.Digest = "sha256:cd"
	j.Cases = append(j.Cases, proof.Case{Binding: model.Binding{Rung: 3}, Pair: proof.Pair{Digest: "sha256:ee"}})
	if got := proof.Inputs(j, c); got != placed {
		t.Errorf("a review's pair and a case with no pair changed a placed cell's inputs: %s, was %s", got, placed)
	}
	j, _ = inputsJob()
	j.Cases[1].Pair.Digest = "sha256:bc"
	if got := proof.Inputs(j, j.Cases[2]); got != review {
		t.Errorf("a neighbour's pair changed a review's inputs: %s, was %s", got, review)
	}
}

// TestBytesMovedBetweenPartsChangeTheInputs: each part is framed by its
// length, so the same bytes cut in another place are other inputs.
func TestBytesMovedBetweenPartsChangeTheInputs(t *testing.T) {
	t.Parallel()
	j, c := inputsJob()
	j.Tool, j.Version = "ab", "c"
	one := proof.Inputs(j, c)
	j.Tool, j.Version = "a", "bc"
	if proof.Inputs(j, c) == one {
		t.Error("tool ab at c and tool a at bc have one digest")
	}
	j, c = inputsJob()
	j.Config = []model.File{{Path: "a", Data: []byte("1:b")}}
	two := proof.Inputs(j, c)
	j.Config = []model.File{{Path: "a1:", Data: []byte("b")}}
	if proof.Inputs(j, c) == two {
		t.Error("a config file's path and bytes cut in another place have one digest")
	}
}

// TestInputsAreEqualOnlyWhereTheInputsAre is the property (tests.generated.2):
// over generated jobs, two digests are equal exactly when the jobs' inputs
// are, up to the order of the config set: for a placed case, the paths and
// pairs of the job's placed cases, in order, among them.
func TestInputsAreEqualOnlyWhereTheInputsAre(t *testing.T) {
	t.Parallel()
	r := &draw{state: 207}
	word := func() string { return []string{"", "a", "b", "ab", "1:", ":"}[r.IntN(6)] }
	rung := func() model.Rung { return []model.Rung{3, model.MaxRung}[r.IntN(2)] }
	gen := func() (proof.Job, proof.Case) {
		j := proof.Job{Tool: word(), Version: word(), Adapter: word(), ConfigPath: word()}
		for range r.IntN(3) {
			j.Config = append(j.Config, model.File{Path: word(), Data: []byte(word())})
		}
		for range r.IntN(3) {
			j.Cases = append(j.Cases, proof.Case{Binding: model.Binding{Rung: rung(), PairPath: word()}, Pair: proof.Pair{Digest: model.Digest(word())}})
		}
		c := proof.Case{Rule: model.RuleID(word()), Binding: model.Binding{Key: word(), Rung: rung(), PairPath: word()}, Pair: proof.Pair{Digest: model.Digest(word())}}
		return j, c
	}
	// beside is what of a job's cases stands in a placed case's key.
	beside := func(j proof.Job, c proof.Case) []string {
		var out []string
		if places(c) {
			for _, m := range j.Cases {
				if places(m) {
					out = append(out, m.Binding.PairPath, string(m.Pair.Digest))
				}
			}
		}
		return out
	}
	same := func(a proof.Job, ac proof.Case, b proof.Job, bc proof.Case) bool {
		if a.Tool != b.Tool || a.Version != b.Version || a.Adapter != b.Adapter || a.ConfigPath != b.ConfigPath ||
			proof.BindingText(ac.Binding.Ref(ac.Rule)) != proof.BindingText(bc.Binding.Ref(bc.Rule)) || ac.Pair.Digest != bc.Pair.Digest || len(a.Config) != len(b.Config) ||
			!slices.Equal(beside(a, ac), beside(b, bc)) {
			return false
		}
		left := map[string]int{}
		for _, f := range a.Config {
			left[f.Path+"\x00"+string(f.Data)]++
		}
		for _, f := range b.Config {
			left[f.Path+"\x00"+string(f.Data)]--
		}
		for _, n := range left {
			if n != 0 {
				return false
			}
		}
		return true
	}
	for range 20000 {
		a, ac := gen()
		b, bc := gen()
		if got, want := proof.Inputs(a, ac) == proof.Inputs(b, bc), same(a, ac, b, bc); got != want {
			t.Fatalf("equal digests %v, equal inputs %v:\n%+v %+v\n%+v %+v", got, want, a, ac, b, bc)
		}
	}
}

// draw is the tests' numbers: a fixed linear congruential sequence from its
// seed, so a draw is the same on every run and no test reads math/rand.
type draw struct{ state uint64 }

// IntN is a number from 0 up to, not including, n.
func (d *draw) IntN(n int) int {
	d.state = d.state*6364136223846793005 + 1442695040888963407
	return int((d.state >> 33) % uint64(n))
}

// Shuffle puts n things in a drawn order through swap.
func (d *draw) Shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		swap(i, d.IntN(i+1))
	}
}
