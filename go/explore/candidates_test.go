package explore

import (
	"strings"
	"testing"
)

func TestCandidatesFiltering(t *testing.T) {
	btn := mk("1", "button", "View details", "a", "", "", true)
	icon := mk("2", "image", "View details", "img", "", "", true)
	inner := mk("3", "button", "View details", "span", "", "", true) // same name, nested, unmatched
	under(btn, icon, inner)
	del := mk("4", "button", "Delete workspace", "button", "", "", true)
	hidden := mk("5", "button", "Hidden", "button", "", "", true)
	hidden.Visible = false
	static := mk("6", "heading", "Title", "h1", "", "", false)
	pay := withProps(mk("7", "button", "Go", "button", "", "PayButton", true), "label", "Pay now")

	got := Candidates([]*Node{btn, icon, inner, del, hidden, static, pay}, CandidateOptions{Avoid: []string{"delete", "Pay"}})
	if len(got) != 1 || got[0].Key != "n1" {
		t.Fatalf("candidates = %v", keys(got))
	}
	if got[0].SeenKey != `clicked button "View details"` {
		t.Fatalf("seenKey = %q", got[0].SeenKey)
	}
}

func TestCandidatesRepeatGuard(t *testing.T) {
	n := mk("1", "button", "Add", "button", "", "", true)
	seen := map[string]int{"https://s/|" + `clicked button "Add"`: 2}
	if got := Candidates([]*Node{n}, CandidateOptions{Seen: seen, URL: "https://s/"}); len(got) != 0 {
		t.Fatalf("expected the repeated action hidden, got %v", keys(got))
	}
	if got := Candidates([]*Node{n}, CandidateOptions{Seen: seen, URL: "https://other/"}); len(got) != 1 {
		t.Fatal("the guard is per URL")
	}
}

func TestDescribe(t *testing.T) {
	row := withProps(mk("10", "generic", "", "div", "", "InventoryItem", false), "name", "Backpack")
	add := withProps(mk("11", "button", "Add to cart", "button", "", "AddToCartButton", true), "label", "Add to cart")
	link := mk("12", "link", "Travel", "a", "href=catalogue/travel/index.html", "", true)
	pw := mk("13", "textbox", "Password", "input", "type=password", "", true)
	under(row, add, link)
	cases := map[*Node]string{
		add:  `[AddToCartButton label="Add to cart"] button "Add to cart" in [InventoryItem name="Backpack"]`,
		link: `link "Travel" href=catalogue/travel/index.html in [InventoryItem name="Backpack"]`,
		pw:   `textbox "Password" type=password`,
	}
	for n, want := range cases {
		if got := Describe(n); got != want {
			t.Errorf("Describe(%s) = %q, want %q", n.ID, got, want)
		}
	}
}

func TestBuildCriteriaSmallPage(t *testing.T) {
	a := mk("1", "button", "A", "button", "", "", true)
	b := mk("2", "link", "B", "a", "", "", true)
	b.InViewport = false
	crit := BuildCriteria(Candidates([]*Node{a, b}, CandidateOptions{}), CriteriaOptions{Goal: "x"})
	want := []string{"n1", "n2", "back", "scroll"}
	if strings.Join(crit.Keys(), ",") != strings.Join(want, ",") {
		t.Fatalf("keys = %v", crit.Keys())
	}
	if crit.Groups != nil {
		t.Fatal("small pages are not grouped")
	}
	// scroll is dropped when everything is already in the viewport
	b.InViewport = true
	crit = BuildCriteria(Candidates([]*Node{a, b}, CandidateOptions{}), CriteriaOptions{Goal: "x"})
	if crit.Has("scroll") {
		t.Fatal("scroll offered with nothing off-screen")
	}
}

func TestBuildCriteriaGroupsAndPromotes(t *testing.T) {
	nav := mk("nav", "navigation", "Main", "nav", "", "", false)
	var nodes []*Node
	for i := 0; i < 65; i++ {
		n := mk("n"+strings.Repeat("x", i%3)+string(rune('a'+i%26))+string(rune('0'+i/26)), "link", "Item", "a", "href=/item", "", true)
		under(nav, n)
		nodes = append(nodes, n)
	}
	sharp := mk("s", "link", "Sharp Objects", "a", "href=/catalogue/sharp-objects_997/index.html", "", true)
	card := withProps(mk("card", "generic", "", "article", "", "ProductCard", false), "title", "Sharp Objects")
	under(card, sharp)
	nodes = append(nodes, sharp)
	crit := BuildCriteria(Candidates(nodes, CandidateOptions{}), CriteriaOptions{Goal: `Find the book "Sharp Objects" and open its page.`, MaxCandidates: 60})
	if crit.Groups == nil {
		t.Fatal("expected grouping")
	}
	if !crit.Has("ns") {
		t.Fatalf("goal-relevant link not promoted: %v", crit.Keys())
	}
	if !crit.Has("g:nav") || len(crit.Groups["g:nav"]) != 65 {
		t.Fatalf("nav group missing or wrong size: %v", crit.Keys())
	}
	for _, o := range crit.Options {
		if o.Key == "g:nav" && !strings.Contains(o.Desc, "65 elements") {
			t.Fatalf("group desc = %q", o.Desc)
		}
	}
	sub := GroupCriteria(crit.Groups["g:nav"])
	if len(sub.Options) != 65 || sub.Has("back") {
		t.Fatalf("group criteria = %d options", len(sub.Options))
	}
}

func TestGoalTokens(t *testing.T) {
	got := GoalTokens("Go to page 2 of the full book list.")
	if strings.Join(got, ",") != "2" {
		t.Fatalf("tokens = %v", got)
	}
	got = GoalTokens(`Open the "Sauce Labs Fleece Jacket" product`)
	if strings.Join(got, ",") != "sauce,labs,fleece,jacket,product" {
		t.Fatalf("tokens = %v", got)
	}
}

func keys(cs []*Candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Key)
	}
	return out
}
