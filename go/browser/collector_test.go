package browser

import (
	"encoding/json"
	"testing"
)

func TestParseConsoleAPI(t *testing.T) {
	raw := json.RawMessage(`{"type":"warning","timestamp":1785015495306,"args":[{"type":"string","value":"hello"},{"type":"number","value":42}]}`)
	e, ok := parseConsoleAPI(raw)
	if !ok {
		t.Fatal("parseConsoleAPI returned ok=false")
	}
	if e.Level != "warn" {
		t.Errorf("level = %q, want warn", e.Level)
	}
	if e.Text != "hello 42" {
		t.Errorf("text = %q, want \"hello 42\"", e.Text)
	}
	if e.Ts != 1785015495306 {
		t.Errorf("ts = %d, want 1785015495306", e.Ts)
	}
}

func TestParseException(t *testing.T) {
	raw := json.RawMessage(`{"timestamp":1785015495306,"exceptionDetails":{"text":"Uncaught","exception":{"description":"TypeError: x is not a function"}}}`)
	e, ok := parseException(raw)
	if !ok {
		t.Fatal("parseException returned ok=false")
	}
	if e.Level != "exception" {
		t.Errorf("level = %q, want exception", e.Level)
	}
	if e.Text != "TypeError: x is not a function" {
		t.Errorf("text = %q", e.Text)
	}
}

func TestParseRequestResponse(t *testing.T) {
	req := json.RawMessage(`{"requestId":"R1","type":"XHR","request":{"url":"https://x/api","method":"POST"}}`)
	e, ok := parseRequest(req)
	if !ok {
		t.Fatal("parseRequest ok=false")
	}
	if e.Method != "POST" || e.URL != "https://x/api" || e.ResourceType != "XHR" || e.requestID != "R1" {
		t.Errorf("request parse = %+v", e)
	}

	resp := json.RawMessage(`{"requestId":"R1","type":"XHR","response":{"status":201,"statusText":"Created"}}`)
	id, st, stText, rtype, ok := parseResponse(resp)
	if !ok || id != "R1" || st != 201 || stText != "Created" || rtype != "XHR" {
		t.Errorf("response parse = %s %d %s %s %v", id, st, stText, rtype, ok)
	}
}

func TestRingOverflowAndDropped(t *testing.T) {
	c := NewCollector("localhost:0")
	c.maxEntries = 3
	for i := 0; i < 5; i++ {
		c.addConsole(ConsoleEntry{Level: "log", Text: "m"})
	}
	got, dropped := c.Console(ConsoleFilter{})
	if len(got) != 3 {
		t.Fatalf("buffered = %d, want 3 (capped)", len(got))
	}
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2", dropped)
	}
	// Indexes are monotonic and reflect the retained tail (2,3,4).
	if got[0].Index != 2 || got[2].Index != 4 {
		t.Errorf("indexes = %d..%d, want 2..4", got[0].Index, got[2].Index)
	}
}

func TestConsoleFilterAndLimit(t *testing.T) {
	c := NewCollector("localhost:0")
	c.addConsole(ConsoleEntry{Level: "log", Text: "a", Tab: "T1"})
	c.addConsole(ConsoleEntry{Level: "error", Text: "b", Tab: "T1"})
	c.addConsole(ConsoleEntry{Level: "error", Text: "c", Tab: "T2"})

	errs, _ := c.Console(ConsoleFilter{Level: "error"})
	if len(errs) != 2 {
		t.Fatalf("level=error → %d, want 2", len(errs))
	}
	t2, _ := c.Console(ConsoleFilter{Tab: "T2"})
	if len(t2) != 1 || t2[0].Text != "c" {
		t.Errorf("tab=T2 → %+v", t2)
	}
	last, _ := c.Console(ConsoleFilter{Limit: 1})
	if len(last) != 1 || last[0].Text != "c" {
		t.Errorf("limit=1 → %+v (want most recent)", last)
	}
}

func TestNetworkFilterAndResponseMatch(t *testing.T) {
	c := NewCollector("localhost:0")
	c.addNetwork(NetworkEntry{Method: "GET", URL: "https://x/style.css", ResourceType: "Stylesheet", requestID: "A"})
	c.addNetwork(NetworkEntry{Method: "POST", URL: "https://x/api/cart", ResourceType: "XHR", requestID: "B"})
	c.applyResponse("B", 500, "Server Error", "XHR")

	xhr, _ := c.Network(NetworkFilter{ResourceType: "xhr"})
	if len(xhr) != 1 || xhr[0].Status != 500 || xhr[0].StatusText != "Server Error" {
		t.Fatalf("xhr filter/response = %+v", xhr)
	}
	byURL, _ := c.Network(NetworkFilter{URLSubstr: "CART"})
	if len(byURL) != 1 || byURL[0].requestID != "B" {
		t.Errorf("url substring (case-insensitive) = %+v", byURL)
	}
}

func TestDecodeBody(t *testing.T) {
	plain, err := decodeBody(json.RawMessage(`{"body":"hello","base64Encoded":false}`))
	if err != nil || string(plain) != "hello" {
		t.Errorf("plain body = %q, %v", plain, err)
	}
	enc, err := decodeBody(json.RawMessage(`{"body":"aGk=","base64Encoded":true}`))
	if err != nil || string(enc) != "hi" {
		t.Errorf("base64 body = %q, %v", enc, err)
	}
}
