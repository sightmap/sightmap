package compquery

import (
	"fmt"
	"strconv"
)

// ParseQuery parses a component-query string into a Query. See the package doc
// for the grammar. Returns an error with position context on malformed input.
func ParseQuery(s string) (*Query, error) {
	p := &parser{s: s}
	q, err := p.parse()
	if err != nil {
		return nil, fmt.Errorf("compquery: parse %q: %w", s, err)
	}
	return q, nil
}

type parser struct {
	s string
	i int
}

func (p *parser) parse() (*Query, error) {
	q := &Query{Index: -1}
	p.skipSpaces()
	for p.i < len(p.s) {
		if p.s[p.i] == '#' {
			if len(q.Parts) == 0 {
				return nil, fmt.Errorf("'#index' must follow a component name")
			}
			p.i++
			idx, err := p.parseIndex()
			if err != nil {
				return nil, err
			}
			q.Index = idx
			p.skipSpaces()
			if p.i < len(p.s) {
				return nil, fmt.Errorf("unexpected %q after occurrence index", p.s[p.i:])
			}
			break
		}
		part, err := p.parsePart()
		if err != nil {
			return nil, err
		}
		q.Parts = append(q.Parts, part)
		p.skipSpaces()
	}
	if len(q.Parts) == 0 {
		return nil, fmt.Errorf("empty query")
	}
	return q, nil
}

func (p *parser) parsePart() (Part, error) {
	name := p.parseIdent(false)
	if name == "" {
		return Part{}, fmt.Errorf("expected component name at position %d", p.i)
	}
	part := Part{Name: name}
	for p.i < len(p.s) && p.s[p.i] == '[' {
		pred, err := p.parsePredicate()
		if err != nil {
			return Part{}, err
		}
		part.Preds = append(part.Preds, pred)
	}
	return part, nil
}

func (p *parser) parsePredicate() (Predicate, error) {
	p.i++ // consume '['
	p.skipSpaces()
	prop := p.parseIdent(true)
	if prop == "" {
		return Predicate{}, fmt.Errorf("expected property name at position %d", p.i)
	}
	p.skipSpaces()
	op, err := p.parseOp()
	if err != nil {
		return Predicate{}, err
	}
	p.skipSpaces()
	val, err := p.parseValue()
	if err != nil {
		return Predicate{}, err
	}
	p.skipSpaces()

	ci := false
	if p.i < len(p.s) && (p.s[p.i] == 'i' || p.s[p.i] == 'I') {
		// Case-insensitive flag, valid only immediately before ']'.
		j := p.i + 1
		for j < len(p.s) && isSpace(p.s[j]) {
			j++
		}
		if j < len(p.s) && p.s[j] == ']' {
			ci = true
			p.i = j
		}
	}

	if p.i >= len(p.s) || p.s[p.i] != ']' {
		return Predicate{}, fmt.Errorf("expected ']' to close predicate at position %d", p.i)
	}
	p.i++ // consume ']'
	return Predicate{Prop: prop, Op: op, Val: val, CI: ci}, nil
}

func (p *parser) parseOp() (string, error) {
	if p.i < len(p.s) {
		switch p.s[p.i] {
		case '^':
			if p.i+1 < len(p.s) && p.s[p.i+1] == '=' {
				p.i += 2
				return "^=", nil
			}
		case '*':
			if p.i+1 < len(p.s) && p.s[p.i+1] == '=' {
				p.i += 2
				return "*=", nil
			}
		case '=':
			p.i++
			return "=", nil
		}
	}
	return "", fmt.Errorf("expected operator (=, ^=, *=) at position %d", p.i)
}

func (p *parser) parseValue() (string, error) {
	if p.i >= len(p.s) {
		return "", fmt.Errorf("expected value at position %d", p.i)
	}
	if p.s[p.i] == '"' {
		return p.parseQuoted()
	}
	start := p.i
	for p.i < len(p.s) && !isSpace(p.s[p.i]) && p.s[p.i] != ']' {
		p.i++
	}
	if p.i == start {
		return "", fmt.Errorf("expected value at position %d", p.i)
	}
	return p.s[start:p.i], nil
}

func (p *parser) parseQuoted() (string, error) {
	p.i++ // consume opening quote
	var b []byte
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch c {
		case '\\':
			if p.i+1 >= len(p.s) {
				return "", fmt.Errorf("dangling escape in quoted value")
			}
			b = append(b, p.s[p.i+1])
			p.i += 2
		case '"':
			p.i++ // consume closing quote
			return string(b), nil
		default:
			b = append(b, c)
			p.i++
		}
	}
	return "", fmt.Errorf("unterminated quoted value")
}

func (p *parser) parseIndex() (int, error) {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if p.i == start {
		return 0, fmt.Errorf("expected digits after '#'")
	}
	n, err := strconv.Atoi(p.s[start:p.i])
	if err != nil {
		return 0, fmt.Errorf("invalid occurrence index %q", p.s[start:p.i])
	}
	return n, nil
}

// parseIdent reads an identifier. Component names allow letters, digits, and
// underscore; property names additionally allow hyphen (allowHyphen).
func (p *parser) parseIdent(allowHyphen bool) string {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if isIdentChar(c) || (allowHyphen && c == '-') {
			p.i++
			continue
		}
		break
	}
	return p.s[start:p.i]
}

func (p *parser) skipSpaces() {
	for p.i < len(p.s) && isSpace(p.s[p.i]) {
		p.i++
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f'
}

func isIdentChar(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}
