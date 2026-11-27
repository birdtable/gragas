package model

// Choice is one row of a menu: what it is called, what it does and the value
// picking it answers. Gap puts a blank line under the row, to separate one
// group of rows from the next.
type Choice struct {
	Label, Blurb, Value string
	Gap                 bool
}
