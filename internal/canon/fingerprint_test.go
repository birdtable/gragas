package canon

import (
	"encoding/base64"
	"errors"
	"testing"
)

// key is a real ed25519 public key made for this test; want is what
// ssh-keygen -lf prints for it.
const (
	key  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKDssIWdyTBS3S8/ejqyEdsXRMyfjDRiHFrhEpPxi+7z test@rulebook"
	want = "SHA256:sm4tmRoNJYEI9tUAwd0QZzgxtox92zY4EkZUdxdCiyw"
)

func TestFingerprintOfALiteralKey(t *testing.T) {
	t.Parallel()
	got, err := Fingerprint(key)
	if err != nil || got != want {
		t.Fatalf("Fingerprint = %q, %v; want %q", got, err, want)
	}
	other, err := Fingerprint("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKDssIWdyTBS3S8/ejqyEdsXRMyfjDRiHFrhEpPxi+7z renamed")
	if err != nil || other != want {
		t.Fatalf("the comment changed the fingerprint: %q, %v", other, err)
	}
}

func TestFingerprintRefusesWhatIsNotAKey(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]string{
		"empty":         "",
		"no base64":     "ssh-ed25519",
		"not base64":    "ssh-ed25519 !!!!",
		"wrong type":    "ssh-rsa AAAAC3NzaC1lZDI1NTE5AAAAIKDssIWdyTBS3S8/ejqyEdsXRMyfjDRiHFrhEpPxi+7z",
		"two lines":     key + "\n" + key,
		"short payload": "ssh-ed25519 AAAA",
	} {
		if _, err := Fingerprint(in); !errors.Is(err, ErrKey) {
			t.Errorf("%s: err = %v; want ErrKey", name, err)
		}
	}
}

// TestFingerprintRefusesALengthFieldPastThePayload: the key's first field says
// how long the type name is, read as an unsigned 32-bit number, so a length of
// 0xFFFFFFFF is refused, not read as a negative one or past the end. Planted
// failure: the length read as a signed int32, or the bound dropped.
func TestFingerprintRefusesALengthFieldPastThePayload(t *testing.T) {
	t.Parallel()
	blob := base64.StdEncoding.EncodeToString([]byte{0xFF, 0xFF, 0xFF, 0xFF, 's', 's', 'h'})
	if _, err := Fingerprint("ssh-ed25519 " + blob); !errors.Is(err, ErrKey) {
		t.Errorf("a length of 0xFFFFFFFF: err = %v; want ErrKey", err)
	}
}
