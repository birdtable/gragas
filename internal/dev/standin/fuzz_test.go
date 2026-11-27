package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// FuzzServe: any bytes in answer with one document of the wire and no panic:
// a single JSON object holding exactly one of "ok"
// and "error", written once with one trailing newline, and the same bytes
// again for the same request. Serve is the one decode of
// this program (Answer reads the body it is handed through the same call), so
// this target is the row of both in internal/testdata/decoders.txt.
//
// What it costs when this fails: the module the wasm host runs answers a
// request the engine sends with no document, or a different one the second
// time, and a call of the real host gives a bad answer where the fake gives a
// good one.
func FuzzServe(f *testing.F) {
	f.Add([]byte(`{"contract":1,"features":[],"op":"rank","body":{"tool_version":"1.0.0","pairs":[{"key":"k","aspect":"value","preset":2,"found":1}]}}`))
	f.Add([]byte(`{"contract":1,"features":[],"op":"plan","body":{"tool_version":"1.0.0","config":[],"have":[],"for":"config","target":"source","round":0}}`))
	f.Add([]byte(`{"contract":1,"features":[],"op":"entries","body":{"tool_version":"1.0.0","config":[{"path":"a","data":{"text":"b warning\n-c\n"}}],"have":[],"target":"source"}}`))
	f.Add([]byte(`{"contract":1,"features":[],"op":"preset","body":{"tool_version":"1.0.0","inputs":[],"entries":[{"key":"bug","target":"source","trial":false,"rules":["r"]}]}}`))
	f.Add([]byte(`{"op":"nope","body":{}}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, in []byte) {
		out := Serve(in)
		if !bytes.HasSuffix(out, []byte("}\n")) || bytes.Count(out, []byte("\n")) != 1 {
			t.Fatalf("Serve(%q) = %q, want one document and one trailing newline", in, out)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("Serve(%q) = %q, no JSON object: %v", in, out, err)
		}
		_, ok := doc["ok"]
		_, bad := doc["error"]
		if len(doc) != 1 || ok == bad {
			t.Fatalf("Serve(%q) = %q, want exactly one of ok and error", in, out)
		}
		if again := Serve(in); !bytes.Equal(again, out) {
			t.Fatalf("Serve(%q) answered %q and then %q", in, out, again)
		}
	})
}
