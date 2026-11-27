// Package command holds the golden example of a command: the small templates
// gragas-new renders into the files of a new command, and the files they
// render for one sample name, which a test holds the templates to. Change a
// template and its golden together, or the gate step fails.
package command

import "embed"

// Files are the templates, by the path they are laid down at once rendered:
// each name is the path with {{...}} holes, in files/.
//
//go:embed files/*.tmpl
var Files embed.FS

// Golden are the files the templates render for the sample name.
//
//go:embed golden
var Golden embed.FS

// Sample is the name the golden files were rendered for.
const (
	SampleGroup = "term"
	SampleVerb  = "restore"
)
