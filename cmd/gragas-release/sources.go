package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/ordered"
)

// configName is goreleaser's config, at the root of the clone it builds from.
const configName = ".goreleaser.yaml"

// linkMode and plainMode are the modes git ls-tree prints for a symbolic link
// and for a plain file that is not executable.
const (
	linkMode  = "120000"
	plainMode = "100644"
)

// sourceKeys are the keys under which goreleaser's config lists files it reads
// from the tree and puts in what it builds: an archive's files, a checksum's or
// a release's extra files, a package's contents and their templated forms.
func sourceKeys() []string {
	return []string{"files", "extra_files", "templated_files", "templated_extra_files", "contents"}
}

// sourcesStep is the step before goreleaser runs (gragas#1206). goreleaser
// follows a files: source that is a symbolic link and writes the bytes of what
// it points to as a plain file, so the unpack step, which admits only plain
// files and folders, never sees the link, and a link to a file outside the tree
// would ship that file (supply.release.6). It reads every source the clone's
// .goreleaser.yaml names and holds each to be a plain file, mode 100644, in the
// commit the clone has checked out. The list is read from the config, never kept
// by hand, and a source the config spells in a way this step cannot read is red.
func sourcesStep(ctx context.Context, tc toolchain, repo string, out *say.Speaker) error {
	config, err := os.ReadFile(filepath.Join(repo, configName))
	if err != nil {
		return err
	}
	sources, err := releaseSources(config)
	if err != nil {
		return err
	}
	tree, err := tc.Tree(ctx, repo)
	if err != nil {
		return err
	}
	modes, err := treeModes(tree)
	if err != nil {
		return err
	}
	var bad []string
	for _, src := range sources {
		switch mode, ok := modes[src]; {
		case !ok:
			bad = append(bad, src+": a source of "+configName+" that is not a file tracked in HEAD")
		case mode == linkMode:
			bad = append(bad, src+": a source of "+configName+" that is a symbolic link in HEAD; goreleaser would ship what it points to")
		case mode != plainMode:
			bad = append(bad, fmt.Sprintf("%s: a source of %s that is mode %s in HEAD, not a plain file (%s)", src, configName, mode, plainMode))
		}
	}
	if len(bad) != 0 {
		return errors.New(strings.Join(bad, "; "))
	}
	out.Text(fmt.Sprintf("sources: the %d sources of %s are plain files in HEAD: %s\n", len(sources), configName, strings.Join(sources, ", ")))
	return nil
}

// releaseSources reads every source goreleaser's config names under one of the
// sourceKeys, at any depth, in the order written: a list item that is a path,
// or a mapping whose src is one. A source that is a pattern or a template, an
// item of another shape, a key of sourceKeys that is not a list and a config
// that names no source at all are errors, so no source goes unread.
func releaseSources(config []byte) ([]string, error) {
	doc, err := ordered.ParseYAML(config)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configName, err)
	}
	var sources, bad []string
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, item := range v {
				walk(item)
			}
		case *ordered.Map:
			for _, k := range v.Keys() {
				if slices.Contains(sourceKeys(), k) {
					got, why := sourcesUnder(v, k)
					sources, bad = append(sources, got...), append(bad, why...)
					continue
				}
				walk(v.Val(k))
			}
		}
	}
	walk(doc)
	if len(bad) != 0 {
		return nil, fmt.Errorf("%s: %s", configName, strings.Join(bad, "; "))
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("%s names no source under %s", configName, strings.Join(sourceKeys(), ", "))
	}
	return sources, nil
}

// sourcesUnder reads the list under the key k of m, and says what of it is not
// a source this step can hold.
func sourcesUnder(m *ordered.Map, k string) (sources, bad []string) {
	at := func(line int) string { return fmt.Sprintf("line %d: %s:", line, k) }
	list, ok := m.Val(k).([]any)
	if !ok {
		return nil, []string{at(m.KeyLine(k)) + " is not a list"}
	}
	for i, item := range list {
		src, line := "", m.KeyLine(k)
		switch item := item.(type) {
		case string:
			src = item
		case *ordered.Map:
			s, isString := item.Val("src").(string)
			if !isString {
				bad = append(bad, fmt.Sprintf("%s item %d has no src that is a path", at(item.Line), i+1))
				continue
			}
			src, line = s, item.KeyLine("src")
		default:
			bad = append(bad, fmt.Sprintf("%s item %d is neither a path nor a mapping with src", at(line), i+1))
			continue
		}
		if src == "" || strings.ContainsAny(src, "*?[]{}\\") {
			bad = append(bad, fmt.Sprintf("%s %q is not a path to one file: a pattern or a template", at(line), src))
			continue
		}
		sources = append(sources, src)
	}
	return sources, bad
}

// treeModes reads git ls-tree -r -z: each record is the mode, the type and the
// object, a tab and the path, ended by a NUL. It answers each path's mode.
func treeModes(listing string) (map[string]string, error) {
	modes := map[string]string{}
	for record := range strings.SplitSeq(strings.TrimSuffix(listing, "\x00"), "\x00") {
		if record == "" {
			continue
		}
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || path == "" {
			return nil, fmt.Errorf("git ls-tree printed %q, which is not a mode, a type, an object and a path", record)
		}
		modes[path] = fields[0]
	}
	return modes, nil
}
