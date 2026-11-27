package link

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// ConfigFile is a config file of a project: its path and its bytes.
type ConfigFile struct {
	Path string
	Data []byte
}

// EntriesCall asks a plugin for the entries of a config: the plan rounds and
// the one entries call, with the plan request as the first
// plan's. The operations (internal/core/api) wire it to plugins.Entries with
// the plugin and the runner of the read, because a context imports no other
// (arch.ports.8); link says what it asks and never how it is answered.
type EntriesCall func(ctx context.Context, in plugin.PlanRequest) (plugin.EntriesResponse, error)

// ReadConfigs turns each config file into the entries it holds, by asking the
// plugin of the tool for them, one file to a call so each entry is held by the
// file it came from. Which files a tool's entries are in, and what is run to
// learn them, is the plugin's plan and never read here: the files named are
// what the plan is given. An entry the plugin says is off holds nothing, so
// nothing is imported from it; one that names no severity stands at error, so
// an import never holds a rule weaker than the config it came from said. The
// call's own error ends the read, naming the file.
func ReadConfigs(ctx context.Context, call EntriesCall, toolVersion string, target model.TargetType, files []ConfigFile) ([]model.ImportEntry, error) {
	t, err := pluginTarget(target)
	if err != nil {
		return nil, err
	}
	var out []model.ImportEntry
	for _, f := range files {
		data, err := textOf(f.Path, f.Data)
		if err != nil {
			return nil, err
		}
		got, err := call(ctx, plugin.PlanRequest{
			ToolVersion: toolVersion,
			Config:      []plugin.Config{{Path: f.Path, Data: data}},
			Target:      t,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		entries, err := importEntries(f.Path, got.Entries)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}
	return out, nil
}

// pluginTarget is the wire's name of a target type: the same word, held to the
// five the contract has.
func pluginTarget(t model.TargetType) (plugin.Target, error) {
	w := plugin.Target(t)
	if slices.Contains([]plugin.Target{plugin.TargetSource, plugin.TargetConfig, plugin.TargetMigration, plugin.TargetContainer, plugin.TargetWorkflow}, w) {
		return w, nil
	}
	return "", fmt.Errorf("the target %q is not one a plugin reads", string(t))
}

// textOf is the bytes of a config as the wire carries them. A config is text:
// bytes that are not UTF-8 are refused here, as the file a person wrote that
// no tool reads, rather than sent as a second spelling.
func textOf(path string, b []byte) (*plugin.Bytes, error) {
	s := string(b)
	if strings.ToValidUTF8(s, "") != s {
		return nil, fmt.Errorf("%s: the config is not UTF-8 text", path)
	}
	return &plugin.Bytes{Text: &s}, nil
}

// importEntries is the entries of the config at path that hold something, at
// the engine's severity.
func importEntries(path string, entries []plugin.Entry) ([]model.ImportEntry, error) {
	var out []model.ImportEntry
	for _, e := range entries {
		if !e.On {
			continue
		}
		value, err := valueText(e.Value, false)
		if err != nil {
			return nil, fmt.Errorf("%s: the value of %s: %w", path, e.Key, err)
		}
		options, err := valueText(e.Options, true)
		if err != nil {
			return nil, fmt.Errorf("%s: the options of %s: %w", path, e.Key, err)
		}
		out = append(out, model.ImportEntry{Config: path, Key: e.Key, Value: value, Options: options, Severity: severityOf(e.Severity)})
	}
	return out, nil
}

// severityOf is the engine's severity a plugin's entry stands at; none stands
// at error.
func severityOf(s *plugin.Severity) model.Severity {
	if s == nil {
		return model.SeverityError
	}
	switch *s {
	case plugin.SeverityWarning:
		return model.SeverityWarning
	case plugin.SeverityInfo:
		return model.SeverityInfo
	case plugin.SeverityError:
		return model.SeverityError
	}
	return model.SeverityError
}

// valueText is a value as model.ToolEntry holds it: "" where the plugin gave
// none, a string as printed (unless asJSON), and any other value as JSON on
// one line.
func valueText(v canon.Value, asJSON bool) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case canon.String:
		if !asJSON {
			return string(x), nil
		}
	}
	b, err := canon.Encode(v, canon.Line)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}
