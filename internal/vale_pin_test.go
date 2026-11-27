package internal

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// TestTheGateRunsThePinnedVale (gragas#1119): the vale task tools places in
// .tools/bin is the program tools.yaml pins, held by the sha256 of its program
// as the preset test of docs.voice.2 (gragas#1097) will hold it, and its
// version run says the pin's line. The plant: a copy of tools.yaml whose
// every vale sum is moved holds the same program to none of them, so a sum
// that is not the program's is red. A member names it as {{.TOOLS}}/vale and
// no other way (TestAPlantedProgramInAMemberIsRed).
func TestTheGateRunsThePinnedVale(t *testing.T) {
	bin, says := pinnedProgram(t, "vale")
	saysItsPin(t, bin, says, exec.CommandContext(t.Context(), "vale", "--version"))

	text := read(t, "tools.yaml")
	_, sums, err := toolPin(lintDoc(t, "tools.yaml"), "vale")
	if err != nil || len(sums) != 5 {
		t.Fatalf("tools.yaml pins vale's program on %d platforms, %v; want darwin and linux on amd64 and arm64, and windows/amd64", len(sums), err)
	}
	for _, sum := range sums {
		text = strings.Replace(text, "sha256: "+sum, "sha256: "+moved(sum), 1)
	}
	v, err := ordered.ParseYAML([]byte(text))
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		t.Fatalf("the planted copy of tools.yaml does not read: %v", err)
	}
	_, planted, err := toolPin(doc, "vale")
	if err != nil {
		t.Fatal(err)
	}
	if err := sumPinned(bin, planted); err == nil || !regexp.MustCompile(`has sha256 [0-9a-f]{64}, which tools.yaml pins on no platform`).MatchString(err.Error()) {
		t.Errorf("the placed vale against a copy of tools.yaml with every vale sum moved: %v, want it red", err)
	}
}

// moved is sum with its first hex digit changed, so it is a sum of the same
// shape that no program has.
func moved(sum string) string {
	if sum[0] == '0' {
		return "1" + sum[1:]
	}
	return "0" + sum[1:]
}
