package audit

import (
	"context"
	"errors"
	"strings"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// readEntries holds a repository's config to the bindings of its tool by the
// entries the tool's plugin reads from it: an entry at
// a binding's key that is on holds the binding, one that is off is a finding,
// and no entry is one. What the config means, what is run to learn it and what
// the plugin refuses to run are the plugin's: the check sends the file as it
// found it, or none, and judges the answer. A file that is not text is not
// sent, and the rules are not checked.
//
// A run the host refuses because it can neither hold nor read the run's
// memory (port.ErrNotHeld) is no "not checked": the rules could be checked on
// a host that holds the run, so the check halts red on it. Any other error of
// the call is the reason the tool's rules are not checked, in the call's
// words.
func readEntries(ctx context.Context, in Input, c config) (verdict, string, error) {
	if in.Entries == nil {
		return nil, "no plugin of " + c.Tool + " is there to read its config", nil
	}
	call, version, why := in.Entries(c.Tool)
	if why != "" {
		return nil, why, nil
	}
	req := plugin.PlanRequest{
		ToolVersion: version, Config: []plugin.Config{}, Have: []plugin.Result{},
		For: plugin.ForConfig, Target: plugin.TargetSource,
	}
	if c.Present {
		text := string(c.Data)
		if strings.ToValidUTF8(text, "") != text {
			return nil, c.File + " is not UTF-8 text, so it is not given to the plugin", nil
		}
		req.Config = []plugin.Config{{Path: c.File, Data: &plugin.Bytes{Text: &text}}}
	}
	got, err := call(ctx, req)
	switch {
	case errors.Is(err, port.ErrNotHeld):
		return nil, "", err
	case err != nil:
		return nil, err.Error(), nil
	}
	on := make(map[string]bool, len(got.Entries))
	for _, e := range got.Entries {
		on[e.Key] = e.On
	}
	return func(b Binding) string { return judgeEntry(on, c, b) }, "", nil
}

// judgeEntry is the finding, "" for none, of one binding: an entry at its key
// that is on holds it.
func judgeEntry(on map[string]bool, c config, b Binding) string {
	enabled, held := on[b.Key]
	switch {
	case held && enabled:
		return ""
	case held:
		return b.Key + " is off"
	case !c.Present:
		return "there is no " + c.File + ", so " + b.Key + " is not set"
	}
	return c.File + " has no entry " + b.Key
}
