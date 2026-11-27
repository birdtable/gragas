package internal

import (
	"strings"
	"testing"
)

// goList is every package of the module.
func goList(t *testing.T) []string {
	t.Helper()
	out, err := goOut(nil, allArgs()...)
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}
	return strings.Fields(string(out))
}
