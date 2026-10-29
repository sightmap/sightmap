package device

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Network sources, from most to least detailed.
const (
	SourceHAR     = "har"
	SourceRNCDP   = "rn-cdp"
	SourceSummary = "summary"
)

// maxBodyBytes caps each stored request and response body.
const maxBodyBytes = 256 << 10

// NetworkEntry is one request the app made, with whatever its source saw.
// A summary entry has no path, headers or bodies: CFNetwork redacts them.
type NetworkEntry struct {
	Index         int               `json:"index"`
	Time          string            `json:"time"`
	Source        string            `json:"source"`
	Method        string            `json:"method,omitempty"`
	URL           string            `json:"url,omitempty"`
	Status        int               `json:"status,omitempty"`
	StatusText    string            `json:"statusText,omitempty"`
	DurationMs    int64             `json:"durationMs,omitempty"`
	Protocol      string            `json:"protocol,omitempty"`
	MimeType      string            `json:"mimeType,omitempty"`
	RequestBytes  int64             `json:"requestBytes,omitempty"`
	ResponseBytes int64             `json:"responseBytes,omitempty"`
	Error         string            `json:"error,omitempty"`
	PID           int               `json:"pid,omitempty"`
	TaskID        string            `json:"taskId,omitempty"`
	ReqHeaders    []sightmap.Header `json:"reqHeaders,omitempty"`
	RspHeaders    []sightmap.Header `json:"rspHeaders,omitempty"`
	ReqBody       *sightmap.Body    `json:"reqBody,omitempty"`
	RspBody       *sightmap.Body    `json:"rspBody,omitempty"`
	// Matches and Props are the corpus requests: that classify the entry and
	// the properties they extract; filled in when the entry is read.
	Matches []string                 `json:"matches,omitempty"`
	Props   []sightmap.PropertyValue `json:"props,omitempty"`
}

func (e NetworkEntry) record() sightmap.Request {
	return sightmap.Request{Index: e.Index, Method: e.Method, URL: e.URL, Status: e.Status, StatusText: e.StatusText, DurationMs: e.DurationMs,
		ReqHeaders: e.ReqHeaders, RspHeaders: e.RspHeaders, ReqBody: e.ReqBody, RspBody: e.RspBody}
}

// Annotate fills Matches and Props from the corpus's requests:. A summary
// entry has no path to match a route against, so it never matches.
func (e NetworkEntry) Annotate(c *sightmap.Corpus) NetworkEntry {
	if c == nil || e.Source == SourceSummary || e.URL == "" {
		return e
	}
	for _, m := range c.RequestsForRecord(e.record()) {
		e.Matches = append(e.Matches, m.Name)
		e.Props = append(e.Props, m.Properties...)
	}
	return e
}

func newBody(content, contentType string, size int) *sightmap.Body {
	if content == "" && size == 0 {
		return nil
	}
	b := &sightmap.Body{Content: content, ContentType: contentType, Size: max(size, len(content))}
	if len(b.Content) > maxBodyBytes {
		b.Content, b.Truncated = b.Content[:maxBodyBytes], true
	}
	return b
}

// Network is a bounded buffer of network entries with stable, ever-increasing
// indexes, like Console.
type Network struct {
	Capacity int

	mu      sync.Mutex
	entries []NetworkEntry
	next    int
}

// NewNetwork returns a Network holding the most recent capacity entries (at
// least one).
func NewNetwork(capacity int) *Network { return &Network{Capacity: max(capacity, 1)} }

// Push appends e, assigning its index (and time, when it has none).
func (n *Network) Push(e NetworkEntry) NetworkEntry {
	n.mu.Lock()
	defer n.mu.Unlock()
	e.Index = n.next
	n.next++
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	n.entries = append(n.entries, e)
	if len(n.entries) > n.Capacity {
		n.entries = n.entries[len(n.entries)-n.Capacity:]
	}
	return e
}

// Len is how many entries have ever been pushed.
func (n *Network) Len() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.next
}

// CountSource is how many held entries came from source.
func (n *Network) CountSource(source string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	count := 0
	for _, e := range n.entries {
		if e.Source == source {
			count++
		}
	}
	return count
}

// NetworkFilter selects entries for List.
type NetworkFilter struct {
	Limit int `json:"limit"`
	// Grep is a case-insensitive substring of the method, URL, status, source
	// or a matched request name.
	Grep   string `json:"grep"`
	Source string `json:"source"`
	// Since, as in ConsoleFilter, pages forward from an index.
	Since      *int `json:"since,omitempty"`
	Unredacted bool `json:"unredacted,omitempty"`
}

// NetworkList is List's result. Its entries carry headers but no bodies;
// Get returns those.
type NetworkList struct {
	Total    int            `json:"total"`
	Entries  []NetworkEntry `json:"entries"`
	Last     int            `json:"last"`
	Dropped  int            `json:"dropped,omitempty"`
	Capacity int            `json:"capacity"`
}

