// Package clock is the adapter of port.Clock: the one package that reads the
// system clock. Everything else asks the Clock port, so a test hands it a
// fixed instant (arch.ports.1). It also reads how busy the host is, the
// load average and the processor time of a process's children, for the tier
// budget (Busy).
package clock

import "time"

// Clock tells the time from the system clock. The zero Clock is ready.
type Clock struct{}

// Now answers the present instant in UTC.
func (Clock) Now() time.Time { return time.Now().UTC() }
