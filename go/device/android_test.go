package device

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

var pixel = AndroidScreen{WidthPx: 1080, HeightPx: 2400, Density: 420}

func loadUIAutomator(t *testing.T, name, pkg string, screen AndroidScreen) *Describe {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := ParseUIAutomator(data, pkg, screen)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseUIAutomatorViews(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-views", "dev.sightmap.sample", pixel)
	if d.Overlay || d.Screen.W != 411.43 || d.Screen.H != 914.29 {
		t.Fatalf("screen %+v overlay %v; want 411.43x914.29 dp", d.Screen, d.Overlay)
	}
	tree := Normalize(d)
	if tree.Root.Role != "application" {
		t.Errorf("root role = %s", tree.Root.Role)
	}
	email := byIdentifier(t, tree, "login_email")
	if email.Role != "textfield" || email.Name != "Email" || !email.IsInteractive {
		t.Errorf("email = %s %q interactive=%v", email.Role, email.Name, email.IsInteractive)
	}
	if pw := byIdentifier(t, tree, "login_password"); pw.Role != "securetextfield" || pw.Name != "Password" {
		t.Errorf("password = %s %q", pw.Role, pw.Name)
	}
	if remember := byIdentifier(t, tree, "remember"); remember.Role != "checkbox" || remember.State["checked"] != "true" || remember.Name != "Remember me" {
		t.Errorf("remember = %s %q %v", remember.Role, remember.Name, remember.State)
	}
	submit := byIdentifier(t, tree, "login_submit")
	if submit.Role != "button" || submit.Name != "Sign in" || *submit.Bounds != (sightmap.Bounds{X: 24, Y: 328, Width: 363, Height: 48}) {
		t.Errorf("submit = %s %q %+v; want bounds in dp", submit.Role, submit.Name, *submit.Bounds)
	}
	if forgot := byIdentifier(t, tree, "forgot"); Enabled(forgot) || forgot.Element.Attrs["disabled"] != "" {
		t.Errorf("forgot is disabled: state %v attrs %v", forgot.State, forgot.Element.Attrs)
	}
	if list := byIdentifier(t, tree, "recent_list"); list.Role != "collection" {
		t.Errorf("RecyclerView role = %s", list.Role)
	}
	row := byIdentifier(t, tree, "recent_row")
	if row.Role != "other" || !row.IsInteractive || row.Name != "Product 1, $6.37" {
		t.Errorf("a clickable row is an interactive other named by its text, got %s %q interactive=%v", row.Role, row.Name, row.IsInteractive)
	}
	home := byIdentifier(t, tree, "nav_home")
	if home.Name != "Home" || home.State["selected"] != "true" || !home.IsInteractive {
		t.Errorf("nav item = %q %v interactive=%v", home.Name, home.State, home.IsInteractive)
	}
	for _, n := range tree.Nodes {
		if id := Identifier(n); id == "content" || id == "statusBarBackground" || strings.Contains(id, ":id/") {
			t.Errorf("identifier %q should be dropped or unqualified", id)
		}
	}
}

// TestParseUIAutomatorForm reads a hand-written dump in the shape older
// Compose releases reported: widget classes on the elements themselves.
func TestParseUIAutomatorForm(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-form", "dev.sightmap.compose", pixel)
	tree := Normalize(d)
	search := byIdentifier(t, tree, "search")
	if search.Role != "textfield" || search.Name != "Search products" || search.Value != "" {
		t.Errorf("search = %s %q value %q", search.Role, search.Name, search.Value)
	}
	if list := byIdentifier(t, tree, "product_list"); list.Role != "scrollview" {
		t.Errorf("a scrollable Compose View is a scrollview, got %s", list.Role)
	}
	row := byIdentifier(t, tree, "product_row_1")
	if !row.IsInteractive || row.Element.Attrs["testid"] != "product_row_1" || row.Element.Id != "product_row_1" {
		t.Errorf("row = interactive %v attrs %v id %q", row.IsInteractive, row.Element.Attrs, row.Element.Id)
	}
	if sw := byIdentifier(t, tree, "notifications"); sw.Role != "switch" || sw.State["checked"] != "false" || sw.Name != "Notifications" {
		t.Errorf("switch = %s %q %v", sw.Role, sw.Name, sw.State)
	}
	if add := byIdentifier(t, tree, "add_to_cart"); add.Role != "button" || add.Name != "Add to cart" {
		t.Errorf("a Compose Button takes its Text child's text, got %s %q", add.Role, add.Name)
	}
}

// The uiautomator-compose* and uiautomator-views-shop fixtures are real dumps
// of a small shop app with Compose and classic-view screens (Compose UI 1.9,
// Android 15).
func TestParseUIAutomatorCompose(t *testing.T) {
	tree := Normalize(loadUIAutomator(t, "uiautomator-compose", "dev.sightmap.sample", pixel))
	if tree.Untagged {
		t.Error("a screen with testTags is not untagged")
	}
	// A clickable row keeps its texts as children, each with its testTag.
	row := byIdentifier(t, tree, "product.1")
	if row.Role != "other" || row.Name != "Lash Mascara, Essence, $9.99" || !row.IsInteractive || len(row.Children) != 3 {
		t.Errorf("row = %s %q interactive %v, %d children", row.Role, row.Name, row.IsInteractive, len(row.Children))
	}
	if title := row.Children[0]; Identifier(title) != "product.title" || title.Name != "Lash Mascara" {
		t.Errorf("row title = %q %q", Identifier(title), title.Name)
	}
	// A segment is a radio carrying its checked state; Compose puts the role
	// on an empty child, which goes, and leaves the checked segment
	// unclickable, which still counts as interactive and named.
	for id, checked := range map[string]string{"shop.filter.all": "true", "shop.filter.sale": "false", "shop.filter.new": "false"} {
		seg := byIdentifier(t, tree, id)
		if seg.Role != "radio" || seg.State["checked"] != checked || !seg.IsInteractive || seg.Name == "" || len(seg.Children) != 0 {
			t.Errorf("%s = %s %q checked %q interactive %v, %d children", id, seg.Role, seg.Name, seg.State["checked"], seg.IsInteractive, len(seg.Children))
		}
	}
	// The selected bottom navigation item is unclickable too.
	shop := byIdentifier(t, tree, "nav.shop")
	if shop.Name != "Shop" || shop.State["selected"] != "true" || !shop.IsInteractive {
		t.Errorf("nav.shop = %q selected %q interactive %v", shop.Name, shop.State["selected"], shop.IsInteractive)
	}
	// An OutlinedTextField's label is a child text.
	search := byIdentifier(t, tree, "shop.search")
	if search.Role != "textfield" || search.Name != "Search products" || search.Value != "" || len(search.Children) != 0 {
		t.Errorf("search = %s %q value %q, %d children", search.Role, search.Name, search.Value, len(search.Children))
	}
	for _, n := range tree.Nodes {
		if n.IsInteractive && n.Name == "" && Identifier(n) == "" {
			t.Errorf("unnamed interactive node %s %s", n.Id, n.Role)
		}
	}

	settings := Normalize(loadUIAutomator(t, "uiautomator-compose-settings", "dev.sightmap.sample", pixel))
	on := Normalize(loadUIAutomator(t, "uiautomator-compose-settings-on", "dev.sightmap.sample", pixel))
	for id, role := range map[string]string{"settings.notifications": "switch", "settings.dark": "switch", "settings.marketing": "checkbox"} {
		off, sw := byIdentifier(t, settings, id), byIdentifier(t, on, id)
		if off.Role != role || sw.Role != role || off.State["checked"] != "false" || sw.State["checked"] != "true" {
			t.Errorf("%s = %s checked %q, then %s checked %q; want %s", id, off.Role, off.State["checked"], sw.Role, sw.State["checked"], role)
		}
	}
	if n := byIdentifier(t, settings, "settings.notifications"); n.Name != "Notifications" {
		t.Errorf("a toggleable row is named by its text, got %q", n.Name)
	}
	// A Compose Button is one button, named by its text, with no text child.
	signOut := byIdentifier(t, settings, "settings.signout")
	if signOut.Role != "button" || signOut.Name != "Sign out" || len(signOut.Children) != 0 {
		t.Errorf("sign out = %s %q, %d children", signOut.Role, signOut.Name, len(signOut.Children))
	}
	if tab := byIdentifier(t, on, "settings.tab.privacy"); tab.Name != "Privacy" || tab.State["selected"] != "true" || !tab.IsInteractive {
		t.Errorf("selected tab = %q selected %q interactive %v", tab.Name, tab.State["selected"], tab.IsInteractive)
	}

	dialog := Normalize(loadUIAutomator(t, "uiautomator-compose-dialog", "dev.sightmap.sample", pixel))
	if n := byIdentifier(t, dialog, "signout.confirm"); n.Role != "button" || n.Name != "Sign out" {
		t.Errorf("dialog confirm = %s %q", n.Role, n.Name)
	}
}

// A bottom sheet is a window of its own, where the screen's
// testTagsAsResourceId doesn't reach.
func TestParseUIAutomatorUntaggedCompose(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-compose-sheet", "dev.sightmap.sample", pixel)
	if !d.Untagged {
		t.Fatal("a Compose sheet with no ids should say so")
	}
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if text := snap.Render(RenderOpts{}); !strings.Contains(text, "[No identifiers]") || !strings.Contains(text, "testTagsAsResourceId") {
		t.Errorf("snapshot:\n%s", text)
	}
	n, err := Resolve(snap, `[text="Add to cart"]`)
	if err != nil || n.Role != "button" {
		t.Errorf("[text=\"Add to cart\"] = %v, %v; want the one button", n, err)
	}
	if plain := loadUIAutomator(t, "uiautomator-compose-plain", "dev.sightmap.sample", pixel); plain.Untagged {
		t.Error("a screen with some ids is not untagged")
	}
}

func TestParseUIAutomatorViewsShop(t *testing.T) {
	tree := Normalize(loadUIAutomator(t, "uiautomator-views-shop", "dev.sightmap.sample", pixel))
	if tree.Untagged {
		t.Error("classic views are not untagged Compose")
	}
	// MaterialButtonToggleGroup segments are checkable buttons.
	if all := byIdentifier(t, tree, "filter_all"); all.State["checked"] != "true" || all.Name != "All" {
		t.Errorf("filter_all = %q checked %q", all.Name, all.State["checked"])
	}
	if sale := byIdentifier(t, tree, "filter_sale"); sale.State["checked"] != "false" {
		t.Errorf("filter_sale checked %q", sale.State["checked"])
	}
	nav := byIdentifier(t, tree, "nav_views")
	if nav.Name != "Views" || nav.State["selected"] != "true" {
		t.Errorf("nav_views = %q selected %q", nav.Name, nav.State["selected"])
	}
	for _, n := range tree.Nodes {
		id := Identifier(n)
		if strings.HasPrefix(id, "navigation_bar_item_") || id == "action_bar_root" {
			t.Errorf("library id %q reached the tree", id)
		}
		if n != nav && tree.IsAncestor(nav, n) && n.State["selected"] == "true" {
			t.Errorf("%s %s under the selected item is marked selected too", n.Id, n.Role)
		}
	}
	rows := 0
	for _, n := range tree.Nodes {
		if Identifier(n) == "product_row" {
			rows++
		}
	}
	if rows != 7 {
		t.Errorf("%d product_row rows, want 7 (a RecyclerView repeats its item layout's ids)", rows)
	}
}

func TestAnnotateAndroidInfersViews(t *testing.T) {
	corpus := loadCorpus(t, "android-corpus")
	snap, err := Annotate(Normalize(loadUIAutomator(t, "uiautomator-views", "dev.sightmap.sample", pixel)), corpus, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ViewName() != "Login" || !snap.ViewInferred {
		t.Fatalf("view = %q inferred %v", snap.ViewName(), snap.ViewInferred)
	}
	n, err := Resolve(snap, "SignInButton")
	if err != nil || Identifier(n) != "login_submit" {
		t.Fatalf("SignInButton = %v, %v", n, err)
	}
	if _, err := Resolve(snap, `RecentRow[title^="Product 2"]`); err != nil {
		t.Errorf("RecentRow by title: %v", err)
	}

	snap, err = Annotate(Normalize(loadUIAutomator(t, "uiautomator-form", "dev.sightmap.compose", pixel)), corpus, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ViewName() != "Products" {
		t.Fatalf("view = %q", snap.ViewName())
	}
	if n, err := Resolve(snap, `ProductRow[title^="Product 2"]`); err != nil || Identifier(n) != "product_row_2" {
		t.Errorf("ProductRow = %v, %v", n, err)
	}
}

func TestParseUIAutomatorForeignWindowIsAnOverlay(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-permission", "dev.sightmap.sample", pixel)
	if !d.Overlay || d.Root.Label != "com.google.android.permissioncontroller" {
		t.Fatalf("overlay %v root %q", d.Overlay, d.Root.Label)
	}
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := Resolve(snap, `button[text="Allow"]`); err != nil || Identifier(n) != "permission_allow_button" {
		t.Errorf("Allow = %v, %v", n, err)
	}
}

// wantTabs checks that bar is a tabbar whose tabs are the elements with ids,
// each interactive and named, and only selected selected.
func wantTabs(t *testing.T, tree *Tree, bar, selected string, ids ...string) {
	t.Helper()
	if b := byIdentifier(t, tree, bar); b.Role != "tabbar" {
		t.Errorf("%s = %s, want tabbar", bar, b.Role)
	}
	for _, id := range ids {
		tab := byIdentifier(t, tree, id)
		if tab.Role != "tab" || tab.Name == "" || tab.Value != "" || !tab.IsInteractive || tab.State["selected"] != strconv.FormatBool(id == selected) {
			t.Errorf("%s = %s %q value %q interactive %v state %v; want a tab, selected %v", id, tab.Role, tab.Name, tab.Value, tab.IsInteractive, tab.State, id == selected)
		}
		if _, ok := tab.State["checked"]; ok {
			t.Errorf("%s carries checked %q; a tab is selected, not checked", id, tab.State["checked"])
		}
		if !tree.hasAncestor(tab, func(a *sightmap.ComponentNode) bool { return Identifier(a) == bar }) {
			t.Errorf("%s is not inside %s", id, bar)
		}
	}
}

// A Material Components navigation item is a FrameLayout, checkable in some
// releases; its library child ids make it a tab, wherever the bar is.
func TestAndroidMaterialNavigationItemsAreTabs(t *testing.T) {
	views := Normalize(loadUIAutomator(t, "uiautomator-views", "dev.sightmap.sample", pixel))
	wantTabs(t, views, "bottom_nav", "nav_home", "nav_home", "nav_account")

	shop := Normalize(loadUIAutomator(t, "uiautomator-views-shop", "dev.sightmap.sample", pixel))
	wantTabs(t, shop, "bottom_nav", "nav_views", "nav_views", "nav_compose")
	// The item's label is its content description; its text child says the
	// same and goes, so [text="Views"] is the tab alone.
	for _, n := range shop.Nodes {
		if n.Role == "text" && n.Name == "Views" {
			t.Errorf("text %s %q is left under the tab", n.Id, n.Name)
		}
	}
	if bar := byIdentifier(t, shop, "bottom_nav"); bar.Bounds.X != 0 || bar.Bounds.Width != 411 {
		t.Errorf("the bar is the whole BottomNavigationView, got %+v", *bar.Bounds)
	}

	// A NavigationRailView stacks the same items down the screen's side.
	item := func(id, label string, y int, selected bool) string {
		state := `clickable="true"`
		if selected {
			state = `selected="true"`
		}
		return fmt.Sprintf(`<node class="android.widget.FrameLayout" package="x" resource-id="x:id/%s" content-desc="%s" focusable="true" enabled="true" %s bounds="[0,%d][210,%d]">`+
			`<node class="android.widget.ImageView" package="x" resource-id="x:id/navigation_bar_item_icon_view" bounds="[73,%d][136,%d]"/>`+
			`<node class="android.widget.TextView" package="x" resource-id="x:id/navigation_bar_item_large_label_view" text="%s" bounds="[60,%d][150,%d]"/></node>`,
			id, label, state, y, y+168, y+20, y+83, label, y+100, y+150)
	}
	rail := `<hierarchy rotation="0"><node class="android.widget.FrameLayout" package="x" bounds="[0,0][1080,2400]">` +
		`<node class="android.widget.FrameLayout" package="x" resource-id="x:id/rail" bounds="[0,400][210,904]">` +
		item("rail_inbox", "Inbox", 400, true) + item("rail_sent", "Sent", 568, false) + item("rail_drafts", "Drafts", 736, false) +
		`</node></node></hierarchy>`
	d, err := ParseUIAutomator([]byte(rail), "x", pixel)
	if err != nil {
		t.Fatal(err)
	}
	wantTabs(t, Normalize(d), "rail", "rail_inbox", "rail_inbox", "rail_sent", "rail_drafts")
}

// tabRowDump is a row of three items with its top at y px, the one at
// selected (if any) selected and, as Compose and TabLayout report it, not
// clickable.
func tabRowDump(class string, y, selected int) []byte {
	item := func(id, label string, x int, selected bool) string {
		attrs := `clickable="true"`
		if selected {
			attrs = `selected="true"`
		}
		return fmt.Sprintf(`<node class="android.view.View" package="x" resource-id="%s" focusable="true" enabled="true" %s bounds="[%d,%d][%d,%d]">`+
			`<node class="android.widget.TextView" package="x" text="%s" enabled="true" bounds="[%d,%d][%d,%d]"/></node>`,
			id, attrs, x, y, x+360, y+126, label, x+120, y+36, x+240, y+90)
	}
	return []byte(fmt.Sprintf(`<hierarchy rotation="0"><node class="android.widget.FrameLayout" package="x" bounds="[0,0][1080,2400]">`+
		`<node class="%s" package="x" resource-id="range" enabled="true" bounds="[0,%d][1080,%d]">%s%s%s</node></node></hierarchy>`,
		class, y, y+126, item("range.day", "Day", 0, selected == 0), item("range.week", "Week", 360, selected == 1), item("range.month", "Month", 720, selected == 2)))
}

// Compose's Role.Tab, a current TabLayout tab and a React Native tab show no
// tab class in a dump, so a row of them is known by its layout. A row like
// it in the middle of the screen is a segmented control, not tabs.
func TestAndroidTabRowsByLayout(t *testing.T) {
	compose := Normalize(loadUIAutomator(t, "uiautomator-compose", "dev.sightmap.sample", pixel))
	wantTabs(t, compose, "nav", "nav.shop", "nav.shop", "nav.plain", "nav.settings", "nav.views")
	settings := Normalize(loadUIAutomator(t, "uiautomator-compose-settings", "dev.sightmap.sample", pixel))
	wantTabs(t, settings, "settings.tabs", "settings.tab.general", "settings.tab.general", "settings.tab.privacy")
	wantTabs(t, settings, "nav", "nav.settings", "nav.shop", "nav.plain", "nav.settings", "nav.views")
	// Segments are checked, not selected: they stay radios.
	for _, id := range []string{"shop.filter.all", "shop.filter.sale", "shop.filter.new"} {
		if n := byIdentifier(t, compose, id); n.Role != "radio" {
			t.Errorf("segment %s = %s, want radio", id, n.Role)
		}
	}
	if n := byIdentifier(t, compose, "shop.filter"); n.Role == "tabbar" {
		t.Error("a segmented button row is not a tab bar")
	}

	top, err := ParseUIAutomator(tabRowDump("android.view.View", 160, 0), "x", pixel)
	if err != nil {
		t.Fatal(err)
	}
	wantTabs(t, Normalize(top), "range", "range.day", "range.day", "range.week", "range.month")
	// Mid-screen, or a row of buttons with nothing selected (a dialog's
	// actions along the bottom), is not a tab bar.
	var tree *Tree
	for _, c := range []struct {
		name        string
		y, selected int
	}{{"a selectable row mid-screen", 1000, 0}, {"a bottom row with nothing selected", 2200, -1}} {
		d, err := ParseUIAutomator(tabRowDump("android.view.View", c.y, c.selected), "x", pixel)
		if err != nil {
			t.Fatal(err)
		}
		tree = Normalize(d)
		for _, n := range tree.Nodes {
			if n.Role == "tab" || n.Role == "tabbar" {
				t.Errorf("%s gave %s %s %q", c.name, n.Id, n.Role, n.Name)
			}
		}
	}
	mid, err := ParseUIAutomator(tabRowDump("android.view.View", 1000, 0), "x", pixel)
	if err != nil {
		t.Fatal(err)
	}
	tree = Normalize(mid)
	if day := byIdentifier(t, tree, "range.day"); day.Role != "other" || day.State["selected"] != "true" || !day.IsInteractive {
		t.Errorf("range.day = %s selected %q interactive %v", day.Role, day.State["selected"], day.IsInteractive)
	}
}

// A TabHost's TabWidget says it holds tabs, wherever it is.
func TestAndroidTabWidgetHoldsTabs(t *testing.T) {
	d, err := ParseUIAutomator(tabRowDump("android.widget.TabWidget", 1000, 0), "x", pixel)
	if err != nil {
		t.Fatal(err)
	}
	wantTabs(t, Normalize(d), "range", "range.day", "range.day", "range.week", "range.month")
}

// Tabs match a cross-platform TabBarItem component, and the bar covers what
// scrolls under it, even between two tabs, where no tab is drawn.
func TestAndroidTabBarMatchesAndCovers(t *testing.T) {
	dir := t.TempDir()
	corpus := "version: 1\ncomponents:\n  - name: TabBarItem\n    selector: 'tab'\n    properties:\n      - name: title\n        extract: { from: dom.text }\nviews:\n  - name: Shop\n    route: /shop\n"
	if err := os.WriteFile(filepath.Join(dir, "screens.yaml"), []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "uiautomator-compose.xml"))
	if err != nil {
		t.Fatal(err)
	}
	// An edge-to-edge list scrolled so its last row sits behind the bar.
	data = bytes.Replace(data, []byte(`bounds="[0,689][1080,2127]"`), []byte(`bounds="[0,689][1080,2400]"`), 1)
	data = bytes.Replace(data, []byte(`bounds="[0,1967][1080,2127]"`), []byte(`bounds="[0,2150][1080,2310]"`), 1)
	d, err := ParseUIAutomator(data, "dev.sightmap.sample", pixel)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Annotate(Normalize(d), c, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	tab, err := Resolve(snap, `TabBarItem[title="Settings"]`)
	if err != nil || Identifier(tab) != "nav.settings" {
		t.Fatalf("TabBarItem Settings = %v, %v", tab, err)
	}
	if err := snap.AssertActionable(tab, "TabBarItem"); err != nil {
		t.Errorf("a tab is not covered by its own bar: %v", err)
	}
	row := byIdentifier(t, snap.Tree, "product.7")
	if err := snap.AssertActionable(row, "product.7"); Code(err) != "covered" || !strings.Contains(err.Error(), "tabbar") {
		t.Errorf("a row behind the tab bar: %v", err)
	}
}

func TestParseUIAutomatorErrorsAndRotation(t *testing.T) {
	_, err := ParseUIAutomator([]byte("ERROR: could not get idle state.\n"), "x", pixel)
	if Code(err) != "ax_unavailable" {
		t.Errorf("idle-state error = %v (%s)", err, Code(err))
	}
	_, err = ParseUIAutomator([]byte("ERROR: UiAutomationService android.accessibilityservice.IAccessibilityServiceClient@1 already registered!\n"), "x", pixel)
	if Code(err) != "ax_error" || !strings.Contains(AsError(err).Hint, "Appium") {
		t.Errorf("already-registered error = %+v", AsError(err))
	}
	d, err := ParseUIAutomator([]byte(`<hierarchy rotation="1"><node class="android.widget.Button" package="x" text="Go" clickable="true" enabled="true" bounds="[0,0][420,210]"/></hierarchy>`), "x", pixel)
	if err != nil {
		t.Fatal(err)
	}
	if d.Screen.W != 914.29 || d.Screen.H != 411.43 {
		t.Errorf("landscape screen = %+v", d.Screen)
	}
	if b := d.Root.Children[0].Frame; b.W != 160 || b.H != 80 {
		t.Errorf("frame = %+v, want 160x80 dp", b)
	}
}

func TestParseWMAndIME(t *testing.T) {
	s := parseWM("Physical size: 1080x2400\nOverride size: 720x1600\n", "Physical density: 420\n")
	if s.WidthPx != 720 || s.HeightPx != 1600 || s.Density != 420 {
		t.Errorf("wm = %+v", s)
	}
	ime := `WINDOW MANAGER WINDOWS (dumpsys window windows)
  Window #2 Window{6c1f2a u0 InputMethod}:
    mDisplayId=0 rootTaskId=1 mSession=Session{8a1 1234:u0a10115} mClient=android.os.BinderProxy@1
    mOwnerUid=10115 showForAllUsers=true package=com.google.android.inputmethod.latin appop=NONE
    mHasSurface=true isReadyForDisplay()=true mWindowRemovalAllowed=false
    Frames: parent=[0,0][1080,2400] display=[0,0][1080,2400] frame=[0,1527][1080,2400] last=[0,1527][1080,2400] insetsChanged=false
    isOnScreen=true isVisible=true
`
	k := parseIMEWindow(ime)
	if len(k) != 1 || k[0] != [4]int{0, 1527, 1080, 2400} {
		t.Fatalf("ime frame = %v", k)
	}
	if parseIMEWindow(strings.ReplaceAll(ime, "isVisible=true", "isVisible=false")) != nil {
		t.Error("a hidden input method is no keyboard")
	}
	screen := pixel
	screen.Keyboard = k
	d := loadUIAutomator(t, "uiautomator-form", "dev.sightmap.compose", screen)
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	add, err := Resolve(snap, "add_to_cart")
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.AssertActionable(add, "add_to_cart"); Code(err) != "covered" {
		t.Errorf("a button under the keyboard is covered, got %v", err)
	}
}

// The input method window on Android 15 spans the display and is transparent
// above the keys; the dumps come from Gboard on an API 35 emulator.
func TestParseIMEWindowFullDisplayFrame(t *testing.T) {
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if k := parseIMEWindow(read("ime-gboard-api35.txt")); len(k) != 1 || k[0] != [4]int{0, 1517, 1080, 2400} {
		t.Errorf("full keyboard = %v, want the keys only, not the window frame", k)
	}
	floating := parseIMEWindow(read("ime-gboard-floating-api35.txt"))
	if len(floating) != 2 || floating[0] != [4]int{21, 963, 169, 1523} || floating[1] != [4]int{0, 2337, 1080, 2400} {
		t.Errorf("floating toolbar = %v", floating)
	}
	if k := parseIMEWindow(read("ime-hidden-api35.txt")); k != nil {
		t.Errorf("hidden keyboard = %v", k)
	}
	insetsOnly := regexp.MustCompile(`touchable region=.*\n`).ReplaceAllString(read("ime-gboard-api35.txt"), "")
	if k := parseIMEWindow(insetsOnly); len(k) != 1 || k[0] != [4]int{0, 132 + 1385, 1080, 2400} {
		t.Errorf("keyboard from content insets = %v", k)
	}

	screen := AndroidScreen{WidthPx: 1080, HeightPx: 2400, Density: 420, Keyboard: floating}
	d := loadUIAutomator(t, "uiautomator-pixel9a", "", screen)
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// NONE spans the toolbar's rows but its center is clear of it.
	none, err := Resolve(snap, `[text="NONE"]`)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.AssertActionable(none, "NONE"); err != nil {
		t.Errorf("a button beside a floating keyboard toolbar is actionable, got %v", err)
	}
}

// fakeADB answers adb commands from a table of command prefixes and records
// every command.
type fakeADB struct {
	answers map[string]string
	calls   []string
}

func (f *fakeADB) run(_ context.Context, args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	best := ""
	for prefix := range f.answers {
		if strings.Contains(cmd, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	return f.answers[best], nil
}

func (f *fakeADB) called(substr string) string {
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return c
		}
	}
	return ""
}

func newFakeADB(t *testing.T, dump string) *fakeADB {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", dump+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	return &fakeADB{answers: map[string]string{
		"wm size":          "Physical size: 1080x2400\n",
		"wm density":       "Physical density: 420\n",
		"uiautomator":      string(data) + "\n" + imeMarker + "\n",
		"version":          "Android Debug Bridge version 1.0.41\nVersion 35.0.2\n",
		"get-state":        "device\n",
		"resolve-activity": "priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=false\ndev.sightmap.compose/.MainActivity\n",
		"pidof":            "4321\n",
		"am start":         "Starting: Intent { cmp=dev.sightmap.compose/.MainActivity }\nStatus: ok\n",
	}}
}

func TestADBHelperInputInPixels(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	h := NewADBHelper(ADB{Run: f.run}, "emulator-5554", "dev.sightmap.compose", nil)
	ctx := context.Background()
	if err := h.Request(ctx, "tap", map[string]any{"x": 100.0, "y": 200.0}, nil); err != nil {
		t.Fatal(err)
	}
	if c := f.called("input tap"); c != "-s emulator-5554 shell input tap 263 525" {
		t.Errorf("tap = %q", c)
	}
	h.Request(ctx, "swipe", map[string]any{"x1": 200.0, "y1": 600.0, "x2": 200.0, "y2": 200.0, "duration": 0.5}, nil)
	if c := f.called("input swipe"); !strings.HasSuffix(c, "input swipe 525 1575 525 525 500") {
		t.Errorf("swipe = %q", c)
	}
	h.Request(ctx, "type", map[string]any{"text": "it's a test"}, nil)
	if c := f.called("input text"); !strings.HasSuffix(c, `input text 'it'\''s%sa%stest'`) {
		t.Errorf("type = %q", c)
	}
	if err := h.Request(ctx, "type", map[string]any{"text": "café"}, nil); Code(err) != "bad_request" {
		t.Errorf("non-ASCII text = %v", err)
	}
	h.Request(ctx, "key", map[string]any{"key": "return"}, nil)
	h.Request(ctx, "button", map[string]any{"name": "back"}, nil)
	if f.called("input keyevent 66") == "" || f.called("input keyevent 4") == "" {
		t.Errorf("keys = %v", f.calls)
	}
	if err := h.Request(ctx, "button", map[string]any{"name": "siri"}, nil); Code(err) != "bad_request" || !strings.Contains(err.Error(), "app_switch") {
		t.Errorf("unknown button = %v", err)
	}
	var d Describe
	if err := h.Request(ctx, "describe", map[string]any{"udid": "emulator-5554", "pid": 4321}, &d); err != nil || d.Root == nil || len(d.Root.Children) == 0 {
		t.Fatalf("describe = %+v, %v", d, err)
	}
	if c := f.called("uiautomator"); !strings.Contains(c, "rm -f "+h.dumpPath) || !strings.Contains(c, "dumpsys window InputMethod") {
		t.Errorf("describe command = %q", c)
	}
}

func TestADBHelperTypesANewlineAsReturn(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	h := NewADBHelper(ADB{Run: f.run}, "emulator-5554", "dev.sightmap.compose", nil)
	if err := h.Request(context.Background(), "type", map[string]any{"text": "first line\n\nthird"}, nil); err != nil {
		t.Fatal(err)
	}
	var typed []string
	for _, c := range f.calls {
		if strings.Contains(c, "shell input") {
			typed = append(typed, strings.TrimPrefix(c, "-s emulator-5554 shell "))
		}
	}
	want := "input text 'first%sline'|input keyevent 66|input keyevent 66|input text 'third'"
	if got := strings.Join(typed, "|"); got != want {
		t.Errorf("typed %q, want %q", got, want)
	}
	if err := h.Request(context.Background(), "type", map[string]any{"text": "a\tb"}, nil); Code(err) != "bad_request" {
		t.Errorf("a tab = %v, want bad_request", err)
	}
}

func TestADBHelperDumpsToItsOwnFile(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	h := NewADBHelper(ADB{Run: f.run}, "emulator-5554", "dev.sightmap.compose", nil)
	var d Describe
	if err := h.Request(context.Background(), "describe", nil, &d); err != nil {
		t.Fatal(err)
	}
	p := fmt.Sprintf("/data/local/tmp/sightmap-%d-window.xml", os.Getpid())
	c := f.called("uiautomator")
	// Removed before the dump (a failed one can't read back as the screen) and
	// after reading it.
	if !strings.Contains(c, "uiautomator dump "+p) || strings.Count(c, "rm -f "+p) != 2 || strings.LastIndex(c, "rm -f "+p) < strings.Index(c, "cat "+p) {
		t.Errorf("describe command = %q", c)
	}
}

func TestAndroidSessionClicksInPixels(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "android-corpus"))); err != nil {
		t.Fatal(err)
	}
	f := newFakeADB(t, "uiautomator-form")
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: dir},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond}.withDefaults()
	r, err := handle(t, s, "click", TargetArgs{Target: "AddToCart"})
	if err != nil {
		t.Fatal(err)
	}
	// add_to_cart is [63,2169][1017,2316] px: 24,826 363x56 dp, center 205,854 dp.
	if c := f.called("input tap"); !strings.HasSuffix(c, "input tap 538 2242") {
		t.Errorf("tap = %q (result %v)", c, r)
	}
	entries, err := ReadRun(s.RunLog.Current().Path, 0)
	if err != nil || len(entries) != 1 || entries[0].View != "Products" || entries[0].Target.Component != "AddToCart" {
		t.Errorf("run log = %+v, %v", entries, err)
	}
}

