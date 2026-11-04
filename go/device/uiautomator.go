package device

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"slices"
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
	Focusable     string   `xml:"focusable,attr"`
	Bounds        string   `xml:"bounds,attr"`
	DrawingOrder  string   `xml:"drawing-order,attr"`
	Children      []uiNode `xml:"node"`

	tab bool // a tab, by a library signal prepare found
}

// classTypes maps an Android class's simple name (lowercased, with any $inner
// suffix kept) to the accessibility type Normalize understands. Jetpack
// Compose reports semantics roles through the same widget class names
// (Role.Button as android.widget.Button, editable text as EditText, …), on a
// node of their own under the element (see prepare).
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
	"tabwidget": "TabBar",
}

// tabClasses report a tab: older TabLayout releases' tab view describes itself
// as an ActionBar tab. Current ones report a LinearLayout (see markTabBars).
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
	id = resourceIDPrefix.ReplaceAllString(id, "")
	if internalIDs[id] || strings.HasPrefix(id, "navigation_bar_item_") || strings.HasPrefix(id, "mtrl_") {
		return ""
	}
	return id
}

// internalIDs are ids from AppCompat and Material Components layouts. Library
// resources are merged into the app's package, so they look like the app's own
// ids, but they name the library's containers (a dialog's panels, a bottom
// navigation item's icon), the same on every screen.
var internalIDs = map[string]bool{
	"action_bar_root": true, "action_bar_container": true, "action_bar": true, "action_context_bar": true,
	"decor_content_parent": true, "parentPanel": true, "topPanel": true, "title_template": true,
	"titleDividerNoCustom": true, "alertTitle": true, "contentPanel": true, "customPanel": true,
	"buttonPanel": true, "textSpacerNoTitle": true, "textSpacerNoButtons": true,
	"scrollIndicatorUp": true, "scrollIndicatorDown": true, "design_bottom_sheet": true, "touch_outside": true,
	"textinput_placeholder": true, "textinput_helper_text": true, "textinput_counter": true,
	"text_input_end_icon": true, "text_input_start_icon": true,
}

// roleClasses are the classes Jetpack Compose gives the empty node it adds
// under an element to carry the element's role: a Button, a RadioButton
// (a segmented button), a CheckBox, a Switch.
var roleClasses = map[string]bool{"button": true, "radiobutton": true, "checkbox": true, "switch": true}

// prepare rewrites a dump where uiautomator's view of Jetpack Compose and of
// Android's selection state would mislead the tree:
//
//   - Compose reports an element's role on an extra empty child spanning it
//     (a Button's `android.widget.Button`, a segment's RadioButton) and the
//     element itself as a plain View. The element takes the child's class, and
//     the child goes.
//   - A checkable element with no role (a Compose Switch, or a row with
//     Modifier.toggleable) is a switch.
//   - Compose withholds the click action from the selected tab or segment,
//     and a checked segment, so they would read as not tappable and unnamed;
//     a focusable one that is selected or checkable counts as clickable.
//   - Selection propagates to every descendant view (a bottom navigation
//     item's icon and label are selected too); only the topmost is kept.
//   - A Material Components bottom navigation or rail item reports itself as
//     a FrameLayout, sometimes checkable; its library child ids say it is a tab.
func prepare(n uiNode, parentSelected bool) uiNode {
	if parentSelected {
		n.Selected = ""
	}
	kids := make([]uiNode, 0, len(n.Children))
	for _, ch := range n.Children {
		if isRoleNode(n, ch) {
			n.Class = ch.Class
			if n.Checkable != "true" && ch.Checkable == "true" {
				n.Checkable, n.Checked = "true", ch.Checked
			}
			continue
		}
		kids = append(kids, prepare(ch, parentSelected || n.Selected == "true"))
	}
	n.Children = kids
	n.tab = n.tab || !navItemPart(n) && slices.ContainsFunc(n.Children, navItemPart)
	if n.Checkable == "true" && classTypes[simpleClass(n.Class)] == "" && !tabClasses[simpleClass(n.Class)] && !n.tab {
		n.Class = "android.widget.Switch"
	}
	if n.Focusable == "true" && n.Enabled != "false" && (n.Selected == "true" || n.Checkable == "true") {
		n.Clickable = "true"
	}
	return n
}

