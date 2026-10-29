package device

import (
	"bytes"
	"encoding/xml"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// On Android the tree comes from `uiautomator dump`: the active window's
// accessibility nodes as XML, in pixels. ParseUIAutomator maps it onto the same
// RawElement shape the iOS helper reports, in density-independent pixels (dp),
// so Normalize, the matcher and every action read both platforms alike and
// "points" means dp on Android.

type uiHierarchy struct {
	Rotation string   `xml:"rotation,attr"`
	Nodes    []uiNode `xml:"node"`
}

type uiNode struct {
	Text          string   `xml:"text,attr"`
	ResourceID    string   `xml:"resource-id,attr"`
	Class         string   `xml:"class,attr"`
	Package       string   `xml:"package,attr"`
	ContentDesc   string   `xml:"content-desc,attr"`
	Hint          string   `xml:"hint,attr"`
	Checkable     string   `xml:"checkable,attr"`
	Checked       string   `xml:"checked,attr"`
	Clickable     string   `xml:"clickable,attr"`
	LongClickable string   `xml:"long-clickable,attr"`
	Enabled       string   `xml:"enabled,attr"`
	Scrollable    string   `xml:"scrollable,attr"`
	Password      string   `xml:"password,attr"`
	Selected      string   `xml:"selected,attr"`
	Bounds        string   `xml:"bounds,attr"`
	Children      []uiNode `xml:"node"`
}

// classTypes maps an Android class's simple name (lowercased, with any $inner
// suffix kept) to the accessibility type Normalize understands. Jetpack
// Compose reports semantics roles through the same widget class names
// (Role.Button as android.widget.Button, editable text as EditText, …).
var classTypes = map[string]string{
	"button": "Button", "imagebutton": "Button", "materialbutton": "Button", "appcompatbutton": "Button",
	"appcompatimagebutton": "Button", "floatingactionbutton": "Button", "extendedfloatingactionbutton": "Button",
	"chip": "Button",

	"textview": "StaticText", "appcompattextview": "StaticText", "materialtextview": "StaticText",
	"checkedtextview": "StaticText",

	"edittext": "TextField", "appcompatedittext": "TextField", "textinputedittext": "TextField",
	"autocompletetextview": "TextField", "appcompatautocompletetextview": "TextField",
	"materialautocompletetextview": "TextField", "multiautocompletetextview": "TextField",
	"searchview$searchautocomplete": "SearchField", "searchautocomplete": "SearchField",

	"checkbox": "CheckBox", "appcompatcheckbox": "CheckBox", "materialcheckbox": "CheckBox",
	"switch": "Switch", "switchcompat": "Switch", "switchmaterial": "Switch", "materialswitch": "Switch",
	"togglebutton": "Switch",
	"radiobutton":  "RadioButton", "appcompatradiobutton": "RadioButton", "materialradiobutton": "RadioButton",

	"imageview": "Image", "appcompatimageview": "Image", "shapeableimageview": "Image",

	"scrollview": "ScrollView", "horizontalscrollview": "ScrollView", "nestedscrollview": "ScrollView",
	"recyclerview": "CollectionView", "listview": "CollectionView", "gridview": "CollectionView",
	"viewpager": "CollectionView", "viewpager2": "CollectionView",

	"seekbar": "Slider", "appcompatseekbar": "Slider", "ratingbar": "Slider", "slider": "Slider",
	"progressbar": "ProgressIndicator", "contentloadingprogressbar": "ProgressIndicator",
	"circularprogressindicator": "ProgressIndicator", "linearprogressindicator": "ProgressIndicator",
	"spinner": "PopUpButton", "appcompatspinner": "PopUpButton",
	"numberpicker": "Picker",
	"webview":      "WebView",
	"toolbar":      "Toolbar", "materialtoolbar": "Toolbar",
}

// tabClasses report a tab: TabLayout's tab view describes itself as an
// ActionBar tab.
var tabClasses = map[string]bool{"actionbar$tab": true, "tablayout$tabview": true}

var checkedTypes = map[string]bool{"CheckBox": true, "Switch": true, "RadioButton": true}

var boundsPattern = regexp.MustCompile(`^\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]$`)

// resourceIDPrefix is a view id's package qualifier, "com.example:id/". A
// Compose testTag published with testTagsAsResourceId has none.
var resourceIDPrefix = regexp.MustCompile(`^[A-Za-z0-9_.]+:id/`)

func simpleClass(class string) string {
	if i := strings.LastIndex(class, "."); i >= 0 {
		class = class[i+1:]
	}
	return strings.ToLower(class)
}

// androidIdentifier is the node's resource-id without its package qualifier.
// The framework's own ids (android:id/content, statusBarBackground, …) name
// containers, not app elements, and are dropped.
func androidIdentifier(id string) string {
	if id == "" || strings.HasPrefix(id, "android:id/") {
		return ""
	}
	return resourceIDPrefix.ReplaceAllString(id, "")
}

// AndroidScreen describes the display a dump was taken on.
type AndroidScreen struct {
	WidthPx, HeightPx int // in the natural (portrait) orientation, as `wm size` reports
	Density           int // dpi, as `wm density` reports; 160 is 1 px per dp
	// Keyboard is the input method window's frame in pixels while it is
	// shown, else nil.
	Keyboard *[4]int
}

func (s AndroidScreen) scale() float64 {
	if s.Density <= 0 {
		return 1
	}
	return float64(s.Density) / 160
}

type uiConverter struct {
	scale float64
}

func (c uiConverter) frame(bounds string) Frame {
	m := boundsPattern.FindStringSubmatch(bounds)
	if m == nil {
		return Frame{}
	}
	var v [4]float64
	for i := range v {
		n, _ := strconv.Atoi(m[i+1])
		v[i] = float64(n) / c.scale
	}
	return Frame{X: v[0], Y: v[1], W: math.Max(0, v[2]-v[0]), H: math.Max(0, v[3]-v[1])}
}

func (c uiConverter) element(n uiNode) *RawElement {
	class := simpleClass(n.Class)
	typ := classTypes[class]
	subrole := ""
	switch {
	case tabClasses[class]:
		typ, subrole = "Button", "AXTabButton"
	case typ == "TextField" && n.Password == "true":
		subrole = "AXSecureTextField"
	case typ == "" && n.Scrollable == "true":
		typ = "ScrollView"
	case typ == "" && n.Password == "true":
		typ, subrole = "TextField", "AXSecureTextField"
	case typ == "":
		typ = "Other"
	}
	enabled := n.Enabled != "false"
	el := &RawElement{
		Role:       "AX" + typ,
		Subrole:    subrole,
		Type:       typ,
		Identifier: androidIdentifier(n.ResourceID),
		Frame:      c.frame(n.Bounds),
		Enabled:    &enabled,
	}
	// An empty field reports its hint as its text; recent dumps carry the hint
	// separately too. The hint is the field's label, and the text its value.
	hint := n.Hint
	if hint == "true" || hint == "false" {
		hint = ""
	}
	switch {
	case typ == "TextField" || typ == "SearchField":
		el.Label = cmpOr(n.ContentDesc, hint)
		el.Value = Value(n.Text)
	case checkedTypes[typ] && n.Checkable == "true":
		el.Label = cmpOr(n.ContentDesc, n.Text)
		el.Value = "0"
		if n.Checked == "true" {
			el.Value = "1"
		}
	case typ == "Slider" || typ == "ProgressIndicator":
		el.Label = n.ContentDesc
		el.Value = Value(n.Text)
	default:
		el.Label = cmpOr(n.ContentDesc, n.Text)
		if n.ContentDesc != "" && n.Text != "" && n.Text != n.ContentDesc {
			el.Value = Value(n.Text)
		}
	}
	clickable := n.Clickable == "true" || n.LongClickable == "true"
	if clickable {
		el.Traits = append(el.Traits, "Button")
	}
	if n.Selected == "true" {
		el.Traits = append(el.Traits, "Selected")
	}
	for _, ch := range n.Children {
		el.Children = append(el.Children, c.element(ch))
	}
	// A clickable container without text of its own (a list row, a card, a
	// Compose Row with Modifier.clickable, a Compose Button around its Text) is
	// announced by its children's text, so it gets that as its label and counts
	// as something to tap.
	if clickable && el.Label == "" && (typ == "Other" || typ == "Image" || typ == "Button") {
		el.Label = truncateRunes(strings.Join(descendantText(el), ", "), maxText)
	}
	return el
}

func descendantText(el *RawElement) []string {
	var out []string
	for _, ch := range el.Children {
		if ch.Label != "" && (ch.Type == "StaticText" || len(ch.Children) == 0) {
			out = append(out, ch.Label)
			continue
		}
		out = append(out, descendantText(ch)...)
	}
	return out
}

// uiautomatorError is the line `uiautomator dump` prints instead of a dump.
var uiautomatorError = regexp.MustCompile(`(?m)^ERROR: (.*)$`)

// ParseUIAutomator converts a `uiautomator dump` into a Describe. appPackage
// is the app under test: a window from any other package (a permission
// dialog, the launcher after a crash) is reported as an overlay, named by its
// package.
func ParseUIAutomator(data []byte, appPackage string, screen AndroidScreen) (*Describe, error) {
	if m := uiautomatorError.FindSubmatch(data); m != nil && !bytes.Contains(data, []byte("<hierarchy")) {
		return nil, uiautomatorFailure(string(m[1]))
	}
	start := bytes.Index(data, []byte("<hierarchy"))
	end := bytes.LastIndex(data, []byte("</hierarchy>"))
	if start < 0 || end < start {
		return nil, errorf("ax_unavailable", "uiautomator returned no hierarchy: %s", truncateRunes(strings.TrimSpace(string(data)), 200))
	}
	var h uiHierarchy
	if err := xml.Unmarshal(data[start:end+len("</hierarchy>")], &h); err != nil {
		return nil, errorf("ax_error", "cannot parse the uiautomator dump: %v", err)
	}
	c := uiConverter{scale: screen.scale()}
	w, hgt := screen.WidthPx, screen.HeightPx
	if h.Rotation == "1" || h.Rotation == "3" {
		w, hgt = hgt, w
	}
	if w == 0 || hgt == 0 {
		for _, n := range h.Nodes {
			f := c.frame(n.Bounds)
			w, hgt = max(w, int((f.X+f.W)*c.scale)), max(hgt, int((f.Y+f.H)*c.scale))
		}
	}
	size := Size{W: math.Round(float64(w)/c.scale*100) / 100, H: math.Round(float64(hgt)/c.scale*100) / 100}
	root := &RawElement{Role: "AXApplication", Type: "Application", Label: appPackage, Frame: Frame{W: size.W, H: size.H}}
	d := &Describe{Screen: size, Root: root}
	foreign := ""
	for _, n := range h.Nodes {
		if appPackage != "" && n.Package != "" && n.Package != appPackage {
			foreign = n.Package
		} else {
			foreign = ""
			break
		}
	}
	if foreign != "" {
		d.Overlay = true
		root.Label = foreign
	}
	for _, n := range h.Nodes {
		root.Children = append(root.Children, c.element(n))
	}
	if k := screen.Keyboard; k != nil && len(root.Children) > 0 {
		f := c.frame("[" + strconv.Itoa(k[0]) + "," + strconv.Itoa(k[1]) + "][" + strconv.Itoa(k[2]) + "," + strconv.Itoa(k[3]) + "]")
		if f.W > 0 && f.H > 0 {
			root.Children = append(root.Children, &RawElement{Role: "AXKeyboard", Type: "Keyboard", Label: "keyboard", Frame: f})
		}
	}
	return d, nil
}

// uiautomatorFailure classifies an `ERROR:` line from uiautomator dump.
func uiautomatorFailure(message string) *Error {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "idle state"):
		return withHint("ax_unavailable", "the screen keeps animating; retry, or turn off animations with `adb shell settings put global animator_duration_scale 0` (and transition_animation_scale, window_animation_scale)",
			"uiautomator could not read the screen while it was animating (%s)", message)
	case strings.Contains(lower, "already registered"):
		return withHint("ax_error", "stop the other UI automation (an Appium or UiAutomator2 server, Android Studio's layout inspector), then retry",
			"another UI automation client holds the device's accessibility bridge (%s)", message)
	case strings.Contains(lower, "null root node"):
		return errorf("ax_unavailable", "uiautomator found no window to read (%s)", message)
	}
	return errorf("ax_error", "uiautomator dump failed: %s", message)
}

