package wire

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// Codec reads and writes the messages of plugin contract 1. It holds the
// port that says what a file name is, and nothing else: it has no state, so
// one Codec serves every call.
type Codec struct {
	rules rules
}

// New is a Codec that asks names whether a part of a path is one file's name
// on every system.
func New(names port.FileNames) *Codec { return &Codec{rules: rules{names: names}} }

// answers is the codes each operation's error may carry. A plan
// may answer unreadable after round 0, as the table says; that rule is
// answerCodeFault's.
func answers(op plugin.Op) []plugin.Code {
	return map[plugin.Op][]plugin.Code{
		plugin.OpPlan:     {plugin.CodeUnsupported, plugin.CodeUnreadable, plugin.CodeRefused, plugin.CodeBug},
		plugin.OpEntries:  {plugin.CodeUnsupported, plugin.CodeUnreadable, plugin.CodeBug},
		plugin.OpFindings: {plugin.CodeUnsupported, plugin.CodeUnreadable, plugin.CodeBug},
		plugin.OpRank:     {plugin.CodeUnsupported, plugin.CodeBug},
		plugin.OpPreset:   {plugin.CodeUnsupported, plugin.CodeRefused, plugin.CodeBug},
	}[op]
}

// DecodePlan reads the answer to a plan request of the given round. A plan
// with no steps that is not final is refused: it asks for nothing and says it
// is not done. Each run step names a program that is one of
// grants, the ids of the manifest: a program the plugin was never granted is
// refused here, before anything runs.
func (c *Codec) DecodePlan(data []byte, round int, grants []string) (plugin.PlanResponse, error) {
	out, err := answer[plugin.PlanResponse](c, data, plugin.OpPlan, round)
	if err != nil {
		return out, err
	}
	if len(out.Steps) == 0 && !out.Final {
		return out, &plugin.AnswerError{Path: "/ok", Why: "has no steps and is not final"}
	}
	for i, s := range out.Steps {
		at := "/ok/steps/" + strconv.Itoa(i)
		switch {
		case s.Run != nil:
			if err := c.checkRun(*s.Run, at, grants); err != nil {
				return out, err
			}
		case s.Read != nil:
			if why := c.rules.globFault(s.Read.Glob); why != "" {
				return out, &plugin.AnswerError{Path: at + "/glob", Why: why}
			}
		}
	}
	return out, nil
}

// checkRun holds a run step to the rules of its argv, its place, its collect
// and its stdin.
func (c *Codec) checkRun(r plugin.Run, at string, grants []string) error {
	if !slices.Contains(grants, r.Program) {
		// The program is written as the plugin sent it: AnswerError.Error writes
		// the why through Shown, and a quote here would double its backslashes.
		return &grantError{&plugin.AnswerError{Path: at + "/program", Why: `names "` + r.Program + `", which is not a grant of the manifest`}}
	}
	for i, w := range r.Argv {
		if why := wordFault(w); why != "" {
			return &plugin.AnswerError{Path: at + "/argv/" + strconv.Itoa(i), Why: why}
		}
	}
	if r.Stdin != nil {
		if why := bytesForm(*r.Stdin, MaxStdin); why != "" {
			return &plugin.AnswerError{Path: at + "/stdin", Why: why}
		}
	}
	for i, f := range r.Place {
		fat := at + "/place/" + strconv.Itoa(i)
		if why := c.rules.pathFault(f.Path); why != "" {
			return &plugin.AnswerError{Path: fat + "/path", Why: why}
		}
		if why := bytesForm(f.Data, MaxFile); why != "" {
			return &plugin.AnswerError{Path: fat + "/data", Why: why}
		}
	}
	for i, p := range r.Collect {
		if why := c.rules.pathFault(p); why != "" {
			return &plugin.AnswerError{Path: at + "/collect/" + strconv.Itoa(i), Why: why}
		}
	}
	return nil
}

// DecodeEntries reads the answer to an entries request: the entries sorted by
// key, as UTF-8 bytes, each key once, and each value and options held to its
// caps. An entry out of order is refused and never sorted.
func (c *Codec) DecodeEntries(data []byte) (plugin.EntriesResponse, error) {
	out, err := answer[plugin.EntriesResponse](c, data, plugin.OpEntries, 0)
	if err != nil {
		return out, err
	}
	for i, e := range out.Entries {
		at := "/ok/entries/" + strconv.Itoa(i)
		if i > 0 {
			switch strings.Compare(out.Entries[i-1].Key, e.Key) {
			case 0:
				return out, &plugin.AnswerError{Path: at + "/key", Why: "repeats the key of the entry before it"}
			case 1:
				return out, &plugin.AnswerError{Path: at + "/key", Why: "is out of order: entries are sorted by key, as UTF-8 bytes"}
			}
		}
		if why := valueFault(e.Value); why != "" {
			return out, &plugin.AnswerError{Path: at + "/value", Why: why}
		}
		if why := valueFault(e.Options); why != "" {
			return out, &plugin.AnswerError{Path: at + "/options", Why: why}
		}
	}
	return out, nil
}

// DecodeFindings reads the answer to a findings request: the findings sorted
// by path, line, column, key and message, a field that is absent first, and
// each path one of the contract.
func (c *Codec) DecodeFindings(data []byte) (plugin.FindingsResponse, error) {
	out, err := answer[plugin.FindingsResponse](c, data, plugin.OpFindings, 0)
	if err != nil {
		return out, err
	}
	for i, f := range out.Findings {
		at := "/ok/findings/" + strconv.Itoa(i)
		if f.Path != nil {
			if why := c.rules.pathFault(*f.Path); why != "" {
				return out, &plugin.AnswerError{Path: at + "/path", Why: why}
			}
		}
		if i > 0 && compareFindings(out.Findings[i-1], f) > 0 {
			return out, &plugin.AnswerError{Path: at, Why: "is out of order: findings are sorted by path, line, column, key and message, an absent one first"}
		}
	}
	return out, nil
}

