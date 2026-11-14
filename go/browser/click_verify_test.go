package browser

import (
	"context"
	"encoding/json"
	"github.com/sightmap/sightmap/go/sightmap"
	"strings"
	"sync"
	"testing"
)

// evalResult wraps a JS return value in the CDP Runtime.evaluate envelope the
// fake server hands back.
func evalResult(valueJSON string) json.RawMessage {
	return json.RawMessage(`{"result":{"value":` + valueJSON + `},"exceptionDetails":null}`)
}

func exprOf(params json.RawMessage) string {
	var p struct {
		Expression string `json:"expression"`
	}
	_ = json.Unmarshal(params, &p)
	return p.Expression
}

func nodeWithID(id string) *sightmap.ComponentNode {
	return &sightmap.ComponentNode{Id: id, Bounds: &sightmap.Bounds{X: 0, Y: 0, Width: 20, Height: 10}}
}

func TestClick_OffScreenErrors(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return evalResult(`{"found":true,"inViewport":false,"hit":false,"cx":0,"cy":0}`)
		}
		return json.RawMessage(`{}`)
	})
	_, _, err := Click(context.Background(), conn, nodeWithID("5"))
	if err == nil || !strings.Contains(err.Error(), "viewport") {
		t.Fatalf("expected an off-screen/viewport error, got %v", err)
	}
}

func TestClick_CoveredErrors(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return evalResult(`{"found":true,"inViewport":true,"hit":false,"cx":100,"cy":50}`)
		}
		return json.RawMessage(`{}`)
	})
	_, _, err := Click(context.Background(), conn, nodeWithID("5"))
	if err == nil || !strings.Contains(err.Error(), "covered") {
		t.Fatalf("expected a covered/overlay error, got %v", err)
	}
}

func TestClick_SuccessReturnsPostScrollCoords(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":123,"cy":45}`)
		}
		return json.RawMessage(`{}`) // Input.dispatchMouseEvent
	})
	x, y, err := Click(context.Background(), conn, nodeWithID("5"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if x != 123 || y != 45 {
		t.Errorf("Click returned (%d,%d), want the post-scroll center (123,45)", x, y)
	}
}

func TestFill_DidNotStickErrors(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			switch {
			case strings.Contains(exprOf(params), "\n__smLocateForClick("):
				return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":10,"cy":5}`)
			case strings.Contains(exprOf(params), "\n__smReadValueBySelector("):
				return evalResult(`""`) // field still empty → didn't stick
			}
		}
		return json.RawMessage(`{}`)
	})
	err := Fill(context.Background(), conn, nodeWithID("3"), "hello")
	if err == nil || !strings.Contains(err.Error(), "--clear") {
		t.Fatalf("expected a 'didn't stick / --clear' error, got %v", err)
	}
}

func TestFill_StuckSucceeds(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			switch {
			case strings.Contains(exprOf(params), "\n__smLocateForClick("):
				return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":10,"cy":5}`)
			case strings.Contains(exprOf(params), "\n__smReadValueBySelector("):
				return evalResult(`"hello"`)
			}
		}
		return json.RawMessage(`{}`)
	})
	if err := Fill(context.Background(), conn, nodeWithID("3"), "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// fillPage fakes the page side of fill: the focus click lands, __smSelectActive
// answers selectResp, __smReadValueBySelector answers readResp (JSON), and the
// key events and eval calls are recorded in order.
type fillPage struct {
	selectResp string
	readResp   string
	clearResp  string // __smClearActiveElement envelope; default: it returns ""
	mu         sync.Mutex
	keys       []map[string]interface{}
	calls      []string // "mouse", "key", or the JS function evaluated
}

func (f *fillPage) conn(t *testing.T) *CDPConn {
	return newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch method {
		case "Input.dispatchMouseEvent":
			f.calls = append(f.calls, "mouse")
		case "Input.dispatchKeyEvent":
			var ev map[string]interface{}
			json.Unmarshal(params, &ev)
			f.keys = append(f.keys, ev)
			f.calls = append(f.calls, "key")
		case "Runtime.evaluate":
			expr := exprOf(params)
			call := expr[strings.LastIndex(expr, "\n")+1:]
			call = call[:strings.Index(call, "(")]
			f.calls = append(f.calls, call)
			switch call {
			case "__smLocateForClick":
				return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":10,"cy":5}`)
			case "__smSelectActive":
				return evalResult(f.selectResp)
			case "__smReadValueBySelector":
				return evalResult(f.readResp)
			case "__smClearActiveElement":
				if f.clearResp != "" {
					return json.RawMessage(f.clearResp)
				}
				return evalResult(`""`)
			}
		}
		return json.RawMessage(`{}`)
	})
}

