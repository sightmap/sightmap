package device

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sightmap/sightmap/go/sightmap"
)

// CFNetwork logs to the unified log under subsystem com.apple.CFNetwork. Two
// of its categories carry requests: HAR, a full HAR entry per transaction,
// which it writes only while HAR capture is switched on and only for a process
// with the get-task-allow entitlement; and Summary, one line per task with its
// status and timing but not its URL. Launched with CFNETWORK_DIAGNOSTICS=1,
// the app also logs Diagnostics blocks that name each request's method and
// host (the path is redacted).

// cfLine is one `log stream --style ndjson` line from CFNetwork.
type cfLine struct {
	EventType    string `json:"eventType"`
	Category     string `json:"category"`
	EventMessage string `json:"eventMessage"`
	Timestamp    string `json:"timestamp"`
	ProcessID    int    `json:"processID"`
}

func parseCFLine(line string) (cfLine, bool) {
	var l cfLine
	if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &l) != nil || l.EventType != "logEvent" {
		return cfLine{}, false
	}
	return l, true
}

// logTime converts a `log stream` timestamp ("2026-10-29 12:13:05.652955-0400")
// to RFC 3339 in UTC.
func logTime(ts string) string {
	t, err := time.Parse("2006-01-02 15:04:05.999999-0700", ts)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harEntry struct {
	TaskUUID        string  `json:"_taskUUID"`
	StartedDateTime string  `json:"startedDateTime"`
	Time            float64 `json:"time"`
	Request         struct {
		Method      string      `json:"method"`
		URL         string      `json:"url"`
		HTTPVersion string      `json:"httpVersion"`
		Headers     []harHeader `json:"headers"`
		BodySize    int64       `json:"bodySize"`
		PostData    *struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"postData"`
	} `json:"request"`
	Response struct {
		Status      int         `json:"status"`
		StatusText  string      `json:"statusText"`
		HTTPVersion string      `json:"httpVersion"`
		Headers     []harHeader `json:"headers"`
		BodySize    int64       `json:"bodySize"`
		Content     struct {
			Size     int64  `json:"size"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
}

func headers(hs []harHeader) []sightmap.Header {
	if len(hs) == 0 {
		return nil
	}
	out := make([]sightmap.Header, len(hs))
	for i, h := range hs {
		out[i] = sightmap.Header{Name: h.Name, Value: h.Value}
	}
	return out
}

// bodyText decodes a base64 HAR body; a binary one keeps only its size.
func bodyText(text, encoding string) string {
	if encoding != "base64" {
		return text
	}
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil || !utf8.Valid(data) {
		return ""
	}
	return string(data)
}

// parseHAR turns a HAR log line's message into an entry. HAR's queryString is
// always empty here; the query is in the URL.
func parseHAR(l cfLine) (NetworkEntry, bool) {
	var h harEntry
	if json.Unmarshal([]byte(l.EventMessage), &h) != nil || h.Request.URL == "" {
		return NetworkEntry{}, false
	}
	e := NetworkEntry{
		Source: SourceHAR, Method: h.Request.Method, URL: h.Request.URL, Status: h.Response.Status, StatusText: h.Response.StatusText,
		DurationMs: int64(h.Time), Protocol: h.Response.HTTPVersion, MimeType: h.Response.Content.MimeType,
		RequestBytes: h.Request.BodySize, ResponseBytes: h.Response.Content.Size, PID: l.ProcessID, TaskID: h.TaskUUID,
		ReqHeaders: headers(h.Request.Headers), RspHeaders: headers(h.Response.Headers),
	}
	if t, err := time.Parse(time.RFC3339Nano, h.StartedDateTime); err == nil {
		e.Time = t.UTC().Format(time.RFC3339Nano)
	} else {
		e.Time = logTime(l.Timestamp)
	}
	if p := h.Request.PostData; p != nil {
		e.ReqBody = newBody(bodyText(p.Text, p.Encoding), p.MimeType, int(h.Request.BodySize))
	}
	c := h.Response.Content
	e.RspBody = newBody(bodyText(c.Text, c.Encoding), c.MimeType, int(c.Size))
	return e, true
}

var (
	summaryLine = regexp.MustCompile(`^Task <([0-9A-Fa-f-]+)>\.<\d+> summary for (?:task )?(\w+) \{(.*)\}`)
	summaryKV   = regexp.MustCompile(`(\w+)=("[^"]*"|[^,]*)`)
	diagReceive = regexp.MustCompile(`Protocol Received: request (\w+) (\w+://[^/\s]+)/\S*\s*\n\s*Response: HTTP/[\d.]+ (\d+)`)
)

// parseSummary turns a Summary log line into an entry with no URL.
func parseSummary(l cfLine) (NetworkEntry, bool) {
	m := summaryLine.FindStringSubmatch(l.EventMessage)
	if m == nil {
		return NetworkEntry{}, false
	}
	kv := map[string]string{}
	for _, p := range summaryKV.FindAllStringSubmatch(m[3], -1) {
		kv[p[1]] = strings.Trim(p[2], `"`)
	}
	num := func(k string) int64 { n, _ := strconv.ParseInt(kv[k], 10, 64); return n }
	e := NetworkEntry{Source: SourceSummary, TaskID: m[1], Status: int(num("response_status")), DurationMs: num("transaction_duration_ms"),
		Protocol: kv["protocol"], RequestBytes: num("request_bytes"), ResponseBytes: num("response_bytes"), PID: l.ProcessID, Time: logTime(l.Timestamp)}
	if m[2] != "success" {
		e.Error = m[2]
	}
	return e, true
}

// diagRequest is a Diagnostics "Protocol Received" block: a request's method,
// origin and status, with no id to tie it to its Summary line.
type diagRequest struct {
	method, origin string
	status, pid    int
	at             time.Time
}

func parseDiagnostics(l cfLine) (diagRequest, bool) {
	m := diagReceive.FindStringSubmatch(l.EventMessage)
	if m == nil {
		return diagRequest{}, false
	}
	status, _ := strconv.Atoi(m[3])
	return diagRequest{method: m[1], origin: m[2], status: status, pid: l.ProcessID, at: time.Now()}, true
}

// diagQueue pairs Summary lines with Diagnostics blocks: the oldest unpaired
// block from the same process with the same status. Concurrent requests with
// the same status can swap hosts; that is the best the redacted log allows.
type diagQueue struct{ pending []diagRequest }

func (q *diagQueue) add(d diagRequest) {
	cutoff := time.Now().Add(-time.Minute)
	kept := q.pending[:0]
	for _, p := range q.pending {
		if p.at.After(cutoff) {
			kept = append(kept, p)
		}
	}
	q.pending = append(kept, d)
}

func (q *diagQueue) fill(e *NetworkEntry) {
	for i, p := range q.pending {
		if p.pid == e.PID && p.status == e.Status {
			e.Method, e.URL = p.method, p.origin
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return
		}
	}
}
