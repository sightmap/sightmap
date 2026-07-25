// Package sel provides CSS selector parsing and single-node matching for the
// sightmap selector subset. See doc.go for the supported grammar.
package sel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
)

// ParsedSelector is the result of parsing a CSS selector string.
// Parts contains one SelectorPart per simple-selector sequence, in
// root-first (ancestor → descendant) order. Combinators[i] is the combinator
// that precedes Parts[i]: empty string for Parts[0], " " (descendant) or ">"
// (direct child) for subsequent parts.
type ParsedSelector struct {
	Parts       []*comps.SelectorPart
	Combinators []string
}

// ParseSightmapSelector parses a sightmap CSS selector string into a
// ParsedSelector. Supports the sightmap subset: tag, #id, .class,
// [attr], [attr=val], [attr^=val], [attr$=val], [attr*=val], [attr~=val],
// [attr|=val], :not(), :is(), :where(), :has(), and descendant /
// direct-child (>) combinators. Sibling combinators (+, ~) are not supported,
// including inside :has().
func ParseSightmapSelector(s string) (ParsedSelector, error) {
	p := &parser{s: s}
	result, err := p.parseSelector()
	if err != nil {
		return ParsedSelector{}, fmt.Errorf("sel: parse %q: %w", s, err)
	}
	if p.i < len(s) {
		return ParsedSelector{}, fmt.Errorf("sel: parse %q: unexpected character %q at position %d", s, s[p.i], p.i)
	}
	return result, nil
}

// ---- parser internals -------------------------------------------------------

type parser struct {
	s string
	i int
}

func (p *parser) peek() (byte, bool) {
	if p.i >= len(p.s) {
		return 0, false
	}
	return p.s[p.i], true
}

func (p *parser) consume() byte {
	c := p.s[p.i]
	p.i++
	return c
}

// skipWhitespace advances past any ASCII whitespace. Returns true if any was skipped.
func (p *parser) skipWhitespace() bool {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' {
			p.i++
		} else {
			break
		}
	}
	return p.i > start
}

// nameChar returns true if c may appear in a CSS identifier (after the first char).
func nameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-' || c > 127
}

// nameStart returns true if c may be the first character of a CSS identifier
// (not counting a leading hyphen).
func nameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c > 127
}

// parseEscape parses a backslash-escape sequence and returns the decoded string.
// Handles both simple escapes (\X → X) and CSS hex escapes (\HHHHHH → rune).
func (p *parser) parseEscape() (string, error) {
	if p.i >= len(p.s) || p.s[p.i] != '\\' {
		return "", fmt.Errorf("expected backslash escape")
	}
	p.i++ // consume '\'
	if p.i >= len(p.s) {
		return "", fmt.Errorf("unexpected EOF after backslash")
	}
	c := p.s[p.i]
	if c == '\r' || c == '\n' || c == '\f' {
		return "", fmt.Errorf("escaped newline outside string")
	}
	// Hex escape: 1–6 hex digits, followed by an optional single whitespace.
	// CSS.escape() uses this form for characters that can't lead an identifier,
	// e.g. digit-leading class names: 2xl:foo → \32 xl\:foo.
	if isHexDigit(c) {
		var hex strings.Builder
		for p.i < len(p.s) && isHexDigit(p.s[p.i]) && hex.Len() < 6 {
			hex.WriteByte(p.s[p.i])
			p.i++
		}
		// Consume the optional single trailing whitespace.
		if p.i < len(p.s) && isCSSWhitespace(p.s[p.i]) {
			p.i++
		}
		cp, err := strconv.ParseInt(hex.String(), 16, 32)
		if err != nil || cp == 0 {
			return "\uFFFD", nil // replacement character for invalid codepoint
		}
		return string(rune(cp)), nil
	}
	// Simple escape: the next character literally.
	p.i++
	return string(c), nil
}

