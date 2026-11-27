package coverage

import (
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}