var (
	wmSizePattern    = regexp.MustCompile(`(Physical|Override) size: (\d+)x(\d+)`)
	wmDensityPattern = regexp.MustCompile(`(Physical|Override) density: (\d+)`)
	imeFramePattern  = regexp.MustCompile(`\b(?:mFrame|frame)=\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]`)
)

// parseWM reads `wm size` and `wm density` output; an override wins over the
// physical value.
func parseWM(size, density string) AndroidScreen {
	var s AndroidScreen
	for _, m := range wmSizePattern.FindAllStringSubmatch(size, -1) {
		if s.WidthPx == 0 || m[1] == "Override" {
			s.WidthPx, _ = strconv.Atoi(m[2])
			s.HeightPx, _ = strconv.Atoi(m[3])
		}
	}
	for _, m := range wmDensityPattern.FindAllStringSubmatch(density, -1) {
		if s.Density == 0 || m[1] == "Override" {
			s.Density, _ = strconv.Atoi(m[2])
		}
	}
	return s
}

// parseIMEWindow reads `dumpsys window InputMethod` and returns the keyboard's
// frame in pixels when it is on screen. The format varies across Android
// versions; anything unrecognized reads as no keyboard.
func parseIMEWindow(out string) *[4]int {
	visible := strings.Contains(out, "isVisible=true") ||
		(strings.Contains(out, "mHasSurface=true") && strings.Contains(out, "mViewVisibility=0x0"))
	if !visible {
		return nil
	}
	m := imeFramePattern.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	var f [4]int
	for i := range f {
		f[i], _ = strconv.Atoi(m[i+1])
	}
	if f[2] <= f[0] || f[3] <= f[1] || f[1] <= 0 {
		return nil
	}
	return &f
}