// isHexDigit reports whether c is a CSS hexadecimal digit.
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isCSSWhitespace reports whether c is CSS whitespace (space, tab, newline, CR, FF).
func isCSSWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// parseIdentifier parses a CSS identifier (tag name, class, attr key, etc.).
// Allows a leading hyphen per CSS spec.
func (p *parser) parseIdentifier() (string, error) {
	if p.i >= len(p.s) {
		return "", fmt.Errorf("expected identifier, found EOF")
	}

	var sb strings.Builder

	// Optional leading hyphen(s)
	numHyphens := 0
	for p.i < len(p.s) && p.s[p.i] == '-' {
		numHyphens++
		p.i++
	}

	if p.i >= len(p.s) {
		return "", fmt.Errorf("expected identifier, found EOF after hyphen")
	}

	c := p.s[p.i]
	if c == '\\' || nameStart(c) {
		sb.WriteString(strings.Repeat("-", numHyphens))
		// Shared loop for both escape-starting and nameStart-starting identifiers.
		// Handles: \32 xl\:foo (digit-leading class via CSS hex escape + Tailwind
		// colon variant) and hover\:text (Tailwind simple-escape mid-identifier).
		for p.i < len(p.s) {
			if p.s[p.i] == '\\' {
				escaped, escErr := p.parseEscape()
				if escErr != nil {
					break
				}
				sb.WriteString(escaped)
			} else if nameChar(p.s[p.i]) {
				sb.WriteByte(p.s[p.i])
				p.i++
			} else {
				break
			}
		}
	} else {
		// Put hyphens back — this isn't an identifier
		p.i -= numHyphens
		return "", fmt.Errorf("expected identifier, found %q", c)
	}

	return sb.String(), nil
}

// parseString parses a single- or double-quoted string value (for attribute selectors).
func (p *parser) parseString() (string, error) {
	if p.i >= len(p.s) {
		return "", fmt.Errorf("expected string, found EOF")
	}
	quote := p.s[p.i]
	if quote != '\'' && quote != '"' {
		return "", fmt.Errorf("expected quote, found %q", quote)
	}
	p.i++

	var sb strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == quote {
			p.i++
			return sb.String(), nil
		}
		if c == '\\' {
			escaped, err := p.parseEscape()
			if err != nil {
				return "", err
			}
			sb.WriteString(escaped)
			continue
		}
		if c == '\r' || c == '\n' || c == '\f' {
			return "", fmt.Errorf("unexpected newline in string")
		}
		sb.WriteByte(c)
		p.i++
	}
	return "", fmt.Errorf("unterminated string")
}

// parseSimpleSelectors parses a compound selector (a sequence of simple
// selectors with no combinators between them, e.g. div.foo#bar[attr]) and
// merges them into a single SelectorPart.
func (p *parser) parseSimpleSelectors() (*comps.SelectorPart, error) {
	part := &comps.SelectorPart{}
	sawAny := false

	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '*':
			// Universal selector — contributes nothing to SelectorPart.
			p.i++
			sawAny = true

		case c == '#':
			// ID selector
			p.i++
			id, err := p.parseIdentifier()
			if err != nil {
				return nil, fmt.Errorf("id selector: %w", err)
			}
			part.Id = id
			sawAny = true

		case c == '.':
			// Class selector
			p.i++
			cls, err := p.parseIdentifier()
			if err != nil {
				return nil, fmt.Errorf("class selector: %w", err)
			}
			part.Classes = append(part.Classes, cls)
			sawAny = true

		case c == '[':
			// Attribute selector
			if err := p.parseAttr(part); err != nil {
				return nil, err
			}
			sawAny = true

		case c == ':':
			// Pseudo-class — only :not() is supported.
			if err := p.parsePseudo(part); err != nil {
				return nil, err
			}
			sawAny = true

		case nameStart(c) || c == '-' || c == '\\':
			// Type selector (tag name). Only valid as the first token.
			tag, err := p.parseIdentifier()
			if err != nil {
				return nil, fmt.Errorf("type selector: %w", err)
			}
			part.Tag = strings.ToLower(tag)
			sawAny = true

		default:
			// Not part of a simple selector — stop.
			goto done
		}
	}
done:
	if !sawAny {
		return nil, fmt.Errorf("expected selector, found %q", func() string {
			if p.i < len(p.s) {
				return string(p.s[p.i])
			}
			return "EOF"
		}())
	}
	return part, nil
}

