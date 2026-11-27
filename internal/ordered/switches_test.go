package ordered_test

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// TestLooseSwitchesAreTheBooleansAYAML12ReaderDoesNotRead: under the keys it
// is given, every value ParseYAML reads as a boolean that is not the plain
// word true or false is said with its line, key and word, and nothing else
// is: not the plain words, not a quoted word (a string to ParseYAML too, so a
// schema refuses it), not a key it was not given. Planted failure: plainWord
// that takes any case of true and false lets True through.
func TestLooseSwitchesAreTheBooleansAYAML12ReaderDoesNotRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in string
		want     []ordered.Switch
	}{
		{"the plain words", "a: {net: true, race: false}\n", nil},
		{"a quoted word", "a: {net: \"yes\", race: 'true'}\n", nil},
		{"a word that is no boolean", "a: {net: maybe, race: 1}\n", nil},
		{"a key it was not given", "a: {open: yes}\n", nil},
		{"yes", "a: {net: yes}\n", []ordered.Switch{{Line: 1, Key: "net", Value: "yes"}}},
		{"off", "a:\n  race: off\n", []ordered.Switch{{Line: 2, Key: "race", Value: "off"}}},
		{"a capital True", "a: {net: True}\n", []ordered.Switch{{Line: 1, Key: "net", Value: "True"}}},
		{"FALSE", "a: {net: FALSE}\n", []ordered.Switch{{Line: 1, Key: "net", Value: "FALSE"}}},
		{"a tagged true", "a: {net: !!bool true}\n", []ordered.Switch{{Line: 1, Key: "net", Value: "true"}}},
		{"an alias of on", "x: &o on\nm:\n  - {net: *o}\n", []ordered.Switch{{Line: 1, Key: "x", Value: "on"}, {Line: 3, Key: "net", Value: "on"}}},
		{"each in a list", "m:\n  - {net: no}\n  - {net: false}\n  - {net: Yes}\n", []ordered.Switch{{Line: 2, Key: "net", Value: "no"}, {Line: 4, Key: "net", Value: "Yes"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keys := []string{"net", "race"}
			if tc.name == "an alias of on" {
				keys = append(keys, "x")
			}
			got, err := ordered.LooseSwitches([]byte(tc.in), keys...)
			if err != nil {
				t.Fatalf("LooseSwitches: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("LooseSwitches(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
	if _, err := ordered.LooseSwitches([]byte("a: [\n")); err == nil {
		t.Error("a document ParseYAML cannot read gave no error")
	}
}