// compareFindings orders two findings as the contract does: by path, line,
// column, key and message, and a field that is absent before one that is not.
func compareFindings(a, b plugin.Finding) int {
	for _, c := range []int{
		compareOpt(a.Path, b.Path, strings.Compare),
		compareOpt(a.Line, b.Line, func(x, y int) int { return x - y }),
		compareOpt(a.Column, b.Column, func(x, y int) int { return x - y }),
		strings.Compare(a.Key, b.Key),
		strings.Compare(a.Message, b.Message),
	} {
		if c != 0 {
			return c
		}
	}
	return 0
}

// compareOpt orders two optional values, an absent one first.
func compareOpt[T any](a, b *T, cmp func(x, y T) int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return cmp(*a, *b)
}

// DecodeRank reads the answer to a rank request of the given number of pairs:
// one word to each pair, by position, and as many.
func (c *Codec) DecodeRank(data []byte, pairs int) (plugin.RankResponse, error) {
	out, err := answer[plugin.RankResponse](c, data, plugin.OpRank, 0)
	if err != nil {
		return out, err
	}
	if len(out.Words) != pairs {
		return out, &plugin.AnswerError{Path: "/ok/words", Why: "has " + strconv.Itoa(len(out.Words)) + " words for " + strconv.Itoa(pairs) + " pairs: a word answers each pair, by position"}
	}
	return out, nil
}

// DecodePreset reads the answer to a preset request that gave mark, "" for none:
// the files sorted by path, each once, each path one of the contract, and the
// mark in the first line of each file when there is one.
func (c *Codec) DecodePreset(data []byte, mark string) (plugin.PresetResponse, error) {
	out, err := answer[plugin.PresetResponse](c, data, plugin.OpPreset, 0)
	if err != nil {
		return out, err
	}
	for i, f := range out.Files {
		at := "/ok/files/" + strconv.Itoa(i)
		if why := c.rules.pathFault(f.Path); why != "" {
			return out, &plugin.AnswerError{Path: at + "/path", Why: why}
		}
		if i > 0 {
			switch strings.Compare(out.Files[i-1].Path, f.Path) {
			case 0:
				return out, &plugin.AnswerError{Path: at + "/path", Why: "repeats the path of the file before it"}
			case 1:
				return out, &plugin.AnswerError{Path: at + "/path", Why: "is out of order: files are sorted by path, as UTF-8 bytes"}
			}
		}
		if mark != "" && !strings.Contains(firstLine(f.Text), mark) {
			return out, &plugin.AnswerError{Path: at + "/text", Why: "does not hold the mark in its first line"}
		}
	}
	return out, nil
}

// firstLine is text up to its first line end.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// answer reads the response in data for op and, when it is an ok, its body as T.
// A plugin's error is a *plugin.CallError; any other refusal is an *plugin.AnswerError, or a
// *plugin.LimitError for a response over its cap.
func answer[T any, P interface {
	*T
	FromCanon(canon.Value) error
}](c *Codec, data []byte, op plugin.Op, round int) (T, error) {
	var out T
	body, err := c.envelope(data, op, round)
	if err != nil {
		return out, err
	}
	if err := P(&out).FromCanon(body); err != nil {
		return out, asAnswer("/ok", err)
	}
	return out, nil
}

// envelope reads the response in data as the one of the contract: an ok or an
// error, and for an error the code its operation answers.
func (c *Codec) envelope(data []byte, op plugin.Op, round int) (canon.Object, error) {
	if len(data) > MaxResponse {
		return nil, &plugin.LimitError{What: plugin.LimitResponse, Size: len(data), Cap: MaxResponse}
	}
	if !utf8.Valid(data) {
		return nil, &plugin.AnswerError{Why: "is not UTF-8"}
	}
	v, err := canon.Decode(data)
	if err != nil {
		return nil, &plugin.AnswerError{Why: "is not JSON the engine reads: " + err.Error()}
	}
	var r plugin.Response
	if err := r.FromCanon(v); err != nil {
		return nil, asAnswer("", err)
	}
	if r.Error == nil {
		return r.Ok, nil
	}
	e := *r.Error
	if !slices.Contains(answers(op), e.Code) || (op == plugin.OpPlan && e.Code == plugin.CodeUnreadable && round == 0) {
		return nil, &plugin.AnswerError{Path: "/error/code", Why: "is " + string(e.Code) + ", which a " + string(op) + " does not answer here"}
	}
	if e.Path != nil {
		if why := pointerFault(*e.Path); why != "" {
			return nil, &plugin.AnswerError{Path: "/error/path", Why: why}
		}
	}
	call := &plugin.CallError{Code: e.Code, Message: e.Message}
	if e.Path != nil {
		call.Path = *e.Path
	}
	return nil, call
}

// asAnswer is err, a refusal of a generated FromCanon, as an AnswerError at
// prefix and the place it names.
func asAnswer(prefix string, err error) error {
	if f, ok := errors.AsType[*plugin.Fault](err); ok {
		return &plugin.AnswerError{Path: prefix + f.At, Why: f.Msg}
	}
	return &plugin.AnswerError{Path: prefix, Why: err.Error()}
}
