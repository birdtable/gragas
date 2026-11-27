package project_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
)

// pinFile is the path every refusal names.
const pinFile = "proj/.gragas/pin.yaml"

// cleanPin is a pin with nothing wrong, a key a line: rulebook at 1, source
// at 2, parts at 3, format at 4. Each row below edits one key of it.
const cleanPin = "rulebook: fixture-base\nsource: rulebook\nparts: [base, go, web]\nformat: 1\n"

// with is cleanPin with key's line written as value, or taken out when value
// is "".
func with(key, value string) string {
	lines := strings.SplitAfter(cleanPin, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, key+":") {
			lines[i] = ""
			if value != "" {
				lines[i] = key + ": " + value + "\n"
			}
		}
	}
	return strings.Join(lines, "")
}

// refused holds ParsePin of doc to the one refusal want, a *PinError with no
// pin.
func refused(t *testing.T, doc, want string) {
	t.Helper()
	pin, err := project.ParsePin(pinFile, []byte(doc))
	var pe *project.PinError
	if !errors.As(err, &pe) || err.Error() != want || !reflect.DeepEqual(pin, model.Pin{}) {
		t.Errorf("ParsePin of %q = %+v, %v; want it refused: %s", doc, pin, err, want)
	}
}

func TestPinReadsACleanPin(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "pin", "clean.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		doc  string
		want model.Pin
	}{
		{string(data), model.Pin{Rulebook: "fixture-base", Source: "rulebook", Parts: []string{"base", "go", "web"}, Format: 1}},
		{with("source", "."), model.Pin{Rulebook: "fixture-base", Source: ".", Parts: []string{"base", "go", "web"}, Format: 1}},
		{with("source", "books/main"), model.Pin{Rulebook: "fixture-base", Source: "books/main", Parts: []string{"base", "go", "web"}, Format: 1}},
		{"format: 1\nparts:\n  - base\nsource: rulebook\nrulebook: fixture-base\n", model.Pin{Rulebook: "fixture-base", Source: "rulebook", Parts: []string{"base"}, Format: 1}},
	} {
		if got, err := project.ParsePin(pinFile, []byte(tc.doc)); err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParsePin of %q = %+v, %v; want %+v", tc.doc, got, err, tc.want)
		}
	}
}

func TestPinRefusesAnUnknownKey(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile(filepath.Join("testdata", "pin", "fixture.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for doc, want := range map[string]string{
		cleanPin + "kinds: [service]\n": pinFile + ":5: kinds is not a key of the pin: pin format 1 holds rulebook, source, parts and format",
		"version: 1.2.0\n" + cleanPin:   pinFile + ":1: version is not a key of the pin: pin format 1 holds rulebook, source, parts and format",
		// The clean fixture project's pin, as the spec writes one: release 1's
		// reader refuses its version, kinds and key by name, the first first.
		string(fixture): pinFile + ":2: version is not a key of the pin: pin format 1 holds rulebook, source, parts and format",
	} {
		refused(t, doc, want)
	}
}

func TestPinRefusesAKeyOfTheWrongType(t *testing.T) {
	t.Parallel()
	for doc, want := range map[string]string{
		with("rulebook", "[a]"):    pinFile + ":1: rulebook must be the rulebook's name, a string, and it is ['a']",
		with("source", "3"):        pinFile + ":2: source must be a folder of the repository, a string, and it is 3",
		with("parts", "base"):      pinFile + ":3: parts must be a list of part names, and it is 'base'",
		with("parts", "[base, 1]"): pinFile + ":3: part 2 must be a part's name, a string, and it is 1",
		with("format", "one"):      pinFile + ":4: format must be a whole number, 1, and it is 'one'",
		with("format", "1.5"):      pinFile + ":4: format must be a whole number, 1, and it is 1.5",
		with("format", "\"1\""):    pinFile + ":4: format must be a whole number, 1, and it is '1'",
	} {
		refused(t, doc, want)
	}
}

func TestPinRefusesASourceThatLeavesTheRepository(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"..", "../rulebook", "books/../../rulebook", "books/.."} {
		refused(t, with("source", source), pinFile+":2: source \""+source+"\" leaves the repository: write a folder inside it, with no .. step")
	}
}

