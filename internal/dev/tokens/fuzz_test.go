package tokens_test

import (
	"os"
	"testing"

	"github.com/birdtable/gragas/internal/dev/tokens"
)

// FuzzParseTokens: any bytes either fail to parse or parse to a document that
// renders its three files and reads its pairs without a panic, and a document
// it takes has a value in both modes of every colour (web.tokens.11).
func FuzzParseTokens(f *testing.F) {
	if b, err := os.ReadFile("../../../" + tokens.File); err == nil {
		f.Add(b)
	}
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"grounds":{"page":{"light":"#ffffff","dark":"#000000"}}}`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := tokens.ParseBytes(b)
		if err != nil {
			return
		}
		_ = d.Low()
		_ = d.CSS()
		_ = d.Theme()
		if _, err := d.Palette(); err != nil {
			t.Fatalf("a document Parse took has no palette: %v", err)
		}
		for _, c := range d.Text {
			if c.Light == "" || c.Dark == "" {
				t.Fatalf("a document Parse took holds a colour with no value in a mode: %+v", c)
			}
		}
	})
}
