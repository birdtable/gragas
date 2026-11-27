package check

import "github.com/birdtable/gragas/internal/model"

// NextRow is a row of the table of Next lines, as the tests read it.
type NextRow struct {
	Cause     model.GateCause
	Next, Why string
}

// NextTable is the table the Next line of a not-ready project check is read
// from, in its order.
func NextTable() []NextRow {
	rows := nextOrder()
	out := make([]NextRow, len(rows))
	for i, r := range rows {
		out[i] = NextRow{r.cause, r.next, r.why}
	}
	return out
}
