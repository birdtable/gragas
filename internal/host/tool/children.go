package tool

import (
	"time"

	"github.com/birdtable/go-proc/proc"
)

// ChildrenCPU is the processor time this process's waited-for children have
// used so far: go-proc's proc.ChildrenCPU, forwarded so that this package
// stays the one importer of go-proc/proc (arch.ports.6). It starts nothing.
// Where the system keeps no such time (Windows) the error wraps
// errors.ErrUnsupported.
func ChildrenCPU() (time.Duration, error) { return proc.ChildrenCPU() }