// List returns the entries f selects, oldest first, each passed through view
// (redaction and corpus matching) before the filter sees it.
func (n *Network) List(f NetworkFilter, view func(NetworkEntry) NetworkEntry) NetworkList {
	n.mu.Lock()
	held := append([]NetworkEntry(nil), n.entries...)
	next := n.next
	n.mu.Unlock()
	needle := strings.ToLower(f.Grep)
	first := next - len(held)
	out := NetworkList{Last: next - 1, Capacity: n.Capacity, Dropped: first}
	if f.Since != nil {
		out.Dropped = max(0, first-(*f.Since+1))
	}
	var hits []NetworkEntry
	for _, e := range held {
		if f.Since != nil && e.Index <= *f.Since {
			continue
		}
		if f.Source != "" && e.Source != f.Source {
			continue
		}
		e = view(e)
		if needle != "" && !strings.Contains(strings.ToLower(e.searchText()), needle) {
			continue
		}
		e.ReqBody, e.RspBody = nil, nil
		hits = append(hits, e)
	}
	out.Total, out.Entries = len(hits), hits
	switch {
	case f.Limit <= 0 || len(hits) <= f.Limit:
	case f.Since != nil:
		out.Entries = hits[:f.Limit]
		out.Last = out.Entries[f.Limit-1].Index
	default:
		out.Entries = hits[len(hits)-f.Limit:]
	}
	if out.Entries == nil {
		out.Entries = []NetworkEntry{}
	}
	return out
}

func (e NetworkEntry) searchText() string {
	return strings.Join(append([]string{e.Method, e.URL, strconv.Itoa(e.Status), e.Source, e.Error}, e.Matches...), " ")
}

// Get returns the entry with index i.
func (n *Network) Get(i int) (NetworkEntry, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, e := range n.entries {
		if e.Index == i {
			return e, true
		}
	}
	return NetworkEntry{}, false
}

// Redacted is the placeholder for a value Redact hides.
const Redacted = "<redacted>"

var sensitiveWord = regexp.MustCompile(`(?i)token|password|secret|key`)

// SensitiveName reports whether a header, query parameter or JSON key holds a
// credential: Authorization, Cookie, Set-Cookie, Proxy-Authorization, or any
// name containing token, password, secret or key.
func SensitiveName(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "cookie", "set-cookie", "proxy-authorization":
		return true
	}
	return sensitiveWord.MatchString(name)
}

// Redact hides credentials in e's URL query, headers and bodies.
func (e NetworkEntry) Redact() NetworkEntry {
	e.URL = redactURL(e.URL)
	e.ReqHeaders, e.RspHeaders = redactHeaders(e.ReqHeaders), redactHeaders(e.RspHeaders)
	e.ReqBody, e.RspBody = redactBody(e.ReqBody), redactBody(e.RspBody)
	return e
}

func redactHeaders(hs []sightmap.Header) []sightmap.Header {
	if hs == nil {
		return nil
	}
	out := make([]sightmap.Header, len(hs))
	for i, h := range hs {
		if SensitiveName(h.Name) {
			h.Value = Redacted
		}
		out[i] = h
	}
	return out
}

// redactPairs hides the sensitive values of a&b=c pairs, keeping their order
// and encoding.
func redactPairs(query string) string {
	pairs := strings.Split(query, "&")
	for i, p := range pairs {
		name, _, hasValue := strings.Cut(p, "=")
		if hasValue && SensitiveName(name) {
			pairs[i] = name + "=" + Redacted
		}
	}
	return strings.Join(pairs, "&")
}

func redactURL(u string) string {
	base, query, ok := strings.Cut(u, "?")
	if !ok {
		return u
	}
	query, fragment, hasFragment := strings.Cut(query, "#")
	out := base + "?" + redactPairs(query)
	if hasFragment {
		out += "#" + fragment
	}
	return out
}

// jsonPair matches a JSON object member whose value is a scalar. It works on
// text rather than a parse so it keeps the body's formatting and key order and
// copes with a truncated body.
var jsonPair = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"(\s*:\s*)("(?:[^"\\]|\\.)*"|-?[0-9][0-9.eE+-]*|true|false|null)`)

func redactBody(b *sightmap.Body) *sightmap.Body {
	if b == nil || b.Content == "" {
		return b
	}
	c := *b
	if strings.Contains(strings.ToLower(c.ContentType), "x-www-form-urlencoded") {
		c.Content = redactPairs(c.Content)
		return &c
	}
	c.Content = jsonPair.ReplaceAllStringFunc(c.Content, func(m string) string {
		sub := jsonPair.FindStringSubmatch(m)
		if !SensitiveName(sub[1]) {
			return m
		}
		return `"` + sub[1] + `"` + sub[2] + `"` + Redacted + `"`
	})
	return &c
}