// parseAttr parses an attribute selector of the form [key], [key=val],
// [key^=val], etc., merging it into part.
func (p *parser) parseAttr(part *comps.SelectorPart) error {
	if p.i >= len(p.s) || p.s[p.i] != '[' {
		return fmt.Errorf("expected '['")
	}
	p.i++ // consume '['
	p.skipWhitespace()

	key, err := p.parseIdentifier()
	if err != nil {
		return fmt.Errorf("attribute key: %w", err)
	}
	key = strings.ToLower(key)
	p.skipWhitespace()

	if p.i >= len(p.s) {
		return fmt.Errorf("unexpected EOF in attribute selector")
	}

	if p.s[p.i] == ']' {
		// Presence-only: [attr]
		p.i++
		if part.Attrs == nil {
			part.Attrs = make(map[string]string)
		}
		if part.AttrOps == nil {
			part.AttrOps = make(map[string]string)
		}
		part.Attrs[key] = ""
		part.AttrOps[key] = "[]"
		return nil
	}

	// Parse operator: =, ~=, |=, ^=, $=, *=
	op, err := p.parseAttrOp()
	if err != nil {
		return err
	}
	p.skipWhitespace()

	if p.i >= len(p.s) {
		return fmt.Errorf("unexpected EOF after attribute operator")
	}

	var val string
	if p.s[p.i] == '\'' || p.s[p.i] == '"' {
		val, err = p.parseString()
	} else {
		val, err = p.parseIdentifier()
	}
	if err != nil {
		return fmt.Errorf("attribute value: %w", err)
	}

	p.skipWhitespace()
	if p.i >= len(p.s) || p.s[p.i] != ']' {
		return fmt.Errorf("expected ']' after attribute value")
	}
	p.i++ // consume ']'

	if part.Attrs == nil {
		part.Attrs = make(map[string]string)
	}
	part.Attrs[key] = val
	if op != "=" {
		if part.AttrOps == nil {
			part.AttrOps = make(map[string]string)
		}
		part.AttrOps[key] = op
	}
	return nil
}

// parseAttrOp parses a CSS attribute operator (=, ~=, |=, ^=, $=, *=).
func (p *parser) parseAttrOp() (string, error) {
	if p.i >= len(p.s) {
		return "", fmt.Errorf("expected attribute operator, found EOF")
	}
	c := p.s[p.i]
	if c == '=' {
		p.i++
		return "=", nil
	}
	if p.i+1 >= len(p.s) || p.s[p.i+1] != '=' {
		return "", fmt.Errorf("expected attribute operator, found %q", p.s[p.i:min(p.i+2, len(p.s))])
	}
	switch c {
	case '~', '|', '^', '$', '*':
		op := string([]byte{c, '='})
		p.i += 2
		return op, nil
	}
	return "", fmt.Errorf("unknown attribute operator starting with %q", c)
}

// parsePseudo parses a pseudo-class selector. Only :not() is supported.
func (p *parser) parsePseudo(part *comps.SelectorPart) error {
	if p.i >= len(p.s) || p.s[p.i] != ':' {
		return fmt.Errorf("expected ':'")
	}
	p.i++ // consume ':'

	name, err := p.parseIdentifier()
	if err != nil {
		return fmt.Errorf("pseudo-class name: %w", err)
	}
	name = strings.ToLower(name)

	switch name {
	case "not":
		// handled below

	case "has":
		if p.i >= len(p.s) || p.s[p.i] != '(' {
			return fmt.Errorf("expected '(' after :has")
		}
		p.i++ // consume '('
		h := &comps.HasSelector{}
		for {
			parts, combs, relErr := p.parseRelativeSelector()
			if relErr != nil {
				return fmt.Errorf(":has() argument: %w", relErr)
			}
			h.Alternatives = append(h.Alternatives, comps.HasRelative{Parts: parts, Combinators: combs})
			p.skipWhitespace()
			if p.i >= len(p.s) {
				return fmt.Errorf("expected ')' or ',' in :has()")
			}
			if p.s[p.i] == ')' {
				p.i++
				break
			}
			if p.s[p.i] != ',' {
				return fmt.Errorf("expected ')' or ',' in :has(), got %q", p.s[p.i])
			}
			p.i++ // consume ','
		}
		part.Has = append(part.Has, h)
		return nil

	case "is", "where":
		if p.i >= len(p.s) || p.s[p.i] != '(' {
			return fmt.Errorf("expected '(' after :%s", name)
		}
		p.i++ // consume '('
		var alts []*comps.SelectorPart
		for {
			p.skipWhitespace()
			inner, err := p.parseSimpleSelectors()
			if err != nil {
				return fmt.Errorf(":%s() argument: %w", name, err)
			}
			alts = append(alts, inner)
			p.skipWhitespace()
			if p.i >= len(p.s) {
				return fmt.Errorf("expected ')' or ',' in :%s()", name)
			}
			if p.s[p.i] == ')' {
				p.i++
				break
			}
			if p.s[p.i] != ',' {
				return fmt.Errorf("expected ')' or ',' in :%s(), got %q", name, p.s[p.i])
			}
			p.i++ // consume ','
		}
		part.Is = alts
		return nil

	default:
		return fmt.Errorf("unsupported pseudo-class :%s (only :not(), :is(), :where(), :has() are supported)"+
			" — to distinguish identical siblings, merge them into one component"+
			" (e.g. one CardActionButton) or add a distinguishing data-* attribute to the source element", name)
	}

	// :not() logic
	// Consume '('
	if p.i >= len(p.s) || p.s[p.i] != '(' {
		return fmt.Errorf("expected '(' after :not")
	}
	p.i++
	p.skipWhitespace()

	// Parse the inner simple selector.
	inner, err := p.parseSimpleSelectors()
	if err != nil {
		return fmt.Errorf(":not() argument: %w", err)
	}

	p.skipWhitespace()
	if p.i >= len(p.s) || p.s[p.i] != ')' {
		return fmt.Errorf("expected ')' to close :not()")
	}
	p.i++ // consume ')'

	part.Not = inner
	return nil
}

