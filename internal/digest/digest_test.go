package digest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/digest"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// known is the known answers of FIPS 180-2 (appendix B) and the empty input:
// the sites that moved here keep their bytes only if these hold.
func known() []struct{ name, in, hex string } {
	return []struct{ name, in, hex string }{
		{"the empty input", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"one block", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"two blocks", "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"},
	}
}

func TestEachFormIsTheKnownSum(t *testing.T) {
	t.Parallel()
	for _, k := range known() {
		b := []byte(k.in)
		if got := digest.Hex(b); got != k.hex {
			t.Errorf("Hex(%q) = %s, want %s", k.in, got, k.hex)
		}
		if got := digest.Prefixed(b); got != "sha256:"+k.hex {
			t.Errorf("Prefixed(%q) = %s, want sha256:%s", k.in, got, k.hex)
		}
		s := digest.Sum(b)
		if got := hex.EncodeToString(s[:]); got != k.hex {
			t.Errorf("Sum(%q) = %s, want %s", k.in, got, k.hex)
		}
		h := digest.New()
		for _, c := range []byte(k.in) {
			h.Write([]byte{c})
		}
		if got := digest.HexOf(h); got != k.hex {
			t.Errorf("HexOf(New, byte by byte, %q) = %s, want %s", k.in, got, k.hex)
		}
		if got := digest.PrefixedOf(h); got != "sha256:"+k.hex {
			t.Errorf("PrefixedOf(%q) = %s, want sha256:%s", k.in, got, k.hex)
		}
		got, n, err := digest.Reader(iotest.OneByteReader(strings.NewReader(k.in)))
		if err != nil || got != k.hex || n != int64(len(k.in)) {
			t.Errorf("Reader(%q) = %s, %d, %v, want %s, %d, nil", k.in, got, n, err, k.hex, len(k.in))
		}
	}
}

// TestEachFormIsTheStandardLibrarys holds every form to crypto/sha256 on
// inputs of every length up to two blocks and a byte, so a form that drops
// or pads a byte is caught where the known answers would not reach.
func TestEachFormIsTheStandardLibrarys(t *testing.T) {
	t.Parallel()
	b := make([]byte, 2*sha256.BlockSize+1)
	for i := range b {
		b[i] = byte(i*7 + 3)
	}
	for n := range len(b) + 1 {
		in := b[:n]
		want := sha256.Sum256(in)
		if got := digest.Sum(in); got != want {
			t.Fatalf("Sum of %d bytes = %x, want %x", n, got, want)
		}
		if got := digest.Hex(in); got != hex.EncodeToString(want[:]) {
			t.Fatalf("Hex of %d bytes = %s, want %x", n, got, want)
		}
		h := digest.New()
		h.Write(in)
		if got := digest.HexOf(h); got != hex.EncodeToString(want[:]) {
			t.Fatalf("HexOf of %d bytes = %s, want %x", n, got, want)
		}
	}
	if digest.Size != sha256.Size || digest.Size != len(digest.Sum(nil)) {
		t.Errorf("Size = %d, want %d", digest.Size, sha256.Size)
	}
}

// TestHexOfLeavesTheHashToWriteOn: a stream summed part way goes on.
func TestHexOfLeavesTheHashToWriteOn(t *testing.T) {
	t.Parallel()
	h := digest.New()
	h.Write([]byte("ab"))
	_ = digest.HexOf(h)
	h.Write([]byte("c"))
	if got := digest.HexOf(h); got != known()[1].hex {
		t.Errorf("HexOf after more = %s, want %s", got, known()[1].hex)
	}
}

// TestAReadThatFailsIsNoSum: a failed read gives the error and no sum.
func TestAReadThatFailsIsNoSum(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	r := io.MultiReader(strings.NewReader("abc"), iotest.ErrReader(boom))
	got, n, err := digest.Reader(r)
	if !errors.Is(err, boom) || got != "" || n != 3 {
		t.Errorf("Reader over a failing read = %q, %d, %v, want \"\", 3, boom", got, n, err)
	}
}
