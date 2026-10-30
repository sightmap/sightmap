package device

import (
	"errors"
	"fmt"
	"strings"
)

// Error is a failure with a stable code and the next step to take. Codes cross
// the daemon socket unchanged, so a CLI or library caller can branch on them.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Error) Error() string { return e.Message }

var hints = map[string]string{
	"no_session":         "run `sightmap device start --detach` first",
	"daemon_unreachable": "run `sightmap device start --detach` to start a new session",
	"device_not_found":   "list devices with `xcrun simctl list devices available`, then pass --device",
	"not_booted":         "run `sightmap device start` to boot the simulator",
	"ax_disabled":        "check the causes listed, in order, then snapshot again",
	"ax_unavailable":     "the app may still be launching; retry, or run `sightmap device relaunch`",
	"ax_error":           "the app may not be in the foreground; run `sightmap device launch`, or `sightmap device relaunch`",
	"hid_error":          "retry; if it keeps failing, run `sightmap device stop` then `sightmap device start`",
	"helper_crashed":     "retry; the daemon restarts the helper automatically",
	"not_found":          "run `sightmap device snapshot` to see what is on screen",
	"stale_target":       "run `sightmap device snapshot` and act on the new ids",
	"offscreen":          "scroll it into view first, e.g. `sightmap device scroll down --until <target>`",
	"covered":            "dismiss what covers it (e.g. `sightmap device key return` for the keyboard) or scroll, then retry",
	"disabled":           "the control is disabled; complete the prerequisite steps, then snapshot again",
	"bad_query":          "targets look like `SignInButton`, `ProductRow[title^=\"Product 1\"]`, or a selector such as `button[identifier=\"login-submit\"]`",
	"unknown_view":       "check the view names under .sightmap/",
	"usage":              "run `sightmap device help`",
	"not_in_front":       "bring the app back with `sightmap device launch`, then retry",
}

// errorf returns an Error with code's default hint.
func errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Hint: hints[code]}
}

// withHint returns an Error whose hint replaces code's default.
func withHint(code, hint, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Hint: hint}
}

// Code returns err's code, or "internal" when it carries none.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "internal"
}

// AsError converts any error to an *Error for the wire.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: "internal", Message: err.Error()}
}

// emptyTreeCauses lists why an app's accessibility tree can come back empty,
// most likely first. App accessibility drops out when it is known to be on.
func emptyTreeCauses(udid string, accessibilityOn bool) []string {
	causes := []string{
		"A view that stays mounted hides everything else: `accessibilityViewIsModal = YES`, or `accessibilityElementsHidden` / React Native `importantForAccessibility=\"no-hide-descendants\"` on a root view (common in React Native, e.g. a full-window overlay that is invisible but still mounted). Check the app's root and overlay views.",
	}
	if !accessibilityOn {
		causes = append(causes, fmt.Sprintf("App accessibility is off on the simulator. Run `sightmap device relaunch` (it enables it first), or `xcrun simctl spawn %s defaults write com.apple.Accessibility ApplicationAccessibilityEnabled -bool true` and relaunch the app.", udid))
	}
	causes = append(causes, "A system sheet (e.g. \"Save Password?\") is animating in. Snapshot again in a moment.")
	for i, c := range causes {
		causes[i] = fmt.Sprintf("  %d. %s", i+1, c)
	}
	return causes
}

func emptyTreeError(udid string, accessibilityOn bool) *Error {
	note := ""
	if accessibilityOn {
		note = " App accessibility is enabled on this simulator, so that is not the cause."
	}
	lines := append([]string{"the app exposes no accessibility elements." + note + " Likely causes, most likely first:"}, emptyTreeCauses(udid, accessibilityOn)...)
	return errorf("ax_disabled", "%s", strings.Join(lines, "\n"))
}