// navItemPart reports whether n is one of the views Material Components lays
// out inside a navigation item (its icon, label, active indicator).
func navItemPart(n uiNode) bool {
	return strings.HasPrefix(resourceIDPrefix.ReplaceAllString(n.ResourceID, ""), "navigation_bar_item_")
}

// isRoleNode reports whether ch is the empty role carrier Compose adds under
// parent: a role class, nothing to show or call it by, nothing below it, and
// a parent that is a plain View that can be acted on.
func isRoleNode(parent, ch uiNode) bool {
	return roleClasses[simpleClass(ch.Class)] && ch.Text == "" && ch.ContentDesc == "" && androidIdentifier(ch.ResourceID) == "" &&
		len(ch.Children) == 0 && ch.Clickable != "true" && simpleClass(parent.Class) == "view" &&
		(parent.Clickable == "true" || parent.Checkable == "true" || parent.Focusable == "true")
}

// AndroidScreen describes the display a dump was taken on.
type AndroidScreen struct {
	WidthPx, HeightPx int // in the natural (portrait) orientation, as `wm size` reports
	Density           int // dpi, as `wm density` reports; 160 is 1 px per dp
	// Keyboard holds the parts of the input method window that are drawn and
	// take touches, in pixels, while it is shown: usually one rectangle, more
	// for a floating toolbar. Empty when no keyboard is shown.
	Keyboard [][4]int
	// StatusBarPx is the status bar's height in pixels, or 0 when unknown.
	StatusBarPx int
}

