package clock

import (
	"time"

	"github.com/birdtable/gragas/internal/host/tool"

	"github.com/birdtable/go-proc/load"
)

// Busy is the host's one-minute load average and the processor time this
// process's waited-for children have used so far: what a tier run reads at
// its start and end to tell a host others load from a slow tree
// (gate.Runner.Host). Either failing fails both. The load is go-proc's
// load.Read; where the system keeps no average or no children's time (Windows) the error
// matches load.ErrNoAverage or errors.ErrUnsupported, and the tier budget is
// judged on the wall time alone. The children's time is go-proc's
// host/tool's ChildrenCPU, go-proc's proc.ChildrenCPU forwarded.
func Busy() (float64, time.Duration, error) {
	r, err := load.Read()
	if err != nil {
		return 0, 0, err
	}
	cpu, err := tool.ChildrenCPU()
	if err != nil {
		return 0, 0, err
	}
	return r.One, cpu, nil
}
