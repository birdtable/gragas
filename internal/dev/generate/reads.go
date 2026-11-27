package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// A consumer's literal field list (contract/consumers/<name>.json) says which
// fields of which documents it reads (arch.ports.7). Generating checks it
// against the schemas, and writes it as a constant the consumer imports.

type consumer struct {
	Name  string
	Reads []read
}

type read struct {
	Document string
	Fields   []string
}

func parseConsumer(name string, data []byte) (consumer, error) {
	var raw struct {
		Consumer string `json:"consumer"`
		Reads    []struct {
			Document string   `json:"document"`
			Fields   []string `json:"fields"`
		} `json:"reads"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return consumer{}, fmt.Errorf("%s: %w", name, err)
	}
	if raw.Consumer == "" {
		return consumer{}, fmt.Errorf("%s names no consumer", name)
	}
	c := consumer{Name: raw.Consumer}
	for _, r := range raw.Reads {
		c.Reads = append(c.Reads, read{r.Document, r.Fields})
	}
	return c, nil
}

// missing lists the fields of a document the schemas do not have, and those
// that are written as a list but are not one.
func (t *tree) missing(document string, fields []string) []string {
	root := t.index[rootName("document-"+document+".schema.json")]
	var out []string
	for _, f := range fields {
		if root == nil || !t.has(root.T, strings.Split(f, ".")) {
			out = append(out, f)
		}
	}
	return out
}

func (t *tree) has(ty *typ, path []string) bool {
	ty = t.resolve(ty)
	if len(path) == 0 {
		return true
	}
	seg, list := strings.CutSuffix(path[0], "[]")
	if ty.Kind != kObject {
		return false
	}
	for _, f := range ty.Fields {
		if f.Key != seg {
			continue
		}
		ft := t.resolve(f.T)
		if list {
			if ft.Kind != kArray {
				return false
			}
			ft = ft.Elem
		}
		return t.has(ft, path[1:])
	}
	return false
}

func (t *tree) resolve(ty *typ) *typ {
	for ty.Kind == kRef {
		ty = t.index[ty.Ref].T
	}
	return ty
}

func readsFile(c consumer) []byte {
	var b bytes.Buffer
	b.WriteString(header("//", ConsumersDir+c.Name+".json") + "\n\n")
	b.WriteString("/** The fields of each document the " + c.Name + " reads, by document. */\n")
	fmt.Fprintf(&b, "export const %sReads = {\n", c.Name)
	for _, r := range c.Reads {
		fmt.Fprintf(&b, "  %s: [\n", r.Document)
		for _, f := range r.Fields {
			fmt.Fprintf(&b, "    %s,\n", jsonOf(f))
		}
		b.WriteString("  ],\n")
	}
	b.WriteString("} as const;\n")
	return b.Bytes()
}