func TestAndroidSimLaunch(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	sim := AndroidSim{ADB: ADB{Run: f.run}}
	pid, err := sim.Launch(context.Background(), "emulator-5554", "dev.sightmap.compose", LaunchOptions{Terminate: true})
	if err != nil || pid != 4321 {
		t.Fatalf("pid %d, %v", pid, err)
	}
	stop, start := f.called("am force-stop"), f.called("am start")
	if stop == "" || !strings.Contains(start, "-n 'dev.sightmap.compose/.MainActivity'") {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestParseADBDevicesAndPick(t *testing.T) {
	devices := parseADBDevices(`List of devices attached
emulator-5554          device product:sdk_gphone64_arm64 model:sdk_gphone64_arm64 device:emu64a transport_id:1
R5CT1234ABC            unauthorized usb:1-1 transport_id:3
`)
	if len(devices) != 2 || devices[0].Serial != "R5CT1234ABC" || devices[1].Model != "sdk_gphone64_arm64" {
		t.Fatalf("devices = %+v", devices)
	}
	devices[1].AVD, devices[1].Release = "Pixel_8_API_35", "15"
	d, err := PickAndroidDevice(devices, "")
	if err != nil || d.Serial != "emulator-5554" || d.Label() != "Pixel_8_API_35, Android 15" {
		t.Errorf("default pick = %+v, %v", d, err)
	}
	if d, err := PickAndroidDevice(devices, "Pixel_8_API_35"); err != nil || d.Serial != "emulator-5554" {
		t.Errorf("pick by AVD = %+v, %v", d, err)
	}
	if _, err := PickAndroidDevice(devices, "R5CT1234ABC"); Code(err) != "not_booted" {
		t.Errorf("an unauthorized device = %v", err)
	}
	if _, err := PickAndroidDevice(devices, "Pixel_9"); Code(err) != "no_device" {
		t.Errorf("unknown device = %v", err)
	}
	if _, err := PickAndroidDevice(nil, ""); Code(err) != "no_device" {
		t.Errorf("no devices = %v", err)
	}
}

func TestParseLogcatLine(t *testing.T) {
	pid, e, ok := parseLogcatLine("10-29 15:31:15.495  4321  4350 I CartRepository: POST /cart -> 200")
	if !ok || pid != 4321 || e.Level != "info" || e.Subsystem != "CartRepository" || e.Message != "POST /cart -> 200" || e.Source != "logcat" {
		t.Errorf("entry = %d %+v %v", pid, e, ok)
	}
	_, e, _ = parseLogcatLine("10-29 15:31:15.501  4321  4321 I System.out: hello from println")
	if e.Level != "stdout" || e.Message != "hello from println" {
		t.Errorf("System.out = %+v", e)
	}
	_, e, _ = parseLogcatLine("10-29 15:31:16.002  4321  4321 E AndroidRuntime: FATAL EXCEPTION: main")
	if e.Level != "error" || e.Subsystem != "AndroidRuntime" {
		t.Errorf("crash = %+v", e)
	}
	if _, _, ok := parseLogcatLine("--------- beginning of main"); ok {
		t.Error("a logcat banner is not an entry")
	}
}

func TestNetworkIsRefusedOnAndroid(t *testing.T) {
	s := &Session{Config: Config{Platform: PlatformAndroid}}
	if err := s.networkSupported(); Code(err) != "unsupported" {
		t.Errorf("err = %v", err)
	}
	if err := (&Session{}).networkSupported(); err != nil {
		t.Errorf("iOS err = %v", err)
	}
}

// uiautomator-pixel9a.xml is a real dump of a sample app's
// click screen, taken on a Pixel 9a (Android 17).
func TestParseUIAutomatorRealDump(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-pixel9a", "com.example.sampleapp", AndroidScreen{WidthPx: 1080, HeightPx: 2424, Density: 420})
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Overlay || !snap.Settled || snap.Coverage.Total != 16 {
		t.Errorf("overlay %v settled %v interactive %d; want the app's tree, settled, 16 interactive", d.Overlay, snap.Settled, snap.Coverage.Total)
	}
	for target, want := range map[string]string{
		"buttonClick":                   "JUST CLICK",
		"buttonClickAndLong4":           "CLICK AND LONG CLICK 4",
		`[text="NOTHING"]`:              "NOTHING",
		`[identifier="toolbar"] button`: "",
	} {
		n, err := Resolve(snap, target)
		if err != nil || n.Role != "button" || n.Name != want {
			t.Errorf("%s = %+v, %v; want button %q", target, n, err, want)
		}
	}
}

func TestAndroidClickReadsTheScreenOnce(t *testing.T) {
	dir := t.TempDir()
	f := newFakeADB(t, "uiautomator-form")
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: dir},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond}.withDefaults()
	if _, err := handle(t, s, "click", TargetArgs{Target: "add_to_cart"}); err != nil {
		t.Fatal(err)
	}
	dumps := 0
	for _, c := range f.calls {
		if strings.Contains(c, "uiautomator dump") {
			dumps++
		}
	}
	if dumps != 1 {
		t.Errorf("click took %d dumps; uiautomator waits for an idle screen, so one is enough", dumps)
	}
}