// defaultStatusBar is the status bar height in dp assumed when `dumpsys
// window` doesn't say: the platform's standard bar, without a display cutout.
const defaultStatusBar = 24

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
	case tabClasses[class] || n.tab:
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
	el.DrawingOrder, _ = strconv.Atoi(n.DrawingOrder)
	// An empty field reports its hint as its text; recent dumps carry the hint
	// separately too. The hint is the field's label, and the text its value.
	hint := n.Hint
	if hint == "true" || hint == "false" {
		hint = ""
	}
	desc, text := withoutIconGlyphs(n.ContentDesc), withoutIconGlyphs(n.Text)
	if n.Checkable == "true" && !checkedTypes[typ] && subrole != "AXTabButton" {
		// A checkable button: a segment of a toggle group, a filter chip.
		checked := n.Checked == "true"
		el.Checked = &checked
	}
	switch {
	case typ == "TextField" || typ == "SearchField":
		el.Label = cmpOr(desc, hint)
		el.Value = Value(n.Text)
	case checkedTypes[typ] && n.Checkable == "true":
		el.Label = cmpOr(desc, text)
		el.Value = "0"
		if n.Checked == "true" {
			el.Value = "1"
		}
	case typ == "Slider" || typ == "ProgressIndicator":
		el.Label = desc
		el.Value = Value(n.Text)
	default:
		el.Label = cmpOr(desc, text)
		if desc != "" && text != "" && text != desc {
			el.Value = Value(text)
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
	// as something to tap. A Compose text field shows its label as a child
	// text, too.
	field := typ == "TextField" || typ == "SearchField"
	if el.Label == "" && (field || clickable && (typ == "Other" || typ == "Image" || typ == "Button" || checkedTypes[typ])) {
		texts := descendantText(el)
		if field && len(texts) > 0 {
			texts = texts[:1]
		}
		el.Label = truncateRunes(strings.Join(texts, ", "), maxText)
		// One text is the element's whole label, as on iOS, where a button
		// is one element: keeping it as a child too would make
		// `[text="Sign out"]` match twice. Several texts (a row's title,
		// brand and price) stay, for child components to read one by one.
		if len(texts) == 1 {
			dropText(el, texts[0])
		}
	}
	return el
}

// Where a row of tabs must sit for its look alone to make it a tab bar, in dp:
// its bottom within tabEdgeBottom of the screen's bottom (room for the system
// navigation bar), or its top within tabEdgeTop of the status bar (room for a
// top app bar). barInset is how much taller than its row a bar can be (the
// system navigation bar again, which a bottom bar draws behind).
const (
	tabEdgeBottom = 64
	tabEdgeTop    = 72
	barInset      = 56
)

// markTabBars finds rows of tabs and marks them and their bar, as iOS reports
// a tab bar. Android has no tab role uiautomator shows: Compose's Role.Tab,
// a Material navigation item and a TabLayout tab carry it only in a role
// description, which the dump leaves out. So a row is a tab bar when a library
// signal says so (see prepare, tabClasses, TabWidget), or when it looks like
// one: labelled tappable items side by side, exactly one selected, along the
// top or bottom edge of the screen. A segmented control mid-screen, or one
// whose segments are checked rather than selected, stays as it is.
//
// The bar is the outermost element wrapping only the row and not much taller,
// so it covers the whole band (a BottomNavigationView around its menu view).
func markTabBars(el *RawElement, screen Size, statusBar float64) {
	row := el
	for len(row.Children) == 1 {
		row = row.Children[0]
	}
	bar := el
	if row.Type == "TabBar" {
		bar = row
	}
	if tabs := tabRow(row, screen, statusBar); tabs != nil && bar.Frame.H-row.Frame.H <= barInset {
		bar.Role, bar.Type, bar.Subrole = "AXTabBar", "TabBar", ""
		for _, t := range tabs {
			t.Role, t.Type, t.Subrole = "AXButton", "Button", "AXTabButton"
			dropText(t, t.Label)
		}
		return
	}
	for _, ch := range el.Children {
		markTabBars(ch, screen, statusBar)
	}
}

// tabRow returns row's children if they are a row of tabs, else nil. Empty
// children (a selection indicator, a divider) are passed over.
func tabRow(row *RawElement, screen Size, statusBar float64) []*RawElement {
	var tabs []*RawElement
	signal := row.Type == "TabBar" || slices.ContainsFunc(row.Children, func(ch *RawElement) bool { return ch.Subrole == "AXTabButton" })
	selected := 0
	for _, ch := range row.Children {
		tappable := slices.Contains(ch.Traits, "Button")
		switch {
		case ch.Subrole == "AXTabButton":
		case ch.Label != "" && tappable && (ch.Checked == nil || signal):
		case ch.Label == "" && ch.Identifier == "" && len(ch.Children) == 0 && !tappable:
			continue
		default:
			return nil
		}
		tabs = append(tabs, ch)
		if slices.Contains(ch.Traits, "Selected") {
			selected++
		}
	}
	if len(tabs) < 2 {
		return nil
	}
	if signal {
		return tabs
	}
	if selected != 1 {
		return nil
	}
	first, last := tabs[0].Frame, tabs[len(tabs)-1].Frame
	for i := 1; i < len(tabs); i++ {
		a, b := tabs[i-1].Frame, tabs[i].Frame
		if b.X < a.X+a.W-1 || math.Abs(b.Y-a.Y) > 4 || math.Abs(b.H-a.H) > 4 {
			return nil
		}
	}
	if last.X+last.W-first.X < screen.W/2 {
		return nil
	}
	if first.Y+first.H < screen.H-tabEdgeBottom && first.Y > statusBar+tabEdgeTop {
		return nil
	}
	return tabs
}

// dropText removes the first descendant of el that is a text labelled label
// with no identifier and nothing below it.
func dropText(el *RawElement, label string) bool {
	for i, ch := range el.Children {
		if ch.Type == "StaticText" && ch.Label == label && ch.Identifier == "" && len(ch.Children) == 0 {
			el.Children = append(el.Children[:i:i], el.Children[i+1:]...)
			return true
		}
		if dropText(ch, label) {
			return true
		}
	}
	return false
}

// withoutIconGlyphs drops Unicode private-use characters, which icon fonts
// (react-native-vector-icons, Material Symbols) draw as glyphs and which
// uiautomator reports as text, so "\U000f0538, Home" names a tab "Home".
func withoutIconGlyphs(s string) string {
	if !strings.ContainsFunc(s, isPrivateUse) {
		return s
	}
	s = strings.Map(func(r rune) rune {
		if isPrivateUse(r) {
			return -1
		}
		return r
	}, s)
	// React Native joins a pressable's children into its label with ", ",
	// so a dropped glyph leaves an empty part behind.
	var parts []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}

func isPrivateUse(r rune) bool {
	return (r >= 0xE000 && r <= 0xF8FF) || (r >= 0xF0000 && r <= 0xFFFFD) || (r >= 0x100000 && r <= 0x10FFFD)
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

// untagged reports whether a window shows Jetpack Compose content that has
// something to act on and no identifier anywhere. Compose publishes testTags as
// resource ids only below Modifier.semantics { testTagsAsResourceId = true },
// and a dialog, bottom sheet or popup is a window of its own that needs it
// again. Compose content is a ComposeView, or a clickable plain View, which
// classic views rarely are.
func untagged(n uiNode) bool {
	compose, actionable, ids := false, false, false
	var walk func(n uiNode)
	walk = func(n uiNode) {
		cls := simpleClass(n.Class)
		clickable := n.Clickable == "true" || n.LongClickable == "true"
		if cls == "composeview" || cls == "view" && clickable {
			compose = true
		}
		actionable = actionable || clickable
		ids = ids || androidIdentifier(n.ResourceID) != ""
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(n)
	return compose && actionable && !ids
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
	// uiautomator dumps only after a second without accessibility events.
	d := &Describe{Screen: size, Root: root, Settled: true, StatusBar: defaultStatusBar, Platform: PlatformAndroid}
	if screen.StatusBarPx > 0 {
		d.StatusBar = math.Round(float64(screen.StatusBarPx)/c.scale*100) / 100
	}
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
		n = prepare(n, false)
		repairSheetBackdrop(&n)
		if foreign == "" && untagged(n) {
			d.Untagged = true
		}
		root.Children = append(root.Children, c.element(n))
	}
	for _, w := range root.Children {
		markTabBars(w, size, d.StatusBar)
	}
	for _, k := range screen.Keyboard {
		if len(root.Children) == 0 {
			break
		}
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

// isNullRoot reports whether err is uiautomator finding no window at all, as
// during a transition or while the display sleeps.
func isNullRoot(err error) bool {
	return err != nil && Code(err) == "ax_unavailable" && strings.Contains(AsError(err).Message, "found no window")
}

var (
	wmSizePattern    = regexp.MustCompile(`(Physical|Override) size: (\d+)x(\d+)`)
	wmDensityPattern = regexp.MustCompile(`(Physical|Override) density: (\d+)`)
	imeFramePattern  = regexp.MustCompile(`\b(?:mFrame|frame)=\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]`)
	// touchable region=SkRegion((21,963,169,1523)(0,2337,1080,2400))
	imeTouchablePattern = regexp.MustCompile(`touchable region=SkRegion\(((?:\(-?\d+,-?\d+,-?\d+,-?\d+\))*)\)`)
	imeRectPattern      = regexp.MustCompile(`\((-?\d+),(-?\d+),(-?\d+),(-?\d+)\)`)
	// mGivenContentInsets=[0,1385][0,0]: left,top then right,bottom
	imeInsetsPattern = regexp.MustCompile(`mGivenContentInsets=\[-?\d+,(-?\d+)\]`)
	// mTouchableInsets=3: 0 frame, 1 content, 2 visible, 3 region
	imeTouchableInsetsPattern = regexp.MustCompile(`mTouchableInsets=(\d+)`)
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

// parseStatusBar reads `dumpsys window StatusBar` and returns the bar's height
// in pixels, or 0 when the output has no frame along the top of the screen.
func parseStatusBar(out string) int {
	for _, m := range imeFramePattern.FindAllStringSubmatch(out, -1) {
		var f [4]int
		for i := range f {
			f[i], _ = strconv.Atoi(m[i+1])
		}
		if f[1] == 0 && f[3] > 0 && f[2] > f[0] && f[3] < f[2] {
			return f[3]
		}
	}
	return 0
}

// parseIMEWindow reads `dumpsys window InputMethod` and returns the parts of
// the keyboard that are on screen, in pixels. Since Android 11 the input
// method window usually spans the whole display and is transparent above the
// keys, so its frame says little: the touchable region (or, failing that, the
// content insets) is where the keyboard is actually drawn. A floating toolbar,
// as Gboard shows when a hardware keyboard is attached, is a small rectangle of
// its own. The format varies across Android versions; anything unrecognized
// reads as no keyboard.
func parseIMEWindow(out string) [][4]int {
	visible := strings.Contains(out, "isVisible=true") ||
		(strings.Contains(out, "mHasSurface=true") && strings.Contains(out, "mViewVisibility=0x0"))
	if !visible {
		return nil
	}
	var frame [4]int
	m := imeFramePattern.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	for i := range frame {
		frame[i], _ = strconv.Atoi(m[i+1])
	}
	if frame[2] <= frame[0] || frame[3] <= frame[1] || frame[1] < 0 {
		return nil
	}
	if t := imeTouchableInsetsPattern.FindStringSubmatch(out); t != nil && t[1] == "0" {
		// The input method takes touches over its whole frame, as in
		// fullscreen extract mode, so the frame is what it covers.
		return [][4]int{frame}
	}
	if region := imeTouchablePattern.FindStringSubmatch(out); region != nil {
		var rects [][4]int
		for _, r := range imeRectPattern.FindAllStringSubmatch(region[1], -1) {
			var f [4]int
			for i := range f {
				f[i], _ = strconv.Atoi(r[i+1])
			}
			if f[2] > f[0] && f[3] > f[1] {
				rects = append(rects, f)
			}
		}
		if len(rects) > 0 {
			return rects
		}
	}
	if c := imeInsetsPattern.FindStringSubmatch(out); c != nil {
		if top, _ := strconv.Atoi(c[1]); top > 0 && frame[1]+top < frame[3] {
			return [][4]int{{frame[0], frame[1] + top, frame[2], frame[3]}}
		}
	}
	if frame[1] <= frame[3]/3 {
		// A frame from the top of the display (or just under the status
		// bar) with no region or insets to narrow it down can't be told
		// apart from a keyboard that isn't drawn.
		return nil
	}
	return [][4]int{frame}
}

// repairSheetBackdrop fixes a Compose modal sheet's bounds. Compose reports
// the sheet's container at its layout position, without the offset that puts
// it at the bottom of the screen, and the dimmed backdrop as the rest of the
// screen: the two tile the window, while the sheet's content sits outside the
// container. A tap at the backdrop's reported center then lands on the sheet.
// The part of the window above the sheet's content is what the backdrop shows.
func repairSheetBackdrop(n *uiNode) {
	for i := range n.Children {
		repairSheetBackdrop(&n.Children[i])
	}
	for i := 0; i+1 < len(n.Children); i++ {
		for _, pair := range [][2]int{{i, i + 1}, {i + 1, i}} {
			scrim, sheet := &n.Children[pair[0]], &n.Children[pair[1]]
			if scrim.Clickable != "true" || len(scrim.Children) > 0 || sheet.Clickable == "true" || len(sheet.Children) == 0 ||
				simpleClass(scrim.Class) != "view" || simpleClass(sheet.Class) != "view" {
				continue
			}
			sb, ok1 := parseBounds(scrim.Bounds)
			pb, ok2 := parseBounds(sheet.Bounds)
			if !ok1 || !ok2 || sb[0] != pb[0] || sb[2] != pb[2] || pb[3] != sb[1] || pb[1] >= sb[1] {
				continue
			}
			top, bottom, found := descendantsSpan(sheet)
			if !found || top <= pb[1] || bottom <= pb[3] {
				continue
			}
			scrim.Bounds = fmt.Sprintf("[%d,%d][%d,%d]", sb[0], pb[1], sb[2], top)
		}
	}
}

func parseBounds(bounds string) ([4]int, bool) {
	var v [4]int
	m := boundsPattern.FindStringSubmatch(bounds)
	if m == nil {
		return v, false
	}
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, v[2] > v[0] && v[3] > v[1]
}

// descendantsSpan returns the topmost and bottommost edge of n's descendants.
func descendantsSpan(n *uiNode) (top, bottom int, found bool) {
	var walk func(*uiNode)
	walk = func(c *uiNode) {
		for i := range c.Children {
			ch := &c.Children[i]
			if b, ok := parseBounds(ch.Bounds); ok {
				if !found || b[1] < top {
					top = b[1]
				}
				if !found || b[3] > bottom {
					bottom = b[3]
				}
				found = true
			}
			walk(ch)
		}
	}
	walk(n)
	return
}
