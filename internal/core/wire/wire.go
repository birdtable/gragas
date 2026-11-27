// Package wire is the engine's codec of plugin contract 1: it writes a request
// in canonical form and reads a response, holding both to every rule of design
// section 4 that the two schemas cannot say.
//
// The messages are the typed values of internal/model/plugin, which
// gragas-generate writes from contract/plugin/v1/*.schema.json: their
// FromCanon refuses what the schemas say and a type can hold (a member of
// another type, a null, a member the object lacks when it is closed, a value
// outside an enum, a pattern or a bound, in UTF-8 bytes). This package holds the
// rest:
//
//   - The size of a whole request (32 MiB) and a response (16 MiB), of a
//     stream of a run (8 MiB), of stdin (1 MiB) and of a file's data (1 MiB).
//   - A value (an entry's value and options, rank's preset and found) is
//     opaque and held to 64 KiB in canonical form and 32 levels.
//   - A path is held part by part to port.FileNames.Portable, not a pattern.
//   - An argv word is literal except {scratch} and {null}; any other { is
//     refused. A glob has *, ? and ** and nothing else.
//   - The order of an answer: entries by key, findings by path, line,
//     column, key and message, files by path, each once; words as many as
//     pairs. The engine refuses an answer out of order and never sorts it.
//   - A plan with no steps and final false, an error code the operation does
//     not answer, and the mark in the first line of every preset file.
//
// A response may be any valid JSON; it is read through canon.Decode, which
// refuses what two readers would read two ways, and what Canonical writes of it
// is what the engine digests and compares.
package wire

import (
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// The caps of the contract, in UTF-8 bytes unless the name says otherwise.
// A cap the schemas say as maxItems or maxLength is the
// generated types'; these are the ones a schema cannot say.
const (
	// Contract is the number of the plugin contract the engine speaks.
	Contract = 1
	// MaxRequest is the most bytes of one request.
	MaxRequest = 32 << 20
	// MaxResponse is the most bytes of one response.
	MaxResponse = 16 << 20
	// MaxEntriesInput is the most bytes of an entries request, encoded as the
	// codec writes it, small enough that a plugin reads it well inside its
	// call's time. A call over it is refused before the plugin is reached.
	MaxEntriesInput = 64 << 10
	// MaxStream, MaxStdin, MaxFile and MaxFiles are the step's caps, which
	// host's step runner holds too, so they are the model's.
	// On an entries call MaxFile and MaxFiles do not bind: MaxEntriesInput,
	// 64 KiB of the whole request, carries the files of its reads and is
	// reached first.
	MaxStream = plugin.MaxStream
	MaxStdin  = plugin.MaxStdin
	MaxFile   = plugin.MaxFile
	MaxFiles  = plugin.MaxFiles
	// MaxValue is the most bytes of a value in canonical form.
	MaxValue = 64 << 10
	// MaxValueDepth is the most levels of arrays and objects a value nests,
	// counting its own.
	MaxValueDepth = 32
)

// MaxReadFiles is the most files one read or glob gives: the model's, as the
// step's caps are.
const MaxReadFiles = plugin.MaxReadFiles

// Features is the one list of features the engine speaks in contract 1:
// cache-vars, a grant's cache_vars. A request carries it and
// nothing else does: Open, `plugin test` and `gragas plugin contract --json`
// read this value, not a copy of it.
func Features() []string { return []string{plugin.FeatureCacheVars} }

// RequestError is a request the engine would send that breaks a rule of the
// contract: a bug of the engine's, found before the plugin is asked.
type RequestError struct {
	Path string
	Msg  string
}

// Error says where, and what is wrong there.
func (e *RequestError) Error() string {
	if e.Path == "" {
		return "plugin request: the request " + e.Msg
	}
	return "plugin request: " + e.Path + " " + e.Msg
}

// Canonical is v in canonical form, RFC 8785 as canon writes it: the bytes a
// digest and a comparison are of. It is how a response's value is made the
// same as another that says the same.
func Canonical(v canon.Value) ([]byte, error) { return canon.Encode(v, canon.Canonical) }
