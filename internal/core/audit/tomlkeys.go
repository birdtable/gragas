package audit

import (
	"context"
	"fmt"
)

// readTOMLKeys holds a TOML config to its bindings. Two keys are known:
// extend.useDefault (the config extends the tool's default rules,
// supply.secrets.1) and allowlist.paths (the config has no default path
// allowlist, supply.secrets.4's config half). Any other key is a finding that
// the reader does not know it, so a binding is never passed over in silence.
// The files the config extends by [extend] path are read with it (extend.go),
// since what they allow is what the config allows.
func readTOMLKeys(_ context.Context, in Input, c config) (verdict, string) {
	var chain []tomlFile
	if c.Present {
		if in.TOML == nil {
			return nil, "no TOML reader was given"
		}
		doc, why := parseTOMLFile(in, c.File, c.Data)
		if why != "" {
			return nil, why
		}
		top := tomlFile{c.File, doc}
		more, why := extendedFiles(in, top)
		if why != "" {
			return nil, why
		}
		chain = append([]tomlFile{top}, more...)
	}
	return func(b Binding) string { return judgeTOMLKeys(chain, c, b) }, ""
}

// judgeTOMLKeys is the finding, "" for none, of one binding over the chain of
// documents: the config and the files it extends.
func judgeTOMLKeys(chain []tomlFile, c config, b Binding) string {
	if !c.Present {
		return "there is no " + c.File
	}
	switch b.Key {
	case "extend.useDefault":
		// gitleaks reads [extend] of the config and of the file it extends, and
		// of no file below those (config.go: maxExtendDepth = 2).
		for _, f := range chain[:min(len(chain), 2)] {
			if ext, ok := f.doc["extend"].(map[string]any); ok && ext["usedefault"] == true {
				return ""
			}
		}
		return "the config does not extend the default rules (extend.useDefault = true)"
	case "allowlist.paths":
		for _, f := range chain {
			if hasPaths(f.doc) {
				return "the config allows paths for every rule: remove allowlist.paths, and scope a path to a rule (" + f.name + ")"
			}
		}
	default:
		return "gragas does not read the key " + b.Key + " of " + c.File
	}
	return ""
}

// hasPaths is whether the config's own allowlist, or any allowlist in
// [[allowlists]] that names no target rule, lists paths.
func hasPaths(doc map[string]any) bool {
	if al, ok := doc["allowlist"].(map[string]any); ok && nonEmpty(al["paths"]) {
		return true
	}
	lists, ok := doc["allowlists"].([]any)
	if !ok {
		return false
	}
	for _, l := range lists {
		if m, ok := l.(map[string]any); ok && nonEmpty(m["paths"]) && !nonEmpty(m["targetrules"]) {
			return true
		}
	}
	return false
}

func nonEmpty(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) > 0
}

// allowlistShape is the reason the allowlists of a document are not read: the
// tool takes one value where it expects a list, and one table where it expects
// a list of tables, so a value of another shape is not guessed at.
func allowlistShape(doc map[string]any) error {
	if v, ok := doc["allowlist"]; ok {
		t, isTable := v.(map[string]any)
		if !isTable {
			return fmt.Errorf("the key allowlist is not a table, and the tool's read of it is not known")
		}
		if err := listsOfStrings("allowlist", t); err != nil {
			return err
		}
	}
	v, ok := doc["allowlists"]
	if !ok {
		return nil
	}
	l, isList := v.([]any)
	if !isList {
		return fmt.Errorf("the key allowlists is not a list of tables, and the tool's read of it is not known")
	}
	for _, e := range l {
		t, isTable := e.(map[string]any)
		if !isTable {
			return fmt.Errorf("the key allowlists is not a list of tables, and the tool's read of it is not known")
		}
		if err := listsOfStrings("allowlists", t); err != nil {
			return err
		}
	}
	return nil
}

// listsOfStrings is the error for a paths or a targetrules of a table that is
// there and is no list of strings.
func listsOfStrings(of string, t map[string]any) error {
	for _, k := range []string{"paths", "targetrules"} {
		v, ok := t[k]
		if !ok {
			continue
		}
		l, isList := v.([]any)
		if !isList {
			return fmt.Errorf("the key %s.%s is not a list of strings, and the tool's read of it is not known", of, k)
		}
		for _, e := range l {
			if _, isString := e.(string); !isString {
				return fmt.Errorf("the key %s.%s is not a list of strings, and the tool's read of it is not known", of, k)
			}
		}
	}
	return nil
}
