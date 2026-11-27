package plugins_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// A config that reaches beyond its one file. The engine
// holds a raw control character, which no format it reads allows and a tool
// may read as another key; the plugin holds what its tool's reader takes for
// extends, plugins, an import or an escape, and refuses it at plan, which the
// read holds to nothing run.

// texted is a config at path whose data is text.
func texted(path, text string) plugin.Config {
	return plugin.Config{Path: path, Data: &plugin.Bytes{Text: &text}}
}

// noStep is a runner that records what it is asked, and is asked nothing by a
// read that ends before its first step.
type noStep struct{ asked []string }

func (r *noStep) Granted(plugin.Step) error {
	r.asked = append(r.asked, "granted")
	return nil
}

func (r *noStep) Run(context.Context, int, int, plugin.Step) (plugin.Result, error) {
	r.asked = append(r.asked, "run")
	return plugin.Result{}, nil
}

// counted is the stand-in plugin with each call it takes counted.
func counted(calls map[string]int) *porttest.StandinPlugin {
	p := porttest.NewStandinPlugin()
	plan, entries, findings := p.OnPlan, p.OnEntries, p.OnFindings
	p.OnPlan = func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
		calls["plan"]++
		return plan(in)
	}
	p.OnEntries = func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
		calls["entries"]++
		return entries(in)
	}
	p.OnFindings = func(in plugin.FindingsRequest) (plugin.FindingsResponse, error) {
		calls["findings"]++
		return findings(in)
	}
	return p
}

// A config whose text holds a control character ends the read Failed before
// the plugin is asked anything, in the entries read and the findings read
// alike, and the cause names the config and the character.
func TestAConfigWithAControlCharacterIsHandedToNoPlugin(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		text string
		code string
	}{
		{"escape", "a\n\x1bxtends ../b\n", "U+001B"},
		{"nul", "a\x00\n", "U+0000"},
		{"delete", "a\x7f\n", "U+007F"},
		{"next line", "a\u0085extends\n", "U+0085"},
		{"control sequence introducer", "a\u009b\n", "U+009B"},
		{"unit separator", "a\x1fb\n", "U+001F"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := seed()
			in.Config = []plugin.Config{texted("clean.conf", "a\n"), texted("dirty.conf", c.text)}
			for read, call := range map[string]func(context.Context, port.Plugin, *noStep) error{
				"entries": func(ctx context.Context, p port.Plugin, r *noStep) error {
					_, err := plugins.Entries(ctx, p, r, in)
					return err
				},
				"findings": func(ctx context.Context, p port.Plugin, r *noStep) error {
					_, err := plugins.Findings(ctx, p, r, in)
					return err
				},
			} {
				calls := map[string]int{}
				r := &noStep{}
				err := call(t.Context(), open(t, counted(calls)), r)
				e := ends(t, err, plugins.Failed)
				if e.Round != 0 {
					t.Fatalf("%s: it ended at round %d", read, e.Round)
				}
				if len(calls) != 0 || len(r.asked) != 0 {
					t.Fatalf("%s: the plugin was asked %v and the runner %v", read, calls, r.asked)
				}
				for _, want := range []string{"/config/1/data", c.code} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("%s: %q does not say %s", read, err, want)
					}
				}
			}
		})
	}
}

// Tab, line feed and carriage return are text, and a config that holds them
// is handed over; so is a config with no data, and one whose data is not text,
// which is the plugin's to read or refuse.
func TestTabsAndLineEndsAreHandedOver(t *testing.T) {
	t.Parallel()
	in := seed()
	in.Config = []plugin.Config{texted("standin.conf", "a\twarning\r\nb\n")}
	calls := map[string]int{}
	if _, err := plugins.Entries(t.Context(), open(t, counted(calls)), &noStep{}, in); err != nil {
		t.Fatalf("a config with tabs and line ends: %v", err)
	}
	if calls["plan"] != 1 || calls["entries"] != 1 {
		t.Fatalf("the plugin was asked %v", calls)
	}
	raw := "AA=="
	in.Config = []plugin.Config{{Path: "standin.conf"}, {Path: "b.bin", Data: &plugin.Bytes{Base64: &raw}}}
	calls = map[string]int{}
	// The stand-in reads no bytes that are not text, so its reading call
	// fails; what counts here is that the plan was asked.
	_, err := plugins.Entries(t.Context(), open(t, counted(calls)), &noStep{}, in)
	if calls["plan"] != 1 || calls["entries"] != 1 {
		t.Fatalf("a config with no text was not handed over: asked %v, %v", calls, err)
	}
}

// End to end over the stand-in: a config the plugin finds reaching beyond its
// file is refused at plan, with the plugin's path into that config's data;
// the read ends Refused, nothing is granted or run, and the reading call is
// never made.
func TestAReachingConfigIsRefusedAndNothingRuns(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"a\nextends ../base.conf\n",
		"plugins ./local\n",
		"-import\n",
		"\\extends error\n",
		"a\\u0065xtends\n",
	} {
		t.Run(fmt.Sprintf("%q", text), func(t *testing.T) {
			t.Parallel()
			in := seed()
			in.Config = []plugin.Config{texted("standin.conf", text)}
			calls := map[string]int{}
			r := &noStep{}
			got, err := plugins.Entries(t.Context(), open(t, counted(calls)), r, in)
			e := ends(t, err, plugins.Refused)
			ce, ok := errors.AsType[*plugin.CallError](err)
			if !ok || ce.Code != plugin.CodeRefused || ce.Path != "/config/0/data" {
				t.Fatalf("the plugin's refusal is lost: %v", err)
			}
			if e.Round != 0 || e.Step != -1 {
				t.Fatalf("it ended at round %d, step %d", e.Round, e.Step)
			}
			if calls["plan"] != 1 || calls["entries"] != 0 || len(r.asked) != 0 || len(got.Entries) != 0 {
				t.Fatalf("the plugin was asked %v, the runner %v, and it gave %v", calls, r.asked, got.Entries)
			}
		})
	}
}