func TestScrollUntilStopsOnTheFirstOfSeveralMatches(t *testing.T) {
	dir := t.TempDir()
	f := newFakeADB(t, "uiautomator-form")
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: dir},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond, ScrollSettle: time.Millisecond}.withDefaults()
	r, err := handle(t, s, "scroll", ScrollArgs{Direction: "down", Until: `[text*="Product 2"]`})
	if err != nil {
		t.Fatal(err)
	}
	if r["matches"] != 2.0 || r["scrolls"] != 0.0 || f.called("input swipe") != "" {
		t.Errorf("result %v, swipe %q; want the first of 2 matches without scrolling", r, f.called("input swipe"))
	}

	_, err = handle(t, s, "scroll", ScrollArgs{Direction: "down", Until: `[text="Product"]`, Max: 3})
	if Code(err) != "not_found" || !strings.Contains(AsError(err).Hint, `[text*="Product"]`) {
		t.Errorf("an exact [text=] miss hints at text*=: %+v", AsError(err))
	}
}

func TestAndroidStatusBarComesFromTheDevice(t *testing.T) {
	bar := `  Window #12 Window{9e1c0d u0 StatusBar}:
    mDisplayId=0 rootTaskId=1 mSession=Session{4b2 1890:u0a10204} mClient=android.os.BinderProxy@2
    mOwnerUid=10204 showForAllUsers=true package=com.android.systemui appop=NONE
    Frames: parent=[0,0][1080,152] display=[0,0][1080,2424] frame=[0,0][1080,152] last=[0,0][1080,152] insetsChanged=false
`
	if px := parseStatusBar(bar); px != 152 {
		t.Fatalf("status bar = %d px, want 152", px)
	}
	if parseStatusBar("Window #1 Window{1 u0 NavigationBar0}:\n    frame=[0,2361][1080,2424]") != 0 {
		t.Error("a bar along the bottom is not the status bar")
	}
	// A toolbar button just below a 24 dp status bar: iOS's 60 pt guess for a
	// tall screen would refuse it.
	dump := []byte(`<hierarchy rotation="0"><node class="android.widget.FrameLayout" package="x" bounds="[0,0][1080,2400]"><node class="android.widget.ImageButton" package="x" resource-id="x:id/nav" clickable="true" enabled="true" bounds="[0,63][147,210]"/></node></hierarchy>`)
	for _, px := range []int{0, 63} {
		screen := pixel
		screen.StatusBarPx = px
		d, err := ParseUIAutomator(dump, "x", screen)
		if err != nil {
			t.Fatal(err)
		}
		snap, _ := Annotate(Normalize(d), nil, nil, "")
		nav, _ := Resolve(snap, "nav")
		if err := snap.AssertActionable(nav, "nav"); err != nil {
			t.Errorf("status bar %d px: the toolbar button is refused: %v", px, err)
		}
	}
	screen := pixel
	screen.StatusBarPx = 152
	d, _ := ParseUIAutomator(dump, "x", screen)
	snap, _ := Annotate(Normalize(d), nil, nil, "")
	nav, _ := Resolve(snap, "nav")
	if err := snap.AssertActionable(nav, "nav"); Code(err) != "covered" {
		t.Errorf("a button under a 152 px status bar is covered, got %v", err)
	}
}

