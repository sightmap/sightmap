package sightmap

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SelectorProfile names a set of chain-evaluable selector features that every
// engine in the profile's baseline implements identically (SEP-0018).
type SelectorProfile string

// ProfileCaptureBaseline is the CSS3 subset common to the oldest engines a
// capture consumer still has to support (Chrome 38, Firefox 29, Safari 9,
// Edge 80) whose selectors depend only on an element and its ancestors.
const ProfileCaptureBaseline SelectorProfile = "capture-baseline"

// CodeSelectorNotInProfile marks a selector construct, or a form, that the
// active profile does not include (SEP-0018). The message says why: an engine
// in the baseline lacks it, or it depends on more than the element and its
// ancestors.
const CodeSelectorNotInProfile = "selector-not-in-profile"

const (
	profileMaxCompounds = 8
	profileMaxBytes     = 1024
)

// Constraint is how narrowly a compound selects on its own.
type Constraint int

const (
	// ConstraintNone: no type, id, class or attribute outside :not() (`*`, `:not(.x)`).
	ConstraintNone Constraint = iota
	// ConstraintType: a type selector and nothing narrower (`body`, `div:not(.x)`).
	ConstraintType
	// ConstraintSpecific: an id, class or attribute outside :not().
	ConstraintSpecific
)

// ProfileCompound is one compound of a profile selector.
type ProfileCompound struct {
	// Text is the compound exactly as written, valid CSS on its own.
	Text string
	// Constraint is how narrowly the compound selects.
	Constraint Constraint
	// Attrs are the attribute names the compound tests, including inside :not().
	Attrs []string
}

// ProfileSelector is a selector accepted by a profile, split into compounds.
type ProfileSelector struct {
	Compounds []ProfileCompound
	// Combinators[i] joins Compounds[i-1] and Compounds[i]: " " or ">".
	// Combinators[0] is "".
	Combinators []string
}

// Subject is the last compound, the one an element must match itself.
func (s ProfileSelector) Subject() ProfileCompound {
	return s.Compounds[len(s.Compounds)-1]
}

// ProfileError reports why a selector is outside a profile.
type ProfileError struct {
	Code string // CodeSelectorNotInProfile
	Pos  int    // byte offset into the selector
	Msg  string
}

func (e *ProfileError) Error() string {
	return fmt.Sprintf("%s at position %d: %s", e.Code, e.Pos, e.Msg)
}

// beyondAncestorPseudos depend on siblings, children or live state.
var beyondAncestorPseudos = map[string]bool{
	"has": true, "empty": true,
	"first-child": true, "last-child": true, "only-child": true,
	"first-of-type": true, "last-of-type": true, "only-of-type": true,
	"nth-child": true, "nth-last-child": true, "nth-of-type": true, "nth-last-of-type": true,
	"hover": true, "active": true, "focus": true, "focus-within": true, "focus-visible": true,
	"visited": true, "link": true, "any-link": true, "target": true, "checked": true,
	"disabled": true, "enabled": true, "indeterminate": true, "default": true,
	"valid": true, "invalid": true, "required": true, "optional": true,
	"read-only": true, "read-write": true, "placeholder-shown": true,
	"in-range": true, "out-of-range": true, "autofill": true, "fullscreen": true,
	"open": true, "closed": true, "playing": true, "paused": true,
}

// ParseProfileSelector parses sel against profile, returning its compounds or
// a *ProfileError naming the first construct outside the profile. Surrounding
// spaces are ignored.
func ParseProfileSelector(sel string, profile SelectorProfile) (ProfileSelector, error) {
	if profile != ProfileCaptureBaseline {
		return ProfileSelector{}, fmt.Errorf("unknown selector profile %q", profile)
	}
	p := &profileParser{s: sel}
	return p.parse()
}

type profileParser struct {
	s string
	i int
}

