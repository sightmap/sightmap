package sightmap

import (
	"strconv"
	"strings"
)

// ExtractProperties resolves d's stack-addressing properties[] against the
// observed message rec, returning the values that resolved, in declaration
// order. A property is silently omitted (never an error) when rec has no stack,
// the addressed frame or attribute doesn't resolve, or a pattern finds no
// match — the same live-traffic contract as request-property extraction: a plain
// console record (empty Stack) simply yields nothing.
//
// Resolution is source → field → pattern. The only source is
// "stack"; field addresses a frame and attribute ("top.file", "1.function"),
// and pattern optionally refines the resolved string (capture group 1 else the
// whole match).
func (d *MessageDef) ExtractProperties(rec Message) []PropertyValue {
	if len(d.Properties) == 0 {
		return nil
	}
	var out []PropertyValue
	for _, p := range d.Properties {
		if v, ok := resolveMessageProperty(p, rec); ok {
			out = append(out, PropertyValue{Name: p.Name, Value: v})
		}
	}
	return out
}

// resolveMessageProperty applies one property definition to rec.
func resolveMessageProperty(p MessagePropertyDef, rec Message) (string, bool) {
	var raw string
	var ok bool
	switch p.Source {
	case "stack":
		raw, ok = resolveStackField(rec.Stack, p.Field)
	default:
		return "", false
	}
	if !ok {
		return "", false
	}
	if p.Pattern != "" {
		matched, ok := applyPattern(p.Pattern, raw)
		if !ok {
			return "", false
		}
		raw = matched
	}
	if raw == "" {
		return "", false
	}
	return raw, true
}

// resolveStackField resolves a "<frame>.<attribute>" field against a call stack.
// <frame> is "top" (alias for index 0) or a non-negative integer index;
// <attribute> is one of function/file/line/column. Returns false when the field
// is malformed, the frame index is out of range, or the attribute is unknown —
// so a bad field omits silently rather than erroring, per the SEP.
func resolveStackField(frames []Frame, field string) (string, bool) {
	sel, attr, found := strings.Cut(field, ".")
	if !found {
		return "", false
	}
	idx := 0
	if sel != "top" {
		n, err := strconv.Atoi(sel)
		if err != nil || n < 0 {
			return "", false
		}
		idx = n
	}
	if idx >= len(frames) {
		return "", false
	}
	f := frames[idx]
	switch attr {
	case "function":
		return f.Function, f.Function != ""
	case "file":
		return f.File, f.File != ""
	case "line":
		if f.Line == nil {
			return "", false
		}
		return strconv.Itoa(*f.Line), true
	case "column":
		if f.Column == nil {
			return "", false
		}
		return strconv.Itoa(*f.Column), true
	default:
		return "", false
	}
}
