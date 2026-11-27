package plugin

import "fmt"

// A call of a plugin fails in one of three ways a caller branches on, each a
// type here beside the messages it is about. The
// host's other failures (not approved, digest, contract, feature, grant, trap)
// carry nothing and are the port's sentinels. A caller takes one of these with
// errors.As.

// CallError is a plugin's own error: a code of the wire, a message for a person
// (display only, less-trusted) and, where it has one, the
// JSON pointer into the request body to the field at fault.
type CallError struct {
	// Code is one of the four of the wire. Decides: what the engine does with
	// the call.
	Code Code
	// Message is what the plugin says. Display only.
	Message string
	// Path is a JSON pointer into the request body, "" where the plugin names
	// none. Decides: which option a refusal names.
	Path string
}

// Error says the code, where there is a path, and the message, each written
// by Shown: they are the plugin's own text, any string the wire lets through,
// and no error that carries them may paint the terminal. This is the one
// place they are escaped; a screen that lays the fields out itself reads them
// raw and shows them by Shown too.
func (e *CallError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("the plugin answered %s at %s: %s", Shown(string(e.Code)), Shown(e.Path), Shown(e.Message))
	}
	return fmt.Sprintf("the plugin answered %s: %s", Shown(string(e.Code)), Shown(e.Message))
}

// Limit names one of the engine's limits on a call.
type Limit string

// The limits of a call.
const (
	// LimitTime is the time one call may take; the only one a quieter
	// machine could help.
	LimitTime Limit = "time"
	// LimitMemory is the memory of one module.
	LimitMemory Limit = "memory"
	// LimitRequest is the size of a request.
	LimitRequest Limit = "request"
	// LimitResponse is the size of a response.
	LimitResponse Limit = "response"
	// LimitRounds is the rounds a read may take.
	LimitRounds Limit = "rounds"
	// LimitEntries is the size of the input of an entries call: what a
	// plugin's entries are made from is capped, as a call over it runs out its
	// time.
	LimitEntries Limit = "entries"
)

// LimitError is a call that went over a limit of the engine's. A cancelled
// context is no limit: it ends the call with the context's own error.
type LimitError struct {
	// What is the limit. Decides: whether a quieter machine could help (time).
	What Limit
	// Size is the bytes there were, and Cap the most allowed, where the limit
	// is one of size and the engine counted it: both are 0 otherwise.
	Size, Cap int
}

// Error names the limit.
func (e *LimitError) Error() string {
	if e.Cap > 0 {
		msg := fmt.Sprintf("the plugin's call went over its limit of %s: %d bytes, over %d", e.What, e.Size, e.Cap)
		if e.What == LimitEntries {
			msg += "; the whole request is capped, so make the config and its files smaller, or have the plugin read fewer"
		}
		return msg
	}
	return "the plugin's call went over its limit of " + string(e.What)
}

// AnswerError is an answer that fails its schema or a rule of the wire.
type AnswerError struct {
	// Path is a JSON pointer into the response, "" for the whole of it.
	Path string
	// Why says what is wrong there. Display only.
	Why string
}

// Error says where and why, each escaped once by Shown, as CallError writes
// its fields. The place can hold the plugin's own key, the member of its
// answer that no message names, and the why can carry the
// words of an error the answer's text caused, so neither is printed raw.
func (e *AnswerError) Error() string {
	if e.Path == "" {
		return "the plugin's answer is not one: " + Shown(e.Why)
	}
	return "the plugin's answer is not one at " + Shown(e.Path) + ": " + Shown(e.Why)
}
