package audit

import (
	"context"
	"fmt"

	"github.com/birdtable/gragas/internal/ordered"
)

// readTaskNames holds a task runner's file to the bindings of its tool: the
// key is a task that must exist.
func readTaskNames(_ context.Context, _ Input, c config) (verdict, string) {
	var tasks *ordered.Map
	if c.Present {
		v, err := ordered.ParseStrict(c.Data)
		doc, ok := v.(*ordered.Map)
		if err != nil || !ok {
			return nil, fmt.Sprintf("%s cannot be read as a mapping: %v", c.File, err)
		}
		tasks = doc.Sub("tasks")
	}
	return func(b Binding) string {
		switch {
		case !c.Present:
			return "there is no " + c.File + ", so no task " + b.Key
		case tasks == nil:
			return c.File + " has no tasks, so no task " + b.Key
		}
		if _, ok := tasks.Get(b.Key); !ok {
			return "no task " + b.Key
		}
		return ""
	}, ""
}