func (p *profileParser) fail(code string, pos int, format string, args ...any) error {
	return &ProfileError{Code: code, Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

func (p *profileParser) notInProfile(pos int, format string, args ...any) error {
	return p.fail(CodeSelectorNotInProfile, pos, format, args...)
}

func (p *profileParser) parse() (ProfileSelector, error) {
	var out ProfileSelector
	if len(p.s) > profileMaxBytes {
		return out, p.notInProfile(profileMaxBytes, "selector is longer than %d bytes", profileMaxBytes)
	}
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
	end := len(p.s)
	for end > p.i && p.s[end-1] == ' ' {
		end--
	}
	if p.i == end {
		return out, p.notInProfile(p.i, "empty selector")
	}
	p.s = p.s[:end]

	combinator := ""
	for {
		c, err := p.compound()
		if err != nil {
			return ProfileSelector{}, err
		}
		out.Compounds = append(out.Compounds, c)
		out.Combinators = append(out.Combinators, combinator)
		if len(out.Compounds) > profileMaxCompounds {
			return ProfileSelector{}, p.notInProfile(p.i, "more than %d compounds", profileMaxCompounds)
		}
		if p.i == len(p.s) {
			return out, nil
		}
		combinator, err = p.combinator()
		if err != nil {
			return ProfileSelector{}, err
		}
	}
}

// combinator consumes a descendant or child combinator between compounds.
func (p *profileParser) combinator() (string, error) {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
	if p.i == len(p.s) {
		return "", p.notInProfile(start, "trailing combinator")
	}
	switch c := p.s[p.i]; c {
	case '>':
		p.i++
		for p.i < len(p.s) && p.s[p.i] == ' ' {
			p.i++
		}
		if p.i == len(p.s) {
			return "", p.notInProfile(start, "trailing combinator")
		}
		return ">", nil
	case '+', '~':
		return "", p.fail(CodeSelectorNotInProfile, p.i, "sibling combinator %q depends on elements off the ancestor chain", c)
	case ',':
		return "", p.notInProfile(p.i, "a selector list; list alternatives as separate selectors")
	}
	if p.i == start {
		return "", p.unexpected()
	}
	return " ", nil
}

func (p *profileParser) unexpected() error {
	r, _ := utf8.DecodeRuneInString(p.s[p.i:])
	if unicode.IsSpace(r) {
		return p.notInProfile(p.i, "whitespace other than U+0020 outside a string")
	}
	return p.notInProfile(p.i, "unexpected %q", r)
}

// compound parses one compound, stopping before a combinator or the end.
func (p *profileParser) compound() (ProfileCompound, error) {
	start := p.i
	var c ProfileCompound
	if p.i < len(p.s) && p.s[p.i] == '*' {
		p.i++
		if !p.atCompoundEnd() {
			return c, p.notInProfile(p.i, "'*' must be a whole compound; drop it")
		}
		c.Text = p.s[start:p.i]
		return c, nil
	}
	sawSimple, sawType, sawID := false, false, false
	for !p.atCompoundEnd() {
		ch := p.s[p.i]
		first := p.i == start
		switch {
		case ch == '*':
			return c, p.notInProfile(p.i, "'*' must be a whole compound")
		case ch == '#':
			if sawID {
				return c, p.notInProfile(p.i, "a compound can have only one #id")
			}
			p.i++
			if err := p.ident("id"); err != nil {
				return c, err
			}
			sawID, sawSimple = true, true
		case ch == '.':
			p.i++
			if err := p.ident("class"); err != nil {
				return c, err
			}
			sawSimple = true
		case ch == '[':
			name, err := p.attribute()
			if err != nil {
				return c, err
			}
			c.Attrs = appendUnique(c.Attrs, name)
			sawSimple = true
		case ch == ':':
			attrs, err := p.pseudo()
			if err != nil {
				return c, err
			}
			for _, a := range attrs {
				c.Attrs = appendUnique(c.Attrs, a)
			}
			continue // :not() narrows nothing on its own
		case isTypeStart(ch) || ('A' <= ch && ch <= 'Z'):
			if !first {
				return c, p.notInProfile(p.i, "a type selector must come first in a compound")
			}
			if err := p.typeName(); err != nil {
				return c, err
			}
			sawType = true
			continue
		default:
			return c, p.unexpected()
		}
	}
	if p.i == start {
		return c, p.unexpected()
	}
	switch {
	case sawSimple:
		c.Constraint = ConstraintSpecific
	case sawType:
		c.Constraint = ConstraintType
	}
	c.Text = p.s[start:p.i]
	return c, nil
}

func (p *profileParser) atCompoundEnd() bool {
	if p.i >= len(p.s) {
		return true
	}
	switch p.s[p.i] {
	case ' ', '>', '+', '~', ',':
		return true
	}
	return false
}

func isTypeStart(c byte) bool { return 'a' <= c && c <= 'z' }

func (p *profileParser) typeName() error {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if 'A' <= c && c <= 'Z' {
			return p.notInProfile(p.i, "type names are written in lowercase")
		}
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			break
		}
		p.i++
	}
	if !isTypeStart(p.s[start]) {
		return p.notInProfile(start, "a type name starts with a lowercase letter")
	}
	return nil
}

