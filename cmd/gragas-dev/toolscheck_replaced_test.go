package main

import "testing"

// TestAReplacedModulesSumIsTheLineOfItsReplacement (gragas#2447): go version -m
// prints a replaced main module's mod line with no sum, and the sum is on the
// => line after it, which names the replacement's path. That binary is built
// from the lock when the => line is one of its lines, and not when it is
// missing, is not a line of the lock, or something else follows the bare mod.
func TestAReplacedModulesSumIsTheLineOfItsReplacement(t *testing.T) {
	t.Parallel()
	sums := map[string]bool{
		"example.com/tool-replaced v1.0.0 h1:aaa=": true,
		"example.com/lib-replaced v2.0.0 h1:bbb=":  true,
		"example.com/plain v3.0.0 h1:ccc=":         true,
	}
	const header = "bin: go1.27.4\n\tpath\texample.com/tool/cmd/tool\n"
	for _, tc := range []struct {
		name string
		out  string
		want bool
	}{
		{"a mod with its sum is held", header + "\tmod\texample.com/plain\tv3.0.0\th1:ccc=\n", true},
		{"a replaced mod is held by its replacement line", header + "\tmod\texample.com/tool\tv1.0.0\n\t=>\texample.com/tool-replaced\tv1.0.0\th1:aaa=\n", true},
		{"a replaced dep is held by its replacement line", header + "\tmod\texample.com/plain\tv3.0.0\th1:ccc=\n\tdep\texample.com/lib\tv2.0.0\n\t=>\texample.com/lib-replaced\tv2.0.0\th1:bbb=\n", true},
		{"a replaced mod with no replacement line is not held", header + "\tmod\texample.com/tool\tv1.0.0\n", false},
		{"a replaced mod whose replacement is not a line of the lock is not held", header + "\tmod\texample.com/tool\tv1.0.0\n\t=>\texample.com/tool-replaced\tv1.0.0\th1:zzz=\n", false},
		{"a replaced mod followed by a dep is not held", header + "\tmod\texample.com/tool\tv1.0.0\n\tdep\texample.com/plain\tv3.0.0\th1:ccc=\n", false},
		{"no mod line is not held", header, false},
	} {
		if got := versionLinesAreTheLocks(tc.out, sums); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
