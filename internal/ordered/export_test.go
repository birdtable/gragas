package ordered

// ParseYAMLSpent is ParseYAML, with how much of its alias budget the
// converter spent and what the budget was: the work the parse did inside
// aliases, which a test holds instead of the time it took. It builds
// the converter as ParseYAML does, and the test holds its error to
// ParseYAML's, so the two cannot part.
func ParseYAMLSpent(data []byte) (v any, spent, budget int, err error) {
	n, err := decodeOne(data)
	if n == nil || err != nil {
		return nil, 0, 0, err
	}
	c := converter{budget: max(aliasFloor, aliasBudget*len(data))}
	c.left = c.budget
	v, err = c.convert(n, 0)
	return v, c.budget - c.left, c.budget, err
}
