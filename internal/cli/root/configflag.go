package root

// --config names the settings file, and the settings decide how the run
// looks before the tree parses its flags (the banner, the colour), so main
// reads it first, through ConfigFile, and the tree parses it again with the
// rest.

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/birdtable/gragas/internal/config"
)

// ConfigFile is the settings file --config names in args, as an absolute
// path, or "" when it names none. The tree's own parser reads the line, so a
// flag's value is never taken for the flag, and nothing after -- is read. A
// line the tree cannot parse names none here, and the run then ends on the
// usage error. A relative path is from the working folder, since a person
// typed it there.
func ConfigFile(ctx context.Context, args []string) string {
	tree := (&session{}).newRoot(ctx)
	c, rest, err := tree.Find(args)
	if err != nil || c == nil {
		c, rest = tree, args
	}
	if c.ParseFlags(rest) != nil {
		return ""
	}
	file, err := c.Flags().GetString(strings.TrimPrefix(config.ConfigFlag, "--"))
	if err != nil || file == "" {
		return ""
	}
	if abs, err := filepath.Abs(file); err == nil {
		return abs
	}
	return file
}