func TestPinRefusesASourceThatIsNoFolderOfTheRepository(t *testing.T) {
	t.Parallel()
	for doc, want := range map[string]string{
		with("source", "\"\""):                                 pinFile + ":2: source \"\" is empty: write the folder of the repository that holds the rulebook, or . for this one",
		with("source", "/books/rulebook"):                      pinFile + ":2: source \"/books/rulebook\" is absolute: write the folder relative to the project, inside the repository",
		with("source", "https://example.com/fixture-base.git"): pinFile + ":2: source \"https://example.com/fixture-base.git\" is a git host or a mirror, not a path: release 1 reads a folder of the repository",
		with("source", "\"git@example.com:fixture-base.git\""): pinFile + ":2: source \"git@example.com:fixture-base.git\" is a git host or a mirror, not a path: release 1 reads a folder of the repository",
		with("source", "books//main"):                          pinFile + ":2: source \"books//main\" is not a clean path: write it slash-separated, with no empty or . step and no slash at the end",
		with("source", "./rulebook"):                           pinFile + ":2: source \"./rulebook\" is not a clean path: write it slash-separated, with no empty or . step and no slash at the end",
		with("source", "rulebook/"):                            pinFile + ":2: source \"rulebook/\" is not a clean path: write it slash-separated, with no empty or . step and no slash at the end",
	} {
		refused(t, doc, want)
	}
}

func TestPinRefusesAnEmptyFile(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{"", "\n\n", "# a pin to come\n", "---\n"} {
		refused(t, doc, pinFile+": is empty: a pin holds rulebook, source, parts and format")
	}
}

func TestPinRefusesAMissingKey(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"rulebook", "source", "parts", "format"} {
		refused(t, with(key, ""), pinFile+":1: has no "+key+" key: a pin holds rulebook, source, parts and format")
	}
}

func TestPinRefusesNoParts(t *testing.T) {
	t.Parallel()
	for doc, want := range map[string]string{
		with("parts", "[]"):            pinFile + ":3: parts is empty: name at least one part",
		with("parts", "[base, \"\"]"):  pinFile + ":3: part 2 is empty: write the part's name",
		with("parts", "[\" \", base]"): pinFile + ":3: part 1 is empty: write the part's name",
		with("parts", "~"):             pinFile + ":3: parts must be a list of part names, and it is None",
	} {
		refused(t, doc, want)
	}
}

func TestPinRefusesAnEmptyRulebookName(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"\"\"", "\"  \""} {
		refused(t, with("rulebook", value), pinFile+":1: rulebook is empty: write the rulebook's name")
	}
}

func TestPinFormatNewerIsNotReadyNamingBoth(t *testing.T) {
	t.Parallel()
	pin, err := project.ParsePin(pinFile, []byte(with("format", "2")+"kinds: [service]\n"))
	var nr *project.PinNotReady
	want := pinFile + ":4: the pin is format 2, and this gragas reads pin format 1: install a gragas that reads pin format 2"
	if !errors.As(err, &nr) || err.Error() != want || !reflect.DeepEqual(pin, model.Pin{}) {
		t.Errorf("ParsePin of a format 2 pin = %+v, %v; want not ready: %s", pin, err, want)
	}
}

func TestPinFormatOlderIsRefusedByAStatedLine(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"0", "-1"} {
		refused(t, with("format", n), pinFile+":4: format "+n+" is not a pin format: write format: 1")
	}
}

func TestPinRefusesWhatTheStrictReaderRefuses(t *testing.T) {
	t.Parallel()
	for doc, want := range map[string]string{
		cleanPin + "source: elsewhere\n":                                          pinFile + ":5: the key source is written again, first at line 2",
		"rulebook: &n fixture-base\nsource: rulebook\nparts: [base]\nformat: 1\n": pinFile + ":1: the anchor &n lets another place stand for this value; remove it",
		"- rulebook\n- source\n":                                                  pinFile + ":1: is not a mapping: a pin holds rulebook, source, parts and format",
	} {
		refused(t, doc, want)
	}
}

// FuzzPin holds the reader never to panic, and to answer either a refusal
// with no pin or a pin whose every key is set and whose source is a clean
// folder of the repository.
func FuzzPin(f *testing.F) {
	for _, name := range []string{"clean.yaml", "fixture.yaml"} {
		data, err := os.ReadFile(filepath.Join("testdata", "pin", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	for _, doc := range []string{cleanPin, with("source", ".."), with("format", "2"), with("parts", "[]"), "", "- a\n", "format: 1\n"} {
		f.Add([]byte(doc))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		pin, err := project.ParsePin(pinFile, data)
		if err != nil {
			var pe *project.PinError
			var nr *project.PinNotReady
			if !errors.As(err, &pe) && !errors.As(err, &nr) || !reflect.DeepEqual(pin, model.Pin{}) {
				t.Fatalf("ParsePin of %q = %+v, %v; a refusal is a PinError or PinNotReady with no pin", data, pin, err)
			}
			return
		}
		if pin.Rulebook == "" || len(pin.Parts) == 0 || pin.Format != project.PinFormat || !fs.ValidPath(pin.Source) || strings.Contains(pin.Source, ":") {
			t.Fatalf("ParsePin of %q = %+v with no error; want every key set and a clean source", data, pin)
		}
	})
}
