package sightmap

// Observed runtime records. These are the tool-free, wire-friendly counterparts
// of the corpus definitions: a Message is what a MessageDef classifies, a
// Request is what a RequestDef classifies. They carry only facts an observer
// captured — no live connection or fetch handle — so any consumer (not just the
// CDP browser tool) can produce and match them without depending on a driver.

// Message is one observed console/exception record. Level is the six-level
// vocabulary the reference capture emits (log, debug, info, warn, error,
// exception); an uncaught exception arrives as level "exception". Index is a
// monotonic per-source id, the stable handle for get-by-index.
type Message struct {
	Index int    `json:"index"`
	Tab   string `json:"tab"`
	Level string `json:"level"`
	Text  string `json:"text"`
	Ts    int64  `json:"ts"` // unix milliseconds

	// Stack is the call stack of an uncaught exception, throwing frame first
	// (frame 0 == "top"). Empty for a plain console record. Optional and
	// omitempty, like the Request payload fields: a producer that didn't capture
	// frames leaves it nil and stack extraction degrades to "absent".
	Stack []Frame `json:"stack,omitempty"`
}

// Frame is one call-stack frame of an exception Message, as the reference
// capture derives it from CDP Runtime.exceptionThrown stackTrace.callFrames.
// Line and Column are 0 when the capture didn't supply them.
type Frame struct {
	Function string `json:"function,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

// Request is one observed network request/response. Status is 0 until the
// response is seen.
//
// Everything a RequestDef's properties[] can address (the SEP-0005 sources
// req.body / rsp.body / req.headers / rsp.headers, plus the reserved identity
// name duration) is carried here, so the record is faithful to what the def can
// extract. But every such field is optional: a lazy or streaming producer may
// leave them nil/empty — bodies are large and are often fetched on demand — and
// property extraction then degrades to "value absent" (silent omission, per the
// SEP) rather than erroring. A producer holding a full capture populates them.
type Request struct {
	Index        int    `json:"index"`
	Tab          string `json:"tab"`
	Method       string `json:"method"`
	URL          string `json:"url"`
	Status       int    `json:"status"`
	StatusText   string `json:"statusText"`
	ResourceType string `json:"resourceType"`
	Ts           int64  `json:"ts"` // unix milliseconds

	// Optional, fetch-on-demand payload material — the targets of properties[]
	// extraction. Omitted from the wire when unpopulated.
	DurationMs int64    `json:"durationMs,omitempty"` // request→response latency; the reserved "duration" identity
	ReqHeaders []Header `json:"reqHeaders,omitempty"` // request headers, ordered, duplicate-preserving
	RspHeaders []Header `json:"rspHeaders,omitempty"` // response headers, ordered, duplicate-preserving
	ReqBody    *Body    `json:"reqBody,omitempty"`    // request body (raw)
	RspBody    *Body    `json:"rspBody,omitempty"`    // response body (raw)
}

// Header is one observed HTTP header. Headers are stored as an ordered,
// duplicate-preserving list rather than a map so repeated headers (Set-Cookie,
// and the like) survive; a properties[] lookup by name is case-insensitive.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Body is an observed request or response body. Content is the raw text; the
// matcher parses it (a properties[] field dot-path implies JSON at match time) —
// parsing is deliberately kept out of the record. Truncated and Size let a
// matcher tell "absent" from "absent because the capture truncated it".
type Body struct {
	Content     string `json:"content,omitempty"`
	Size        int    `json:"size,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}
