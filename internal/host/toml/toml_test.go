package toml_test

import (
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/toml"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func TestReadGivesPlainMapsAndLists(t *testing.T) {
	t.Parallel()
	doc, err := toml.Reader{}.Read([]byte("[extend]\nuseDefault = true\n[allowlist]\npaths = ['vendor/']\n"))
	if err != nil {
		t.Fatal(err)
	}
	if ext, _ := doc["extend"].(map[string]any); ext["useDefault"] != true {
		t.Errorf("extend = %v", doc["extend"])
	}
	if al, _ := doc["allowlist"].(map[string]any); len(al["paths"].([]any)) != 1 {
		t.Errorf("allowlist = %v", doc["allowlist"])
	}
}

func TestReadRefusesBadTOMLAndARepeatedKey(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]string{"syntax": "a = = 1", "repeat": "a = 1\na = 2\n"} {
		if _, err := (toml.Reader{}).Read([]byte(in)); err == nil {
			t.Errorf("%s: Read(%q) = nil error, want one", name, in)
		}
	}
}
