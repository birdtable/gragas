package wcag_test

import (
	"errors"
	"math"
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/wcag"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestTheContrastIsTheStandards: white on black is 21:1, a colour on itself
// 1:1, either way round the same, and a known pair reads as the standard's
// worked value (web.tokens.8).
func TestTheContrastIsTheStandards(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		a, b string
		want float64
	}{
		{"white on black is the widest", "#ffffff", "#000000", 21},
		{"black on white is the same", "#000000", "#ffffff", 21},
		{"a colour on itself is one", "#777777", "#777777", 1},
		{"the standard's worked grey on white", "#767676", "#ffffff", 4.54},
	} {
		got, err := wcag.Contrast(c.a, c.b)
		if err != nil || math.Abs(got-c.want) > 0.01 {
			t.Errorf("%s: Contrast(%s, %s) = %v, %v, want %v", c.name, c.a, c.b, got, err, c.want)
		}
	}
}

// TestAColourThatIsNoHexIsRefused: a name, a short form, a missing hash and a
// letter that is no digit each fail with ErrColour, so a typo in the document
// is never read as a ratio.
func TestAColourThatIsNoHexIsRefused(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"red", "#fff", "ffffff0", "#gggggg", "", "#ffffffff"} {
		if _, err := wcag.Luminance(bad); !errors.Is(err, wcag.ErrColour) {
			t.Errorf("Luminance(%q) = %v, want ErrColour", bad, err)
		}
		if _, err := wcag.Contrast("#ffffff", bad); !errors.Is(err, wcag.ErrColour) {
			t.Errorf("Contrast(white, %q) = %v, want ErrColour", bad, err)
		}
	}
}
