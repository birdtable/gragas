package wire

import (
	"strconv"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// EncodeRequest is r in canonical form, the bytes the engine writes to a
// plugin's stdin and digests. It refuses a request that is over 32 MiB, or an
// entries request over 64 KiB (*plugin.LimitError), a value over its caps, a path of a config file that is not
// one of the contract, and the data of a config file over 1 MiB (*RequestError).
func (c *Codec) EncodeRequest(r plugin.Request) ([]byte, error) {
	if err := c.checkConfig(r); err != nil {
		return nil, err
	}
	if err := c.checkHave(r); err != nil {
		return nil, err
	}
	if err := checkPairs(r); err != nil {
		return nil, err
	}
	b, err := canon.Encode(r.ToCanon(), canon.Canonical)
	if err != nil {
		return nil, &RequestError{Msg: "has no canonical form: " + err.Error()}
	}
	if len(b) > MaxRequest {
		return nil, &plugin.LimitError{What: plugin.LimitRequest, Size: len(b), Cap: MaxRequest}
	}
	if r.Op == plugin.OpEntries && len(b) > MaxEntriesInput {
		return nil, &plugin.LimitError{What: plugin.LimitEntries, Size: len(b), Cap: MaxEntriesInput}
	}
	return b, nil
}

// NewRequest is a request of op with body and the features of the contract.
func NewRequest(op plugin.Op, body canon.Object) plugin.Request {
	return plugin.Request{Features: Features(), Op: op, Body: body}
}

// checkHave holds the results of a request body's have to the contract: in
// order by round, then step, each once; each stream within 8 MiB; the files of
// one result at most 256, 1 MiB each and 8 MiB in all; and bytes in the form
// of the contract (text when valid UTF-8, else base64).
func (c *Codec) checkHave(r plugin.Request) error {
	list, ok := r.Body["have"].(canon.Array)
	if !ok {
		return nil
	}
	prev := [2]int{-1, -1}
	for i, e := range list {
		at := "/body/have/" + strconv.Itoa(i)
		var res plugin.Result
		if err := res.FromCanon(e); err != nil {
			return &RequestError{Path: at, Msg: err.Error()}
		}
		if res.Round < prev[0] || (res.Round == prev[0] && res.Step <= prev[1]) {
			return &RequestError{Path: at, Msg: "is out of order: results go by round, then step, each once"}
		}
		prev = [2]int{res.Round, res.Step}
		if err := c.checkResult(res, at); err != nil {
			return err
		}
	}
	return nil
}

// checkResult holds one result's streams and files to their caps and to the
// form of bytes, and each file's path to the path rule: no path outside the
// project and none under .git.
func (c *Codec) checkResult(res plugin.Result, at string) error {
	for name, b := range map[string]*plugin.Bytes{"stdout": res.Stdout, "stderr": res.Stderr} {
		if b == nil {
			continue
		}
		if why := bytesForm(*b, MaxStream); why != "" {
			return &RequestError{Path: at + "/" + name, Msg: why}
		}
	}
	if len(res.Files) > MaxReadFiles {
		return &RequestError{Path: at + "/files", Msg: "has more than " + strconv.Itoa(MaxReadFiles) + " files"}
	}
	total := 0
	for j, f := range res.Files {
		if why := c.rules.pathFault(f.Path); why != "" {
			return &RequestError{Path: at + "/files/" + strconv.Itoa(j) + "/path", Msg: why}
		}
		if why := bytesForm(f.Data, MaxFile); why != "" {
			return &RequestError{Path: at + "/files/" + strconv.Itoa(j) + "/data", Msg: why}
		}
		n, _ := sizeOf(f.Data)
		total += n
	}
	if total > MaxFiles {
		return &RequestError{Path: at + "/files", Msg: "hold more than " + strconv.Itoa(MaxFiles) + " bytes in all"}
	}
	return nil
}

// checkConfig holds the config files of a request body to the contract's path
// rule and its 1 MiB cap of data. A body with no config has none to hold.
func (c *Codec) checkConfig(r plugin.Request) error {
	list, ok := r.Body["config"].(canon.Array)
	if !ok {
		return nil
	}
	for i, e := range list {
		at := "/body/config/" + strconv.Itoa(i)
		var cf plugin.Config
		if err := cf.FromCanon(e); err != nil {
			return &RequestError{Path: at, Msg: err.Error()}
		}
		if why := c.rules.pathFault(cf.Path); why != "" {
			return &RequestError{Path: at + "/path", Msg: why}
		}
		if cf.Data != nil {
			if why := bytesFault(*cf.Data, MaxFile); why != "" {
				return &RequestError{Path: at + "/data", Msg: why}
			}
		}
	}
	return nil
}

// checkPairs holds the preset and found of each pair of a rank request to the
// caps of a value: 64 KiB in canonical form and 32 levels.
func checkPairs(r plugin.Request) error {
	list, ok := r.Body["pairs"].(canon.Array)
	if !ok {
		return nil
	}
	for i, e := range list {
		o, isObject := e.(canon.Object)
		if !isObject {
			continue
		}
		for _, name := range []string{"preset", "found"} {
			if why := valueFault(o[name]); why != "" {
				return &RequestError{Path: "/body/pairs/" + strconv.Itoa(i) + "/" + name, Msg: why}
			}
		}
	}
	return nil
}