// typed returns the text of the keyDown events that carry text.
func (f *fillPage) typed() string {
	var b strings.Builder
	for _, k := range f.keys {
		if s, ok := k["text"].(string); ok && k["type"] == "keyDown" {
			b.WriteString(s)
		}
	}
	return b.String()
}

// Fill selects the field's content through the DOM and types over it, without
// sending a select-all chord.
func TestFill_SelectsThroughDOM(t *testing.T) {
	page := &fillPage{selectResp: `{"selected":true,"value":"2026-04-10"}`, readResp: `"2026-04-17"`}
	if err := Fill(context.Background(), page.conn(t), nodeWithID("3"), "2026-04-17"); err != nil {
		t.Fatal(err)
	}
	if got := page.typed(); got != "2026-04-17" {
		t.Errorf("typed %q", got)
	}
	for _, k := range page.keys {
		if _, ok := k["commands"]; ok || k["key"] == "a" {
			t.Errorf("a select-all chord was sent although the DOM selected: %v", k)
		}
	}
	// Order: focus click, select, type, read back.
	click := indexOf(page.calls, "mouse")
	sel := indexOf(page.calls, "__smSelectActive")
	first := indexOf(page.calls, "key")
	read := indexOf(page.calls, "__smReadValueBySelector")
	if !(0 <= click && click < sel && sel < first && first < read) {
		t.Errorf("order wrong, calls = %v", page.calls)
	}
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

// When the DOM cannot select the focused element, Fill sends the select-all chord
// with the A key's code and key code and the selectAll editing command, then
// types.
func TestFill_FallsBackToSelectAllChord(t *testing.T) {
	defer func(old bool) { selectAllIsMeta = old }(selectAllIsMeta)
	for _, meta := range []bool{true, false} {
		selectAllIsMeta = meta
		page := &fillPage{selectResp: `{"selected":false,"value":""}`, readResp: `"hi"`}
		if err := Fill(context.Background(), page.conn(t), nodeWithID("3"), "hi"); err != nil {
			t.Fatal(err)
		}
		mods := float64(2)
		if meta {
			mods = 4
		}
		if len(page.keys) != 2+4 { // chord down/up + h, i
			t.Fatalf("meta=%v: %d key events: %v", meta, len(page.keys), page.keys)
		}
		down, up := page.keys[0], page.keys[1]
		if down["type"] != "keyDown" || down["key"] != "a" || down["code"] != "KeyA" ||
			down["windowsVirtualKeyCode"] != float64(65) || down["modifiers"] != mods {
			t.Errorf("meta=%v: chord down = %v", meta, down)
		}
		cmds, _ := down["commands"].([]interface{})
		if len(cmds) != 1 || cmds[0] != "selectAll" {
			t.Errorf("meta=%v: commands = %v, want [selectAll]", meta, down["commands"])
		}
		if up["type"] != "keyUp" || up["code"] != "KeyA" || up["modifiers"] != mods {
			t.Errorf("meta=%v: chord up = %v", meta, up)
		}
		if got := page.typed(); got != "hi" {
			t.Errorf("meta=%v: typed %q, want hi", meta, got)
		}
	}
}

// A fill whose select-all did not select leaves old+new in the field; that is an
// error naming both, not a success.
func TestFill_AppendIsAnError(t *testing.T) {
	page := &fillPage{
		selectResp: `{"selected":true,"value":"2026-04-10"}`,
		readResp:   `"2026-04-102026-04-17"`,
	}
	err := Fill(context.Background(), page.conn(t), nodeWithID("3"), "2026-04-17")
	if err == nil {
		t.Fatal("want an error for an appended value")
	}
	for _, want := range []string{"2026-04-102026-04-17", "2026-04-10", "not replaced", "--clear"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestVerifyFilled(t *testing.T) {
	for name, tc := range map[string]struct {
		value, before, got string
		wantErr            string
	}{
		"replaced":                 {"2026-04-17", "2026-04-10", "2026-04-17", ""},
		"same value refilled":      {"abc", "abc", "abc", ""},
		"was empty":                {"abc", "", "abc", ""},
		"typed in the middle":      {"X", "abc", "aXbc", "not replaced"},
		"typed at the start":       {"ab", "cd", "abcd", "not replaced"},
		"masked value is fine":     {"5551234567", "", "(555) 123-4567", ""},
		"formatted replace":        {"1234", "9999", "1,234", ""},
		"truncated by maxlength":   {"abcdefgh", "zz", "abcde", ""},
		"still empty":              {"abc", "old", "", "still empty"},
		"no old value, appended":   {"abc", "", "abcabc", ""},
		"blank value is not check": {"  ", "old", "old", ""},
	} {
		t.Run(name, func(t *testing.T) {
			page := &fillPage{readResp: strconvQuote(tc.got)}
			err := verifyFilled(context.Background(), page.conn(t), "3", tc.value, tc.before)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// --clear on a textarea: the page clears it and hands back the old text, the
// typed text is what is read afterwards, and no select-all chord is sent.
func TestClearAndFill_Textarea(t *testing.T) {
	page := &fillPage{
		clearResp: string(evalResult(`"Reminder scheduling overhaul"`)),
		readResp:  `"Reminder scheduling engine"`,
	}
	if err := ClearAndFill(context.Background(), page.conn(t), nodeWithID("3"), "Reminder scheduling engine"); err != nil {
		t.Fatal(err)
	}
	if got := page.typed(); got != "Reminder scheduling engine" {
		t.Errorf("typed %q", got)
	}
	for _, k := range page.keys {
		if _, ok := k["commands"]; ok {
			t.Errorf("a select-all chord was sent: %v", k)
		}
	}
}

// A clear that throws in the page (an "Illegal invocation" on the wrong setter,
// nothing focused) is an error before any key is typed, not a silent append.
func TestClearAndFill_ClearFailureIsAnError(t *testing.T) {
	page := &fillPage{
		clearResp: `{"result":{"type":"object"},"exceptionDetails":{"text":"Uncaught","exception":{"description":"TypeError: Illegal invocation"}}}`,
		readResp:  `"Reminder scheduling overhaulReminder scheduling engine"`,
	}
	err := ClearAndFill(context.Background(), page.conn(t), nodeWithID("3"), "Reminder scheduling engine")
	if err == nil || !strings.Contains(err.Error(), "clear") {
		t.Fatalf("err = %v, want a clear error", err)
	}
	if len(page.keys) != 0 {
		t.Errorf("typed %d key events after a failed clear", len(page.keys))
	}
}

// If the clear silently left the old text and the new text was appended, the
// read-back catches it.
func TestClearAndFill_AppendIsAnError(t *testing.T) {
	page := &fillPage{
		clearResp: string(evalResult(`"old title"`)),
		readResp:  `"old titlenew title"`,
	}
	err := ClearAndFill(context.Background(), page.conn(t), nodeWithID("3"), "new title")
	if err == nil || !strings.Contains(err.Error(), "not replaced") {
		t.Fatalf("err = %v, want a not-replaced error", err)
	}
}
