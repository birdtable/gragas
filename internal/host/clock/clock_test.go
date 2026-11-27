package clock_test

import (
	"os"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/clock"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func TestTheClockAdapterPassesTheClockSuite(t *testing.T) {
	t.Parallel()
	contracttest.Clock(t, func(*testing.T) port.Clock { return clock.Clock{} }, time.Now)
}