// parseRelativeSelector parses one relative selector inside :has(). It allows an
// optional leading combinator (">" for direct child; default " " for any
// descendant) and returns the compound parts plus their combinators, where
// Combinators[0] links the :has() anchor element to Parts[0]. Sibling
// combinators (+, ~) are rejected — the matcher has no sibling support.
func (p *parser) parseRelativeSelector() ([]*comps.SelectorPart, []string, error) {
	p.skipWhitespace()

	leading := " "
	if p.i < len(p.s) {
		switch p.s[p.i] {
		case '>':
			leading = ">"
			p.i++
			p.skipWhitespace()
		case '+', '~':
			return nil, nil, fmt.Errorf("sibling combinator %q is not supported", p.s[p.i])
		}
	}

	first, err := p.parseSimpleSelectors()
	if err != nil {
		return nil, nil, err
	}
	parts := []*comps.SelectorPart{first}
	combinators := []string{leading}

	for {
		hadSpace := p.skipWhitespace()
		if p.i >= len(p.s) {
			break
		}
		c := p.s[p.i]
		if c == ',' || c == ')' {
			break
		}
		combinator := ""
		switch {
		case c == '>':
			combinator = ">"
			p.i++
			p.skipWhitespace()
		case c == '+' || c == '~':
			return nil, nil, fmt.Errorf("sibling combinator %q is not supported", c)
		case hadSpace:
			combinator = " "
		default:
			// No combinator and no whitespace after a complete sequence.
			goto done
		}
		next, nerr := p.parseSimpleSelectors()
		if nerr != nil {
			return nil, nil, nerr
		}
		parts = append(parts, next)
		combinators = append(combinators, combinator)
	}
done:
	return parts, combinators, nil
}

// parseSelector parses a full selector (one or more simple sequences joined
// by combinators) and returns a ParsedSelector in root-first order.
func (p *parser) parseSelector() (ParsedSelector, error) {
	p.skipWhitespace()

	first, err := p.parseSimpleSelectors()
	if err != nil {
		return ParsedSelector{}, err
	}

	parts := []*comps.SelectorPart{first}
	combinators := []string{""}

	for {
		// Record whether whitespace precedes the next token.
		hadSpace := p.skipWhitespace()

		if p.i >= len(p.s) {
			break
		}

		// Check for end-of-selector tokens (comma, closing paren).
		c := p.s[p.i]
		if c == ',' || c == ')' {
			break
		}

		// Determine combinator.
		combinator := ""
		if c == '>' {
			combinator = ">"
			p.i++ // consume '>'
			p.skipWhitespace()
		} else if hadSpace {
			combinator = " "
		} else {
			// No combinator and no whitespace — shouldn't happen after a complete
			// simple sequence, but guard anyway.
			break
		}

		next, err := p.parseSimpleSelectors()
		if err != nil {
			return ParsedSelector{}, err
		}

		parts = append(parts, next)
		combinators = append(combinators, combinator)
	}

	return ParsedSelector{Parts: parts, Combinators: combinators}, nil
}