func newAndroidTestSession(t *testing.T, f *fakeADB, timing Timing) *Session {
	t.Helper()
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: t.TempDir()},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = timing.withDefaults()
	return s
}

// A settled read already waited for the screen to go idle, so a screen that
// changed on the read before is acted on without the hold-still wait.
func TestAndroidSettledReadSkipsTheQuietWait(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	s := newAndroidTestSession(t, f, Timing{Quiet: time.Minute, Settle: 2 * time.Second, RetryTimeout: 2 * time.Second,
		RetryInterval: time.Millisecond, ScrollSettle: time.Millisecond})
	for _, step := range []struct {
		cmd  string
		args any
	}{
		{"click", TargetArgs{Target: "add_to_cart"}},
		{"scroll", ScrollArgs{Direction: "down"}},
	} {
		f.calls = nil
		started := time.Now()
		if _, err := handle(t, s, step.cmd, step.args); err != nil {
			t.Fatalf("%s: %v", step.cmd, err)
		}
		if took := time.Since(started); took > time.Second {
			t.Errorf("%s took %s; a settled read needs no hold-still wait", step.cmd, took)
		}
		if f.called("input tap") == "" && f.called("input swipe") == "" {
			t.Errorf("%s sent no input: %v", step.cmd, f.calls)
		}
	}
}

