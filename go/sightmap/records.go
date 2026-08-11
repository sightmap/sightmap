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
}

// Request is one observed network request/response. Bodies are not included —
// they are large and, for a live capture, fetched lazily by the tool that
// observed the request. Status is 0 until the response is seen.
type Request struct {
	Index        int    `json:"index"`
	Tab          string `json:"tab"`
	Method       string `json:"method"`
	URL          string `json:"url"`
	Status       int    `json:"status"`
	StatusText   string `json:"statusText"`
	ResourceType string `json:"resourceType"`
	Ts           int64  `json:"ts"` // unix milliseconds
}
