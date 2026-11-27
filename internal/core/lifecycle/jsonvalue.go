package lifecycle

import "strings"

// oneLineJSON is whether v is one JSON value (RFC 8259) on one line: a number,
// a literal, a string, a list or a map, with no whitespace but spaces. Such a
// value is also YAML flow, so it can be written as it is; it cannot hold a
// newline, so it cannot add a key.
func oneLineJSON(v string) bool {
	p := jsonScan{s: v}
	p.skip()
	if !p.value(0) {
		return false
	}
	p.skip()
	return p.i == len(p.s)
}

// maxJSONDepth bounds the nesting so a hostile value cannot recurse far.
const maxJSONDepth = 64

type jsonScan struct {
	s string
	i int
}

func (p *jsonScan) skip() {
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
}

func (p *jsonScan) eat(c byte) bool {
	if p.i < len(p.s) && p.s[p.i] == c {
		p.i++
		return true
	}
	return false
}

func (p *jsonScan) value(depth int) bool {
	if depth > maxJSONDepth || p.i >= len(p.s) {
		return false
	}
	switch c := p.s[p.i]; {
	case c == '"':
		return p.str()
	case c == '[':
		return p.list(depth)
	case c == '{':
		return p.object(depth)
	case c == '-' || c >= '0' && c <= '9':
		return p.number()
	}
	for _, lit := range []string{"true", "false", "null"} {
		if strings.HasPrefix(p.s[p.i:], lit) {
			p.i += len(lit)
			return true
		}
	}
	return false
}

func (p *jsonScan) list(depth int) bool {
	p.i++
	p.skip()
	if p.eat(']') {
		return true
	}
	for {
		p.skip()
		if !p.value(depth + 1) {
			return false
		}
		p.skip()
		if p.eat(']') {
			return true
		}
		if !p.eat(',') {
			return false
		}
	}
}

func (p *jsonScan) object(depth int) bool {
	p.i++
	p.skip()
	if p.eat('}') {
		return true
	}
	for {
		p.skip()
		if p.i >= len(p.s) || p.s[p.i] != '"' || !p.str() {
			return false
		}
		p.skip()
		if !p.eat(':') {
			return false
		}
		p.skip()
		if !p.value(depth + 1) {
			return false
		}
		p.skip()
		if p.eat('}') {
			return true
		}
		if !p.eat(',') {
			return false
		}
	}
}

func (p *jsonScan) str() bool {
	p.i++
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return true
		case c < 0x20:
			return false
		case c == '\\':
			if !p.escape() {
				return false
			}
		default:
			p.i++
		}
	}
	return false
}

// escape reads the escape at p.i, which starts at the backslash.
func (p *jsonScan) escape() bool {
	p.i++
	if p.i >= len(p.s) {
		return false
	}
	if strings.IndexByte("\"\\/bfnrt", p.s[p.i]) >= 0 {
		p.i++
		return true
	}
	if p.s[p.i] != 'u' || p.i+4 >= len(p.s) {
		return false
	}
	for _, h := range p.s[p.i+1 : p.i+5] {
		if !isHex(h) {
			return false
		}
	}
	p.i += 5
	return true
}

func isHex(h rune) bool {
	return h >= '0' && h <= '9' || h >= 'a' && h <= 'f' || h >= 'A' && h <= 'F'
}

func (p *jsonScan) number() bool {
	p.eat('-')
	switch {
	case p.eat('0'):
	case p.i < len(p.s) && p.s[p.i] >= '1' && p.s[p.i] <= '9':
		p.digits()
	default:
		return false
	}
	if p.eat('.') && !p.digits() {
		return false
	}
	if p.eat('e') || p.eat('E') {
		if !p.eat('+') {
			p.eat('-')
		}
		return p.digits()
	}
	return true
}

func (p *jsonScan) digits() bool {
	n := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	return p.i > n
}
