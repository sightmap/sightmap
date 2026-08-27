package webmcp

// Ordered JSON values and a serializer that reproduces JavaScript's
// JSON.stringify(v, null, 2) byte for byte. The generated bundle embeds the
// compiled IR as JSON, and the Go and Node generators must emit identical
// files (the golden tests byte-compare them), so this package controls key
// order and escaping itself instead of using encoding/json — which sorts map
// keys, HTML-escapes, and encodes U+2028/U+2029 differently from JS.

import (
	"fmt"
	"strconv"
	"strings"
)

// OM is an insertion-ordered string-keyed map, mirroring a JS object literal.
type OM struct {
	keys []string
	m    map[string]any
}

func NewOM() *OM {
	return &OM{m: map[string]any{}}
}

// Set appends the key on first write and overwrites in place after.
func (o *OM) Set(key string, v any) *OM {
	if _, ok := o.m[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.m[key] = v
	return o
}

func (o *OM) Get(key string) (any, bool) {
	v, ok := o.m[key]
	return v, ok
}

func (o *OM) Has(key string) bool {
	_, ok := o.m[key]
	return ok
}

func (o *OM) Keys() []string { return o.keys }

func (o *OM) Len() int { return len(o.keys) }

// StringifyJSON matches JSON.stringify(v, null, 2) for the value types the
// compiler produces: nil, bool, int, float64, string, []any, *OM.
func StringifyJSON(v any) string {
	var b strings.Builder
	writeJSON(&b, v, 0)
	return b.String()
}

func writeJSON(b *strings.Builder, v any, depth int) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		b.WriteString(strconv.Itoa(t))
	case int64:
		b.WriteString(strconv.FormatInt(t, 10))
	case float64:
		// JS renders integral floats without a decimal point.
		if t == float64(int64(t)) {
			b.WriteString(strconv.FormatInt(int64(t), 10))
		} else {
			b.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
		}
	case string:
		writeJSONString(b, t)
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, item := range t {
			b.WriteString(strings.Repeat("  ", depth+1))
			writeJSON(b, item, depth+1)
			if i < len(t)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString("]")
	case *OM:
		if t == nil || t.Len() == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range t.keys {
			b.WriteString(strings.Repeat("  ", depth+1))
			writeJSONString(b, k)
			b.WriteString(": ")
			writeJSON(b, t.m[k], depth+1)
			if i < len(t.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString("}")
	default:
		panic(fmt.Sprintf("StringifyJSON: unsupported type %T", v))
	}
}

// writeJSONString matches JSON.stringify's string escaping: the two-character
// escapes for \" \\ \b \f \n \r \t, \u00XX for other control characters, and
// everything else — U+2028/U+2029 included — literal.
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