// ident consumes a CSS identifier without hex escapes.
func (p *profileParser) ident(what string) error {
	start := p.i
	if p.i < len(p.s) && p.s[p.i] == '-' {
		p.i++
	}
	first := true
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '\\':
			if err := p.escape(); err != nil {
				return err
			}
		case c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
			p.i++
		case !first && ('0' <= c && c <= '9' || c == '-'):
			p.i++
		case c >= utf8.RuneSelf:
			r, n := utf8.DecodeRuneInString(p.s[p.i:])
			if unicode.IsSpace(r) || r == utf8.RuneError {
				return p.notInProfile(p.i, "whitespace other than U+0020 outside a string")
			}
			p.i += n
		default:
			if first {
				return p.notInProfile(p.i, "%s name must start with a letter, '_', a non-ASCII character or an escape", what)
			}
			return nil
		}
		first = false
	}
	if first {
		return p.notInProfile(start, "empty %s name", what)
	}
	return nil
}

// escape consumes a backslash and one printable ASCII character that is not a
// hex digit. Hex escapes are outside the profile.
func (p *profileParser) escape() error {
	pos := p.i
	p.i++
	if p.i >= len(p.s) {
		return p.notInProfile(pos, "escape at end of selector")
	}
	c := p.s[p.i]
	if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' {
		return p.notInProfile(pos, "hex escapes are not in the profile; escape a single character instead")
	}
	if c < '!' || c > '~' {
		return p.notInProfile(pos, "only a printable ASCII character can be escaped")
	}
	p.i++
	return nil
}

// attribute consumes [name], [name op value], returning the name.
func (p *profileParser) attribute() (string, error) {
	open := p.i
	p.i++ // '['
	nameStart := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if 'A' <= c && c <= 'Z' {
			return "", p.notInProfile(p.i, "attribute names are written in lowercase")
		}
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '_' || c == '-') {
			break
		}
		p.i++
	}
	name := p.s[nameStart:p.i]
	if name == "" || !('a' <= name[0] && name[0] <= 'z' || name[0] == '_') {
		return "", p.attrUnexpected(nameStart)
	}
	if p.i >= len(p.s) {
		return "", p.notInProfile(open, "unclosed attribute selector")
	}
	if p.s[p.i] == ']' {
		p.i++
		return name, nil
	}
	opStart := p.i
	op := ""
	switch {
	case p.s[p.i] == '=':
		op = "="
		p.i++
	case p.i+1 < len(p.s) && p.s[p.i+1] == '=' && strings.IndexByte("~|^$*", p.s[p.i]) >= 0:
		op = p.s[p.i : p.i+2]
		p.i += 2
	default:
		return "", p.attrUnexpected(p.i)
	}
	if p.i >= len(p.s) {
		return "", p.notInProfile(open, "unclosed attribute selector")
	}
	valStart := p.i
	var value string
	if q := p.s[p.i]; q == '"' || q == '\'' {
		v, err := p.str()
		if err != nil {
			return "", err
		}
		value = v
	} else {
		if err := p.ident("attribute value"); err != nil {
			return "", err
		}
		value = p.s[valStart:p.i]
	}
	if p.i >= len(p.s) {
		return "", p.notInProfile(open, "unclosed attribute selector")
	}
	if p.s[p.i] != ']' {
		if p.s[p.i] == ' ' {
			return "", p.notInProfile(p.i, "whitespace inside an attribute selector, or an i/s flag, is not in the profile")
		}
		return "", p.attrUnexpected(p.i)
	}
	p.i++
	switch {
	case op != "=" && value == "":
		return "", p.notInProfile(opStart, "%s with an empty value never matches", op)
	case op == "~=" && strings.ContainsAny(value, " \t\n\r\f"):
		return "", p.notInProfile(valStart, "a ~= value with whitespace never matches")
	case name == "class" && op != "~=" && op != "*=":
		return "", p.notInProfile(opStart, "on class only [class], ~= and *= are in the profile; use .class")
	}
	return name, nil
}

