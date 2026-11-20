package browser

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"unicode/utf8"
)

// CDP Input.dispatchKeyEvent modifier bits.
const (
	modAlt   = 1
	modCtrl  = 2
	modMeta  = 4
	modShift = 8
)

// controlOrMetaIsMeta decides what the ControlOrMeta modifier means: Meta on
// macOS, Control elsewhere (as Playwright does). It follows the host running
// the CLI, which is where the browser normally runs.
var controlOrMetaIsMeta = runtime.GOOS == "darwin"

// modifierKey describes one modifier, both as a bit and as a key of its own.
type modifierKey struct {
	bit  int
	key  string
	code string
	vk   int
}

var (
	modShiftKey = modifierKey{modShift, "Shift", "ShiftLeft", 16}
	modCtrlKey  = modifierKey{modCtrl, "Control", "ControlLeft", 17}
	modAltKey   = modifierKey{modAlt, "Alt", "AltLeft", 18}
	modMetaKey  = modifierKey{modMeta, "Meta", "MetaLeft", 91}
)

// modifierByName resolves a chord token to a modifier, case-insensitively.
func modifierByName(name string) (modifierKey, bool) {
	switch strings.ToLower(name) {
	case "shift":
		return modShiftKey, true
	case "control", "ctrl":
		return modCtrlKey, true
	case "alt", "option":
		return modAltKey, true
	case "meta", "cmd", "command":
		return modMetaKey, true
	case "controlormeta":
		if controlOrMetaIsMeta {
			return modMetaKey, true
		}
		return modCtrlKey, true
	}
	return modifierKey{}, false
}

// namedKey is a key that has a name rather than a character.
type namedKey struct {
	key  string
	code string
	vk   int
	text string // what pressing it types, if anything
}

// namedKeys is keyed by lower-cased name.
var namedKeys = map[string]namedKey{
	"enter":      {"Enter", "Enter", 13, "\r"},
	"tab":        {"Tab", "Tab", 9, ""},
	"escape":     {"Escape", "Escape", 27, ""},
	"backspace":  {"Backspace", "Backspace", 8, ""},
	"delete":     {"Delete", "Delete", 46, ""},
	"arrowup":    {"ArrowUp", "ArrowUp", 38, ""},
	"arrowdown":  {"ArrowDown", "ArrowDown", 40, ""},
	"arrowleft":  {"ArrowLeft", "ArrowLeft", 37, ""},
	"arrowright": {"ArrowRight", "ArrowRight", 39, ""},
	"space":      {" ", "Space", 32, " "},
	"home":       {"Home", "Home", 36, ""},
	"end":        {"End", "End", 35, ""},
	"pageup":     {"PageUp", "PageUp", 33, ""},
	"pagedown":   {"PageDown", "PageDown", 34, ""},
}

func init() {
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("F%d", i)
		namedKeys[strings.ToLower(name)] = namedKey{name, name, 111 + i, ""}
	}
}

// usKey is one physical key of the US layout: the character it types plain
// and with Shift held.
type usKey struct {
	code    string
	vk      int
	base    string
	shifted string
}

var (
	usByCode    = map[string]usKey{} // lower-cased code -> key
	usByBase    = map[string]usKey{} // unshifted character -> key
	usByShifted = map[string]usKey{} // shifted character -> key
)

func init() {
	add := func(k usKey) {
		usByCode[strings.ToLower(k.code)] = k
		usByBase[k.base] = k
		usByShifted[k.shifted] = k
	}
	for c := 'a'; c <= 'z'; c++ {
		add(usKey{"Key" + strings.ToUpper(string(c)), 65 + int(c-'a'), string(c), strings.ToUpper(string(c))})
	}
	const digitShifts = ")!@#$%^&*("
	for d := 0; d <= 9; d++ {
		add(usKey{fmt.Sprintf("Digit%d", d), 48 + d, fmt.Sprint(d), string(digitShifts[d])})
	}
	for _, k := range []usKey{
		{"Backquote", 192, "`", "~"},
		{"Minus", 189, "-", "_"},
		{"Equal", 187, "=", "+"},
		{"BracketLeft", 219, "[", "{"},
		{"BracketRight", 221, "]", "}"},
		{"Backslash", 220, "\\", "|"},
		{"Semicolon", 186, ";", ":"},
		{"Quote", 222, "'", "\""},
		{"Comma", 188, ",", "<"},
		{"Period", 190, ".", ">"},
		{"Slash", 191, "/", "?"},
	} {
		add(k)
	}
}

// keyStroke is what one key press looks like to the page.
type keyStroke struct {
	key  string
	code string
	vk   int
	text string // what the key types, before the modifier rule in keyEventParams
}

// resolveKeyStroke turns the key part of a chord into a keyStroke. shift says
// whether Shift is held, which selects the shifted character of a key.
//
// Accepted: a named key (Enter, Space, F5, ...), a code name (KeyC, Digit1,
// BracketLeft), or a single character (c, C, 5, ?). A character that is the
// shifted face of a US key (C, ?) keeps itself as key and takes the code of
// the physical key; it does not imply Shift, as in Playwright.
func resolveKeyStroke(name string, shift bool) (keyStroke, error) {
	if name == " " {
		name = "Space"
	}
	if n, ok := namedKeys[strings.ToLower(name)]; ok {
		return keyStroke{n.key, n.code, n.vk, n.text}, nil
	}
	pick := func(k usKey) keyStroke {
		char := k.base
		if shift {
			char = k.shifted
		}
		return keyStroke{char, k.code, k.vk, char}
	}
	if k, ok := usByCode[strings.ToLower(name)]; ok {
		return pick(k), nil
	}
	if utf8.RuneCountInString(name) == 1 {
		if k, ok := usByBase[name]; ok {
			return pick(k), nil
		}
		if k, ok := usByShifted[name]; ok {
			return keyStroke{name, k.code, k.vk, name}, nil
		}
		// Not on the US layout: a character with no code or key code.
		return keyStroke{name, "", 0, name}, nil
	}
	return keyStroke{}, fmt.Errorf("unknown key %q (use a character, a code name like KeyC or Digit1, or Enter Tab Escape Space Backspace Delete Home End PageUp PageDown Arrow* F1-F12)", name)
}