// A switch is pressed with a stationary swipe at the trailing end of its row;
// on Android the whole row toggles it, and 150 ms is well under a long press.
func TestAndroidSwitchIsPressedAtItsTrailingEnd(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	f.answers["uiautomator"] = `<hierarchy rotation="0"><node class="android.widget.FrameLayout" package="dev.sightmap.compose" bounds="[0,0][1080,2400]">` +
		`<node class="android.widget.Switch" package="dev.sightmap.compose" resource-id="dev.sightmap.compose:id/notifications" text="Notifications" checkable="true" checked="false" clickable="true" enabled="true" bounds="[0,400][1080,547]"/>` +
		`</node></hierarchy>` + "\n" + imeMarker + "\n"
	s := newAndroidTestSession(t, f, Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond})
	if _, err := handle(t, s, "click", TargetArgs{Target: "notifications"}); err != nil {
		t.Fatal(err)
	}
	var x1, y1, x2, y2, ms int
	c := f.called("input swipe")
	if c == "" {
		t.Fatalf("a switch is pressed with a stationary swipe: %v", f.calls)
	}
	if _, err := fmt.Sscanf(c[strings.Index(c, "input swipe"):], "input swipe %d %d %d %d %d", &x1, &y1, &x2, &y2, &ms); err != nil {
		t.Fatalf("swipe = %q: %v", c, err)
	}
	// 1080 px is 411 dp; 44 dp in from the end is 367 dp, 963 px.
	if x1 != x2 || y1 != y2 || ms != 150 || x1 < 900 || x1 > 1080 || y1 < 400 || y1 > 547 {
		t.Errorf("switch press = %q; want a 150 ms press at the row's trailing end", c)
	}
	if f.called("input tap") != "" {
		t.Errorf("a switch is pressed, not tapped: %v", f.calls)
	}
}

