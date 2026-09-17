package explore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestJevPickerWireFormat(t *testing.T) {
	var gotBody map[string]interface{}
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"jev-latest","usage":{"input_tokens":120,"output_tokens":9},"answers":{"next":{"type":"choice","choice":"n2","confidence":0.8,"probabilities":{"n1":0.2,"n2":0.8}},"done":{"type":"noul","noul":0.07}}}`))
	}))
	defer srv.Close()

	j := &JevPicker{APIKey: "k", BaseURL: srv.URL, Model: "jev-latest", Client: srv.Client()}
	crit := Criteria{Options: []Criterion{{"n1", "button A"}, {"n2", "link B"}, {"back", "go back"}}}
	pick, err := j.Pick(context.Background(), "STATE", crit)
	if err != nil {
		t.Fatal(err)
	}
	if pick.Next != "n2" || pick.Done != 0.07 || pick.Probs["n2"] != 0.8 {
		t.Fatalf("pick = %+v", pick)
	}
	if gotAuth != "Bearer k" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotBody["model"] != "jev-latest" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	state := gotBody["state"].(map[string]interface{})
	if state["context"] != "STATE" {
		t.Fatalf("state = %v", state)
	}
	qs := gotBody["questions"].(map[string]interface{})
	next := qs["next"].(map[string]interface{})
	if next["type"] != "choice" || next["criteria"].(map[string]interface{})["n2"] != "link B" || next["instructions"] == "" {
		t.Fatalf("next question = %v", next)
	}
	if qs["done"].(map[string]interface{})["type"] != "noul" {
		t.Fatalf("done question = %v", qs["done"])
	}
	st := j.Stats()
	if st.Calls != 1 || st.InputTokens != 120 || st.OutputTokens != 9 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestJevPickerFallsBackToFirstOptionOnUnknownChoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"answers":{"next":{"choice":"n99","probabilities":{}},"done":{"noul":0}}}`))
	}))
	defer srv.Close()
	j := &JevPicker{APIKey: "k", BaseURL: srv.URL, Model: "m", Client: srv.Client()}
	pick, err := j.Pick(context.Background(), "s", Criteria{Options: []Criterion{{"n1", "a"}, {"n2", "b"}}})
	if err != nil || pick.Next != "n1" {
		t.Fatalf("pick = %+v err = %v", pick, err)
	}
}

func TestJevPickerRetriesServerErrors(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.Header().Set("retry-after-ms", "1")
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{"answers":{"pick":{"choice":"o1"}}}`))
	}))
	defer srv.Close()
	j := &JevPicker{APIKey: "k", BaseURL: srv.URL, Model: "m", Client: &http.Client{Timeout: 5 * time.Second}}
	got, err := j.Choose(context.Background(), "s", Criteria{Options: []Criterion{{"o0", "a"}, {"o1", "b"}}}, "which?")
	if err != nil || got != "o1" || atomic.LoadInt32(&n) != 2 {
		t.Fatalf("got %q err %v calls %d", got, err, n)
	}
}

func TestJevPickerReportsClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()
	j := &JevPicker{APIKey: "k", BaseURL: srv.URL, Model: "m", Client: srv.Client()}
	if _, err := j.Pick(context.Background(), "s", Criteria{Options: []Criterion{{"n1", "a"}}}); err == nil {
		t.Fatal("expected an error on 401")
	}
}

func TestNewJevPickerNeedsKey(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	if _, err := NewJevPicker(""); err == nil {
		t.Fatal("expected error without key")
	}
	t.Setenv("TYPESAFE_API_KEY", "abc")
	t.Setenv("TYPESAFE_BASE_URL", "")
	j, err := NewJevPicker("")
	if err != nil || j.Model != "jev-latest" || j.BaseURL != JevDefaultBaseURL {
		t.Fatalf("picker = %+v err = %v", j, err)
	}
}
