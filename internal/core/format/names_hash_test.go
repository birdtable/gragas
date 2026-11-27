package format

import (
	"strings"
	"testing"
)

// These lines are what gragas-dev names hash (internal/dev/scan NameHashes)
// writes for the stand-in names below: the SHA-256 of
// the words folded to lower case and joined by a space, then of the words run
// together. They were made by that hasher, not by this package, so a reader
// that drifts from the push scan's form turns the test red.
const (
	hashAlex        = "4135aa9dc1b842a653dea846903ddb95bfb8c5a10c504a7fa16e10bc31d1fdf0"
	hashTwoWords    = "a03f1d611645eb53ad16c1af546ca0792dc884505bab57ede80f4dad6b911d3a"
	hashTwoWordsRun = "6875a6f9b0c997b6fe57911bd1e08c027a537eadd44fc4824bc4033719b0aba6"
	hashExampleCorp = "ed1b86576e743486a66278dfaaa58e98a3fc59d845b1d245e41776e89c4aa407"
	hashExampleRun  = "53410ed29282959d64b9cc36e0b47a955e01132d1c52e7c53bae381877453cc2"
	hashThreeWords  = "09388bd41376a6ae891623f1ab595a48345ae43d437e11253b425ce33ea31401"
)

func hashedNames(t *testing.T) *Names {
	t.Helper()
	n, err := ParseNames([]byte(strings.Join([]string{
		"# made by gragas-dev names hash from stand-in names", hashAlex, hashTwoWords, hashTwoWordsRun, hashExampleCorp, hashExampleRun, hashThreeWords, "",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestAHashLineMadeByTheScansHasherIsFound: each line the scan's hasher wrote
// for a stand-in name finds that name in a text, in the spellings the scan
// finds it in, and finds nothing in a clean control. Risk: one hash file that
// the push scan reads and this check does not.
func TestAHashLineMadeByTheScansHasherIsFound(t *testing.T) {
	t.Parallel()
	n := hashedNames(t)
	for text, want := range map[string]bool{
		"Wrap the error, Alex.":              true,
		"ALEX":                               true,
		"two words":                          true,
		"the TwoWords type":                  true,
		"twowords":                           true,
		"the ExampleCorp plan":               true,
		"example_corp":                       true,
		"Example Host Name":                  true,
		"example-host-name stays":            true,
		"alexander and alexa":                false, // a name is a whole word, not a prefix
		"Wrap the error and return it.":      false,
		"two or three words":                 false,
		"example host":                       false, // a part of a name is not the name
		"Example Host Name Of Five Words So": true,
	} {
		got := n.Find(text)
		if (got != "") != want {
			t.Errorf("Find(%q) = %q, found %v want %v", text, got, got != "", want)
		}
		if got != "" && got != MaskedName {
			t.Errorf("Find(%q) = %q: a hashed name is never printed, only %q", text, got, MaskedName)
		}
	}
}

// TestAHashedAndARegexLineLiveInOneFile: a file holds both forms, comments
// and blanks, and a hash line is not compiled as a pattern (a 64-digit line
// as a regex would match its own digits).
func TestAHashedAndARegexLineLiveInOneFile(t *testing.T) {
	t.Parallel()
	n, err := ParseNames([]byte("# c\n\n" + hashAlex + "\n\\bacme\\b\n" + strings.ToUpper(hashExampleCorp) + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		"by Acme":                    "Acme",
		"by Alex":                    MaskedName,
		"the ExampleCorp":            MaskedName, // an upper-case digest is the same hash
		"the " + hashAlex + " digit": "",         // the digits of a hash are not a name
		"nothing here":               "",
	} {
		if got := n.Find(text); got != want {
			t.Errorf("Find(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestAFindingNeverHoldsAHashedName: the finding a hashed name makes has the
// mask where the name was and nothing of the name.
func TestAFindingNeverHoldsAHashedName(t *testing.T) {
	t.Parallel()
	n := hashedNames(t)
	m := n.Find("only Alex would write this")
	if m != MaskedName || strings.Contains(strings.ToLower(m), "alex") {
		t.Errorf("Find = %q, want the mask", m)
	}
}

// A name the list holds only run together is found in text that writes it in
// camel case, where the words are split by the case turning.
func TestANameHeldOnlyRunTogetherIsFoundWrittenInCamelCase(t *testing.T) {
	t.Parallel()
	n, err := ParseNames([]byte(hashExampleRun + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{"made by ExampleCorp": MaskedName, "made by example corp": ""} {
		if got := n.Find(text); got != want {
			t.Errorf("Find(%q) = %q, want %q", text, got, want)
		}
	}
}
