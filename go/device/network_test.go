package device

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sightmap/sightmap/go/sightmap"
)

// The fixtures under testdata/network are recorded from the sample app and a
// React Native app on a simulator, with user paths and hosts anonymized.

func fixtureLines(t *testing.T, name string) []cfLine {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "network", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []cfLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if l, ok := parseCFLine(sc.Text()); ok {
			lines = append(lines, l)
		}
	}
	return lines
}

func harFixture(t *testing.T) NetworkEntry {
	t.Helper()
	for _, l := range fixtureLines(t, "har.ndjson") {
		if l.Category == "HAR" {
			e, ok := parseHAR(l)
			if !ok {
				t.Fatal("the HAR line does not parse")
			}
			return e
		}
	}
	t.Fatal("no HAR line in the fixture")
	return NetworkEntry{}
}

func sampleCorpus(t *testing.T) *sightmap.Corpus {
	t.Helper()
	c, err := sightmap.Load(filepath.Join("testdata", "SampleApp", ".sightmap"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseHAR(t *testing.T) {
	e := harFixture(t)
	if e.Source != SourceHAR || e.Method != "POST" || e.URL != "http://127.0.0.1:18765/anything/sightmap/order-2" || e.Status != 201 {
		t.Fatalf("entry = %+v", e)
	}
	if e.TaskID != "96FB5440-96FD-4124-A5E4-A608A32C3F9D" || e.PID != 66680 || !strings.HasSuffix(e.Time, "Z") {
		t.Errorf("identity = %q pid %d time %q", e.TaskID, e.PID, e.Time)
	}
	if e.ReqBody == nil || !strings.Contains(e.ReqBody.Content, `"order":2`) || e.ReqBody.ContentType != "application/json" {
		t.Errorf("request body = %+v", e.ReqBody)
	}
	if e.RspBody == nil || !strings.Contains(e.RspBody.Content, `"json"`) {
		t.Errorf("response body = %+v", e.RspBody)
	}
	if headerValue(e.ReqHeaders, "authorization") != "Bearer sample-not-a-secret" || headerValue(e.RspHeaders, "set-cookie") != "session=sample" {
		t.Errorf("headers = %v / %v", e.ReqHeaders, e.RspHeaders)
	}
}

func TestParseSummaryJoinsTheDiagnosticsHost(t *testing.T) {
	var q diagQueue
	var got []NetworkEntry
	for _, l := range fixtureLines(t, "summary.ndjson") {
		switch l.Category {
		case "Diagnostics":
			if d, ok := parseDiagnostics(l); ok {
				q.add(d)
			}
		case "Summary":
			e, ok := parseSummary(l)
			if !ok {
				t.Fatalf("summary line does not parse: %s", l.EventMessage)
			}
			q.fill(&e)
			got = append(got, e)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want one summary entry, got %d", len(got))
	}
	e := got[0]
	if e.Method != "POST" || e.URL != "http://127.0.0.1" || e.Status != 201 || e.DurationMs != 20 || e.Protocol != "http/1.1" || e.Error != "" {
		t.Errorf("entry = %+v", e)
	}
	if e.Annotate(sampleCorpus(t)).Matches != nil {
		t.Error("a summary entry has no path, so it must not match a route")
	}
}

func newTestCollector(armed bool) *NetCollector {
	c := &NetCollector{Mode: NetworkAuto, Network: NewNetwork(100), Log: func(string) {}, harArmed: armed, harTasks: map[string]bool{},
		states: map[string]*NetSourceStatus{}}
	for _, name := range []string{SourceHAR, SourceRNCDP, SourceSummary} {
		c.states[name] = &NetSourceStatus{Name: name, State: stateActive}
	}
	return c
}

func TestSummaryDefersToHARForTheSameTask(t *testing.T) {
	defer func(d time.Duration) { summaryHold = d }(summaryHold)
	summaryHold = 20 * time.Millisecond
	c := newTestCollector(true)
	for _, l := range fixtureLines(t, "har.ndjson") {
		c.handleLine(l) // the Summary line comes first, as recorded
	}
	time.Sleep(100 * time.Millisecond)
	l := c.Network.List(NetworkFilter{}, func(e NetworkEntry) NetworkEntry { return e })
	if l.Total != 1 || l.Entries[0].Source != SourceHAR {
		t.Fatalf("want only the HAR entry, got %+v", l.Entries)
	}

	c = newTestCollector(true)
	for _, l := range fixtureLines(t, "summary.ndjson") {
		c.handleLine(l)
	}
	time.Sleep(100 * time.Millisecond)
	st := c.Status()
	if c.Network.Len() != 1 || st.Sources[0].Entries != 0 || !strings.Contains(st.Sources[0].Detail, "no HAR entry for the 1 request") {
		t.Fatalf("a summary HAR never claimed is listed and reported: %d entries, %+v", c.Network.Len(), st.Sources[0])
	}
}

func TestRedact(t *testing.T) {
	e := NetworkEntry{
		URL:        "https://api.example.test/v1/items?q=shoes&secret_token=abc&apiKey=k#frag",
		ReqHeaders: []sightmap.Header{{Name: "Authorization", Value: "Bearer x"}, {Name: "X-Api-Key", Value: "k"}, {Name: "Accept", Value: "*/*"}},
		RspHeaders: []sightmap.Header{{Name: "Set-Cookie", Value: "s=1"}, {Name: "Content-Type", Value: "application/json"}},
		ReqBody:    &sightmap.Body{Content: `{"user":{"password": "hunter2","name":"Ada"},"refreshToken":"r","count":3}`, ContentType: "application/json"},
		RspBody:    &sightmap.Body{Content: `{"access_token":"t","n":1,"pin":12, "secret": 42, "tail": "cut off`, Truncated: true},
	}
	r := e.Redact()
	if r.URL != "https://api.example.test/v1/items?q=shoes&secret_token=<redacted>&apiKey=<redacted>#frag" {
		t.Errorf("url = %s", r.URL)
	}
	if headerValue(r.ReqHeaders, "authorization") != Redacted || headerValue(r.ReqHeaders, "x-api-key") != Redacted || headerValue(r.ReqHeaders, "accept") != "*/*" {
		t.Errorf("request headers = %v", r.ReqHeaders)
	}
	if headerValue(r.RspHeaders, "set-cookie") != Redacted || headerValue(r.RspHeaders, "content-type") != "application/json" {
		t.Errorf("response headers = %v", r.RspHeaders)
	}
	if want := `{"user":{"password": "<redacted>","name":"Ada"},"refreshToken":"<redacted>","count":3}`; r.ReqBody.Content != want {
		t.Errorf("request body = %s", r.ReqBody.Content)
	}
	if want := `{"access_token":"<redacted>","n":1,"pin":12, "secret": "<redacted>", "tail": "cut off`; r.RspBody.Content != want {
		t.Errorf("response body = %s", r.RspBody.Content)
	}
	if e.ReqHeaders[0].Value != "Bearer x" || !strings.Contains(e.ReqBody.Content, "hunter2") {
		t.Error("Redact changed the stored entry")
	}
	form := redactBody(&sightmap.Body{Content: "user=ada&password=p", ContentType: "application/x-www-form-urlencoded"})
	if form.Content != "user=ada&password=<redacted>" {
		t.Errorf("form body = %s", form.Content)
	}
}

func TestAnnotateMatchesRequestsAndExtractsFromTheRedactedEntry(t *testing.T) {
	e := harFixture(t).Redact().Annotate(sampleCorpus(t))
	if len(e.Matches) != 1 || e.Matches[0] != "CreateOrder" {
		t.Fatalf("matches = %v", e.Matches)
	}
	props := map[string]string{}
	for _, p := range e.Props {
		props[p.Name] = p.Value
	}
	if props["order"] != "2" || props["echoed"] != "2" {
		t.Errorf("props = %v", e.Props)
	}
}

func TestNetworkListFiltersAndLeavesOutBodies(t *testing.T) {
	n := NewNetwork(3)
	n.Push(NetworkEntry{Source: SourceSummary, Status: 200})
	for i := range 3 {
		n.Push(NetworkEntry{Source: SourceHAR, Method: "GET", URL: "https://a.test/items/" + string(rune('a'+i)), Status: 200, RspBody: &sightmap.Body{Content: "x"}})
	}
	all := n.List(NetworkFilter{}, func(e NetworkEntry) NetworkEntry { return e })
	if all.Total != 3 || all.Dropped != 1 || all.Entries[0].RspBody != nil {
		t.Fatalf("list = %+v", all)
	}
	since := 1
	paged := n.List(NetworkFilter{Since: &since, Limit: 1}, func(e NetworkEntry) NetworkEntry { return e })
	if len(paged.Entries) != 1 || paged.Entries[0].Index != 2 || paged.Last != 2 || paged.Total != 2 {
		t.Errorf("paged = %+v", paged)
	}
	named := n.List(NetworkFilter{Grep: "getitem"}, func(e NetworkEntry) NetworkEntry {
		if strings.HasSuffix(e.URL, "/c") {
			e.Matches = []string{"GetItem"}
		}
		return e
	})
	if named.Total != 1 || named.Entries[0].Index != 3 {
		t.Errorf("grep on a match name = %+v", named)
	}
	if n.List(NetworkFilter{Source: SourceSummary}, func(e NetworkEntry) NetworkEntry { return e }).Total != 0 {
		t.Error("the dropped summary entry is still listed")
	}
	if e, ok := n.Get(3); !ok || e.RspBody == nil {
		t.Error("get keeps the body")
	}
}

func TestSessionNetworkCommands(t *testing.T) {
	s, _ := newTestSession(t, "describe-simple")
	requests, err := os.ReadFile(filepath.Join("testdata", "SampleApp", ".sightmap", "requests.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(s.SightmapDir, "requests.yaml"), requests, 0o644)
	s.Network.Push(harFixture(t))
	list, err := handle(t, s, "networkList", NetworkFilter{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(list)
	for _, want := range []string{`"matches":["CreateOrder"]`, `{"name":"order","value":"2"}`, `"value":"\u003credacted\u003e"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("list lacks %s:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "sample-not-a-secret") || strings.Contains(string(data), "reqBody") {
		t.Errorf("list leaks a credential or a body:\n%s", data)
	}
	got, _ := handle(t, s, "networkGet", ValueArgs{Index: 0, Unredacted: true})
	data, _ = json.Marshal(got)
	if !strings.Contains(string(data), "Bearer sample-not-a-secret") || !strings.Contains(string(data), "reqBody") {
		t.Errorf("unredacted get lacks the credential or body:\n%s", data)
	}
	if _, err := handle(t, s, "networkGet", ValueArgs{Index: 7}); Code(err) != "not_found" {
		t.Errorf("err = %v", err)
	}
	if _, err := handle(t, s, "networkList", NetworkFilter{Source: "pcap"}); Code(err) != "usage" {
		t.Errorf("err = %v", err)
	}
	if runs, _ := filepath.Glob(filepath.Join(s.SightmapDir, ".device-runs", "*.jsonl")); len(runs) > 0 {
		t.Errorf("network commands must stay out of the run log: %v", runs)
	}
}

type cdpFixtureLine struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Body   string          `json:"body"`
	Reply  json.RawMessage `json:"reply"`
}

func cdpFixture(t *testing.T) []cdpFixtureLine {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "network", "cdp.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []cdpFixtureLine
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var f cdpFixtureLine
		if err := json.Unmarshal([]byte(l), &f); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, f)
	}
	return lines
}

func checkCDPEntries(t *testing.T, got []NetworkEntry) {
	t.Helper()
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(got), got)
	}
	get, post, missing := got[0], got[1], got[2]
	if get.Source != SourceRNCDP || get.Method != "GET" || get.URL != "https://api.example.test/get?q=shoes&page=2&round=6" || get.Status != 200 || get.DurationMs != 29 {
		t.Errorf("get = %+v", get)
	}
	if get.RspBody == nil || !strings.Contains(get.RspBody.Content, `"q": "shoes"`) || !strings.HasSuffix(get.Time, "Z") {
		t.Errorf("get body = %+v, time %q", get.RspBody, get.Time)
	}
	if post.ReqBody == nil || !strings.Contains(post.ReqBody.Content, `"sku":"P-1"`) || headerValue(post.ReqHeaders, "Authorization") == "" {
		t.Errorf("post = %+v", post)
	}
	if missing.Status != 404 || missing.RspBody != nil {
		t.Errorf("404 = %+v", missing)
	}
}

func TestCDPTrackerAssemblesRequests(t *testing.T) {
	tracker := newCDPTracker()
	bodies := map[string]string{}
	var done []*cdpRequest
	for _, l := range cdpFixture(t) {
		if l.Body != "" {
			bodies[l.Body] = decodeCDPBody(l.Reply)
			continue
		}
		if r, ok := tracker.event(l.Method, l.Params); ok {
			done = append(done, r)
		}
	}
	var got []NetworkEntry
	for _, r := range done {
		got = append(got, r.entry(bodies[r.RequestID]))
	}
	checkCDPEntries(t, got)
}

func TestPickMetroPage(t *testing.T) {
	pages := []metroPage{
		{ID: "1", AppID: "com.other", WebSocketDebuggerURL: "ws://x/1", ReactNative: json.RawMessage(`{}`)},
		{ID: "2", AppID: "dev.sightmap.RNSample", WebSocketDebuggerURL: "ws://x/2", ReactNative: json.RawMessage(`{}`)},
	}
	if p, ok := pickMetroPage(pages, "dev.sightmap.RNSample"); !ok || p.ID != "2" {
		t.Errorf("picked %+v", p)
	}
	if _, ok := pickMetroPage(pages, "dev.missing"); ok {
		t.Error("another app's page was picked")
	}
	if p, ok := pickMetroPage([]metroPage{{ID: "3", WebSocketDebuggerURL: "ws://x/3", ReactNative: json.RawMessage(`{}`)}}, "dev.any"); !ok || p.ID != "3" {
		t.Error("a page without appId is the fallback")
	}
}

// fakeMetro serves /json/list and a debugger socket that, like Metro, wants
// its own Origin, then replays the fixture once Network.enable arrives.
func fakeMetro(t *testing.T, lines []cdpFixtureLine) *httptest.Server {
	var srv *httptest.Server
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		ws := "ws" + strings.TrimPrefix(srv.URL, "http") + "/inspector/debug?device=d&page=1"
		json.NewEncoder(w).Encode([]metroPage{{ID: "1", AppID: "dev.sightmap.RNSample", WebSocketDebuggerURL: ws, ReactNative: json.RawMessage(`{}`)}})
	})
	mux.HandleFunc("/inspector/debug", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		bodies := map[string]json.RawMessage{}
		for _, l := range lines {
			if l.Body != "" {
				bodies[l.Body] = l.Reply
			}
		}
		for {
			var m cdpMessage
			if conn.ReadJSON(&m) != nil {
				return
			}
			switch m.Method {
			case "Network.enable":
				conn.WriteJSON(cdpMessage{ID: m.ID, Result: json.RawMessage(`{}`)})
				for _, l := range lines {
					if l.Method != "" {
						conn.WriteJSON(cdpMessage{Method: l.Method, Params: l.Params})
					}
				}
			case "Network.getResponseBody":
				var p struct{ RequestID string }
				json.Unmarshal(m.Params, &p)
				reply := cdpMessage{ID: m.ID, Result: bodies[p.RequestID]}
				if strings.Contains(string(bodies[p.RequestID]), `"code"`) {
					reply = cdpMessage{ID: m.ID}
					reply.Error = &struct {
						Message string `json:"message"`
					}{"Could not retrieve response body"}
				}
				conn.WriteJSON(reply)
			default:
				conn.WriteJSON(cdpMessage{ID: m.ID, Result: json.RawMessage(`{}`)})
			}
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCDPSourceAttachesToMetro(t *testing.T) {
	srv := fakeMetro(t, cdpFixture(t))
	var port int
	if _, err := fmtSscan(srv.URL, &port); err != nil {
		t.Fatal(err)
	}
	pushed := make(chan NetworkEntry, 10)
	states := make(chan string, 10)
	src := &cdpSource{port: port, bundleID: "dev.sightmap.RNSample", log: func(string) {},
		push: func(e NetworkEntry) { pushed <- e }, setState: func(state, detail string) { states <- state + ": " + detail }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go src.run(ctx)
	var got []NetworkEntry
	for len(got) < 3 {
		select {
		case e := <-pushed:
			got = append(got, e)
		case <-time.After(5 * time.Second):
			t.Fatalf("got %d of 3 entries", len(got))
		}
	}
	checkCDPEntries(t, got)
	if s := <-states; !strings.HasPrefix(s, "active: attached to Metro") {
		t.Errorf("state = %s", s)
	}
}

func TestCDPSourceReportsNoMetro(t *testing.T) {
	src := &cdpSource{port: 1, bundleID: "dev.example"}
	if why := src.attach(context.Background()); !strings.Contains(why, "no Metro on port 1") {
		t.Errorf("why = %s", why)
	}
}

func TestEntitlementCheck(t *testing.T) {
	gta := `<plist version="1.0"><dict>	<key>application-identifier</key>	<string>T.dev.x</string>	<key>get-task-allow</key>	<true/></dict></plist>`
	if !getTaskAllow.MatchString(gta) {
		t.Error("get-task-allow not found")
	}
	if getTaskAllow.MatchString(strings.Replace(gta, "<true/>", "<false/>", 1)) {
		t.Error("get-task-allow false read as true")
	}
	c := &NetCollector{AppPath: t.TempDir(), Process: "Missing"}
	if ok, why := c.entitled(context.Background()); ok || !strings.Contains(why, "Missing lacks the get-task-allow entitlement") {
		t.Errorf("entitled = %v, %s", ok, why)
	}
}

func TestSetStatePausesSummaryWhileRNCDPIsAttached(t *testing.T) {
	c := newTestCollector(false)
	c.summaryDetail = "host only"
	c.setState(SourceRNCDP, stateActive, "attached")
	if c.states[SourceSummary].State != stateStandby {
		t.Fatalf("summary = %+v", c.states[SourceSummary])
	}
	c.onSummary(NetworkEntry{Source: SourceSummary, Status: 200})
	if c.Network.Len() != 0 {
		t.Error("a paused summary source listed an entry")
	}
	c.setState(SourceRNCDP, stateStandby, "connection closed")
	if c.states[SourceSummary].State != stateActive || c.states[SourceSummary].Detail != "host only" {
		t.Errorf("summary = %+v", c.states[SourceSummary])
	}
}

func TestLaunchEnv(t *testing.T) {
	var none *NetCollector
	if none.LaunchEnv() != nil {
		t.Error("a nil collector adds no environment")
	}
	if env := (&NetCollector{diagnostics: true}).LaunchEnv(); len(env) != 1 || env[0] != "CFNETWORK_DIAGNOSTICS=1" {
		t.Errorf("env = %v", env)
	}
}

func fmtSscan(serverURL string, port *int) (int, error) {
	_, p, _ := strings.Cut(strings.TrimPrefix(serverURL, "http://"), ":")
	return fmt.Sscan(p, port)
}