// splitChord splits "Shift+Alt+KeyP" into its modifier tokens and its key.
// The key may itself be "+", written "Shift++" or just "+".
func splitChord(chord string) (mods []string, key string, err error) {
	if chord == "" {
		return nil, "", fmt.Errorf("empty key")
	}
	if !strings.Contains(chord, "+") {
		return nil, chord, nil
	}
	if chord == "+" {
		return nil, "+", nil
	}
	var rest string
	if strings.HasSuffix(chord, "++") {
		key, rest = "+", strings.TrimSuffix(chord, "++")
	} else {
		i := strings.LastIndex(chord, "+")
		key, rest = chord[i+1:], chord[:i]
	}
	if key == "" || rest == "" {
		return nil, "", fmt.Errorf("invalid key %q", chord)
	}
	mods = strings.Split(rest, "+")
	for _, m := range mods {
		if m == "" {
			return nil, "", fmt.Errorf("invalid key %q", chord)
		}
	}
	return mods, key, nil
}

// maskOf is the CDP modifiers bitmask of mods.
func maskOf(mods []modifierKey) int {
	mask := 0
	for _, m := range mods {
		mask |= m.bit
	}
	return mask
}

// modEvent is a keyDown or keyUp of a modifier key itself, with the modifier
// state the page sees at that moment.
func modEvent(typ string, m modifierKey, mask int) map[string]interface{} {
	return map[string]interface{}{
		"type": typ, "key": m.key, "code": m.code,
		"windowsVirtualKeyCode": m.vk, "location": 1, "modifiers": mask,
	}
}

// keyEventParams is the sequence of Input.dispatchKeyEvent params that
// presses chord: a keyDown for each modifier, keyDown and keyUp of the key,
// then a keyUp for each modifier in reverse.
//
// text is sent only when no Ctrl, Meta or Alt is held and the key types a
// character, so Meta+K does not insert a "k". A key without text goes down as
// rawKeyDown, as Playwright sends it.
func keyEventParams(chord string) ([]map[string]interface{}, error) {
	modTokens, keyName, err := splitChord(chord)
	if err != nil {
		return nil, err
	}
	var mods []modifierKey
	for _, tok := range modTokens {
		m, ok := modifierByName(tok)
		if !ok {
			return nil, fmt.Errorf("unknown modifier %q in %q (use Shift, Control/Ctrl, Alt/Option, Meta/Cmd/Command, ControlOrMeta)", tok, chord)
		}
		if maskOf(mods)&m.bit != 0 {
			return nil, fmt.Errorf("invalid key %q: %s given twice", chord, m.key)
		}
		mods = append(mods, m)
	}
	mask := maskOf(mods)

	var events []map[string]interface{}
	for i, m := range mods {
		events = append(events, modEvent("rawKeyDown", m, maskOf(mods[:i+1])))
	}

	if m, ok := modifierByName(keyName); ok {
		// A modifier pressed as the key itself ("Shift", "Control+Alt").
		if mask&m.bit != 0 {
			return nil, fmt.Errorf("invalid key %q: %s given twice", chord, m.key)
		}
		events = append(events, modEvent("rawKeyDown", m, mask|m.bit), modEvent("keyUp", m, mask))
	} else {
		ks, err := resolveKeyStroke(keyName, mask&modShift != 0)
		if err != nil {
			return nil, err
		}
		text := ks.text
		if mask&(modCtrl|modMeta|modAlt) != 0 {
			text = ""
		}
		event := func(typ string) map[string]interface{} {
			p := map[string]interface{}{"type": typ, "key": ks.key, "modifiers": mask}
			if ks.code != "" {
				p["code"] = ks.code
			}
			if ks.vk != 0 {
				p["windowsVirtualKeyCode"] = ks.vk
			}
			return p
		}
		down := event("rawKeyDown")
		if text != "" {
			down["type"] = "keyDown"
			down["text"] = text
		}
		events = append(events, down, event("keyUp"))
	}

	for i := len(mods) - 1; i >= 0; i-- {
		events = append(events, modEvent("keyUp", mods[i], maskOf(mods[:i])))
	}
	return events, nil
}

// KeyPress presses a key or a Playwright-style chord (Modifier+...+Key).
//
// Modifiers: Shift, Control (Ctrl), Alt (Option), Meta (Cmd, Command),
// ControlOrMeta. The key is a character (c, C, 5, ., ?), a code name (KeyC,
// Digit1, Period, BracketLeft) or a named key (Enter, Tab, Escape, Space,
// Backspace, Delete, Arrow*, Home, End, PageUp, PageDown, F1-F12). Examples:
// "Escape", "Shift+KeyV", "Meta+K", "Alt+Shift+P", "ControlOrMeta+Enter".
//
// The page sees the key and code of the pressed key, the modifier state on
// keydown and keyup, and key events for the modifiers themselves around it.
func KeyPress(ctx context.Context, conn *CDPConn, chord string) error {
	events, err := keyEventParams(chord)
	if err != nil {
		return fmt.Errorf("KeyPress: %w", err)
	}
	for _, params := range events {
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", params); err != nil {
			return fmt.Errorf("KeyPress: %s %q: %w", params["type"], chord, err)
		}
	}
	return nil
}