// Only the iOS helper reports an empty tree as ax_disabled, the code a sheet
// going away is waited out on. A screen uiautomator can't read is retried by
// the helper itself and then reported, not re-read for OverlaySettle.
func TestAndroidUnreadableScreenIsNotWaitedOutAsASheet(t *testing.T) {
	f := newFakeADB(t, "uiautomator-form")
	s := newAndroidTestSession(t, f, Timing{OverlaySettle: time.Minute, RetryInterval: time.Millisecond})
	if _, err := s.fresh(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	f.answers["uiautomator"] = "ERROR: null root node returned by UiTestAutomationBridge.\n" + imeMarker + "\n"
	f.calls = nil
	started := time.Now()
	_, err := s.fresh(context.Background(), "")
	if Code(err) != "ax_unavailable" || time.Since(started) > 10*time.Second {
		t.Errorf("err = %v after %s; want ax_unavailable without waiting out OverlaySettle", err, time.Since(started))
	}
	if c := f.called("uiautomator"); !strings.Contains(c, "uiautomator dump") {
		t.Errorf("no dump: %v", f.calls)
	}
}

func TestEmulatorEnv(t *testing.T) {
	env := []string{"HOME=/Users/x", "QT_QPA_PLATFORM=wayland", "PATH=/bin"}
	if got := emulatorEnv(env, "darwin"); strings.Join(got, " ") != "HOME=/Users/x PATH=/bin" {
		t.Errorf("darwin env = %v", got)
	}
	if got := emulatorEnv([]string{"QT_QPA_PLATFORM=cocoa"}, "darwin"); len(got) != 1 {
		t.Errorf("cocoa is kept, got %v", got)
	}
	if got := emulatorEnv(env, "linux"); len(got) != 3 {
		t.Errorf("linux env is untouched, got %v", got)
	}
}

func TestEmulatorExitedQuotesTheFatalLine(t *testing.T) {
	log := filepath.Join(t.TempDir(), "emu.log")
	os.WriteFile(log, []byte("INFO | Checking system compatibility\nINFO | Fatal: This application failed to start because no Qt platform plugin could be initialized.\nINFO | done\n"), 0o644)
	err := emulatorExited("pixel", log, nil)
	if Code(err) != "emulator_failed" || !strings.Contains(err.Error(), "no Qt platform plugin") {
		t.Errorf("err = %v", err)
	}
}