func (p *profileParser) attrUnexpected(pos int) error {
	if pos < len(p.s) && p.s[pos] == '|' {
		return p.notInProfile(pos, "attribute namespaces are not in the profile")
	}
	if pos < len(p.s) && (p.s[pos] == ' ' || p.s[pos] == '\t') {
		return p.notInProfile(pos, "whitespace inside an attribute selector is not in the profile")
	}
	return p.notInProfile(pos, "malformed attribute selector")
}

// str consumes a quoted string, returning its unescaped-enough content (escapes
// kept as written; only emptiness and whitespace are inspected).
func (p *profileParser) str() (string, error) {
	q := p.s[p.i]
	open := p.i
	p.i++
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == q:
			v := p.s[start:p.i]
			p.i++
			return v, nil
		case c == '\\':
			if err := p.escape(); err != nil {
				return "", err
			}
		case c < 0x20 || c == 0x7f:
			return "", p.notInProfile(p.i, "control character in a string")
		default:
			p.i++
		}
	}
	return "", p.notInProfile(open, "unterminated string")
}

// pseudo consumes a pseudo-class. Only :not() with one simple selector is in
// the profile; it returns the attribute names the argument tests.
func (p *profileParser) pseudo() ([]string, error) {
	pos := p.i
	p.i++ // ':'
	if p.i < len(p.s) && p.s[p.i] == ':' {
		return nil, p.notInProfile(pos, "pseudo-elements are not in the profile")
	}
	nameStart := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-') {
			break
		}
		p.i++
	}
	written := p.s[nameStart:p.i]
	name := strings.ToLower(written)
	switch {
	case name == "":
		return nil, p.notInProfile(pos, "malformed pseudo-class")
	case beyondAncestorPseudos[name]:
		return nil, p.fail(CodeSelectorNotInProfile, pos, ":%s depends on more than the element and its ancestors", name)
	case name != "not":
		return nil, p.notInProfile(pos, ":%s is not in the profile; only :not() with one simple selector is", name)
	case written != name:
		return nil, p.notInProfile(pos, "pseudo-class names are written in lowercase")
	}
	if p.i >= len(p.s) || p.s[p.i] != '(' {
		return nil, p.notInProfile(pos, ":not needs an argument")
	}
	p.i++
	argStart := p.i
	var attrs []string
	if p.i < len(p.s) {
		switch c := p.s[p.i]; {
		case c == ':':
			if _, err := p.pseudo(); err != nil {
				return nil, err
			}
			return nil, p.notInProfile(argStart, "a pseudo-class inside :not() is not in the profile")
		case c == '*':
			return nil, p.notInProfile(argStart, ":not(*) is not in the profile")
		case c == '#':
			p.i++
			if err := p.ident("id"); err != nil {
				return nil, err
			}
		case c == '.':
			p.i++
			if err := p.ident("class"); err != nil {
				return nil, err
			}
		case c == '[':
			name, err := p.attribute()
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, name)
		case isTypeStart(c) || 'A' <= c && c <= 'Z':
			if err := p.typeName(); err != nil {
				return nil, err
			}
		}
	}
	if p.i == argStart {
		return nil, p.notInProfile(argStart, ":not() needs one simple selector")
	}
	if p.i >= len(p.s) || p.s[p.i] != ')' {
		return nil, p.notInProfile(p.i, ":not() takes exactly one simple selector (no list, compound or combinator)")
	}
	p.i++
	return attrs, nil
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}
