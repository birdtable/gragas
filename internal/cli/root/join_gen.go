// Written by gragas-dev gen commands from internal/cli/cmd, which task gen runs; edit no line of it.

package root

import (
	attentioncmd "github.com/birdtable/gragas/internal/cli/cmd/attention"
	bindcmd "github.com/birdtable/gragas/internal/cli/cmd/bind"
	cachecmd "github.com/birdtable/gragas/internal/cli/cmd/cache"
	checkcmd "github.com/birdtable/gragas/internal/cli/cmd/check"
	conflictscmd "github.com/birdtable/gragas/internal/cli/cmd/conflicts"
	contractcmd "github.com/birdtable/gragas/internal/cli/cmd/contract"
	coveragecmd "github.com/birdtable/gragas/internal/cli/cmd/coverage"
	decisioncmd "github.com/birdtable/gragas/internal/cli/cmd/decision"
	explaincmd "github.com/birdtable/gragas/internal/cli/cmd/explain"
	guardcmd "github.com/birdtable/gragas/internal/cli/cmd/guard"
	historycmd "github.com/birdtable/gragas/internal/cli/cmd/history"
	importercmd "github.com/birdtable/gragas/internal/cli/cmd/importer"
	journalcmd "github.com/birdtable/gragas/internal/cli/cmd/journal"
	migratecmd "github.com/birdtable/gragas/internal/cli/cmd/migrate"
	plugincmd "github.com/birdtable/gragas/internal/cli/cmd/plugin"
	presetcmd "github.com/birdtable/gragas/internal/cli/cmd/preset"
	rebindcmd "github.com/birdtable/gragas/internal/cli/cmd/rebind"
	resolvecmd "github.com/birdtable/gragas/internal/cli/cmd/resolve"
	sourcecmd "github.com/birdtable/gragas/internal/cli/cmd/source"
	toolscmd "github.com/birdtable/gragas/internal/cli/cmd/tools"
	unbindcmd "github.com/birdtable/gragas/internal/cli/cmd/unbind"
	undocmd "github.com/birdtable/gragas/internal/cli/cmd/undo"
	validatecmd "github.com/birdtable/gragas/internal/cli/cmd/validate"
	versioncmd "github.com/birdtable/gragas/internal/cli/cmd/version"
)

// built are the command packages below internal/cli/cmd, by name: the row each
// declares and the cobra command it makes.
func built() []builtCommand {
	return []builtCommand{
		{Row: attentioncmd.Row(), New: attentioncmd.New},
		{Row: bindcmd.Row(), New: bindcmd.New},
		{Row: cachecmd.Row(), New: cachecmd.New},
		{Row: checkcmd.Row(), New: checkcmd.New},
		{Row: conflictscmd.Row(), New: conflictscmd.New},
		{Row: contractcmd.Row(), New: contractcmd.New},
		{Row: coveragecmd.Row(), New: coveragecmd.New},
		{Row: decisioncmd.Row(), New: decisioncmd.New},
		{Row: explaincmd.Row(), New: explaincmd.New},
		{Row: guardcmd.Row(), New: guardcmd.New},
		{Row: historycmd.Row(), New: historycmd.New},
		{Row: importercmd.Row(), New: importercmd.New},
		{Row: journalcmd.Row(), New: journalcmd.New},
		{Row: migratecmd.Row(), New: migratecmd.New},
		{Row: plugincmd.Row(), New: plugincmd.New},
		{Row: presetcmd.Row(), New: presetcmd.New},
		{Row: rebindcmd.Row(), New: rebindcmd.New},
		{Row: resolvecmd.Row(), New: resolvecmd.New},
		{Row: sourcecmd.Row(), New: sourcecmd.New},
		{Row: toolscmd.Row(), New: toolscmd.New},
		{Row: unbindcmd.Row(), New: unbindcmd.New},
		{Row: undocmd.Row(), New: undocmd.New},
		{Row: validatecmd.Row(), New: validatecmd.New},
		{Row: versioncmd.Row(), New: versioncmd.New},
	}
}
