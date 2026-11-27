package gate

import "slices"

// closure holds the shipped binary's linked modules to the list: a module
// linked and not listed fails, and a listed module the binary no longer links
// fails until it is taken off, so the list only falls (supply.intake.2).
func (r *Registry) closure() {
	list := r.text[ClosureList]
	if len(list) == 0 || r.in.Closure == nil {
		if r.in.Closure == nil {
			r.add(ClosureList, "the shipped binary's linked modules were not read, so nothing was checked")
		}
		return
	}
	listed := make([]string, len(list))
	for i, e := range list {
		listed[i] = e.Value
	}
	for _, m := range r.in.Closure {
		if !slices.Contains(listed, m) {
			r.add(ClosureList, "the shipped binary links %s, which the declared closure does not list", m)
		}
	}
	for _, e := range list {
		if !slices.Contains(r.in.Closure, e.Value) {
			r.add(ClosureList, "line %d: %s is listed and no longer linked: take it off", e.Line, e.Value)
		}
	}
}
