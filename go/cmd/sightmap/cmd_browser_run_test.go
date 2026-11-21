package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestRunVars(t *testing.T) {
	t.Setenv("N0", "from-env")
	defer func() { runVars = nil }()
	setRunVar("N0", "19")
	setRunVar("W", "720.4")
	cases := map[string][]string{
		`expect InboxRow --count $N0`:               {"expect", "InboxRow", "--count", "19"}, // a run variable shadows the environment
		`expect InboxRow --count $((N0 - 1))`:       {"expect", "InboxRow", "--count", "18"},
		`expect Board --prop "width>=$((W + 100))"`: {"expect", "Board", "--prop", "width>=820"},
		`keys $(( (2+3)*4 / 2 ))`:                   {"keys", "10"},
		`eval '$((N0))'`:                            {"eval", "$((N0))"}, // single quotes stay literal
	}
	for in, want := range cases {
		got, err := shellWords(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("shellWords(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "1x", "a-b"} {
		if validVarName(bad) {
			t.Errorf("validVarName(%q) = true", bad)
		}
	}
	if v, err := singleValue(&pageState{Nodes: []componentState{{}, {}}}, true, nil); err != nil || v != "2" {
		t.Errorf("singleValue count = %q, %v", v, err)
	}
	if _, err := singleValue(&pageState{Nodes: []componentState{{}, {}}}, false, []string{"x"}); err == nil {
		t.Error("singleValue of two matches: want an error")
	}
}

func TestShellWords(t *testing.T) {
	t.Setenv("ID", "WEB-143")
	cases := map[string][]string{
		`keys j s 3`: {"keys", "j", "s", "3"},
		`expect 'IssueRow[identifier=WEB-143]' --prop group='status:in progress'`: {"expect", "IssueRow[identifier=WEB-143]", "--prop", "group=status:in progress"},
		`expect "IssueRow[identifier=$ID]" --text "say \"hi\""`:                   {"expect", "IssueRow[identifier=WEB-143]", "--text", `say "hi"`},
		`click ${ID}x  # trailing comment`:                                        {"click", "WEB-143x"},
		`fill Composer 'it'\''s'`:                                                 {"fill", "Composer", "it's"},
		`keys Meta+k --type 'a b' Enter`:                                          {"keys", "Meta+k", "--type", "a b", "Enter"},
		`eval '$NOT_EXPANDED'`:                                                    {"eval", "$NOT_EXPANDED"},
	}
	for in, want := range cases {
		got, err := shellWords(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("shellWords(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`click 'open`, `click "open`} {
		if _, err := shellWords(bad); err == nil {
			t.Errorf("shellWords(%q) accepted an unclosed quote", bad)
		}
	}
}

func TestParseSteps(t *testing.T) {
	script := `# J8: peek and keyboard navigation
navigate /my-issues/assigned --settle

sightmap browser keys j    # pasted with its prefix
expect 'IssueRow[identifier=WEB-130]' \
  --prop focused=true
`
	steps, err := parseSteps(strings.NewReader(script))
	if err != nil {
		t.Fatal(err)
	}
	want := []step{
		{line: 2, text: "navigate /my-issues/assigned --settle", words: []string{"navigate", "/my-issues/assigned", "--settle"}},
		{line: 4, text: "sightmap browser keys j    # pasted with its prefix", words: []string{"keys", "j"}},
		{line: 5, text: "expect 'IssueRow[identifier=WEB-130]'    --prop focused=true", words: []string{"expect", "IssueRow[identifier=WEB-130]", "--prop", "focused=true"}},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("parseSteps =\n%+v\nwant\n%+v", steps, want)
	}
}

func TestWithoutGiven(t *testing.T) {
	common := []string{"--sightmap-dir", "d", "--tab", "T"}
	if got := withoutGiven(common, []string{"x", "--sightmap-dir=other"}); !reflect.DeepEqual(got, []string{"--tab", "T"}) {
		t.Errorf("withoutGiven = %q", got)
	}
	if got := withoutGiven(common, []string{"x"}); !reflect.DeepEqual(got, common) {
		t.Errorf("withoutGiven = %q", got)
	}
}

func TestClip(t *testing.T) {
	if got := clip("short", 80); got != "short" {
		t.Errorf("clip short = %q", got)
	}
	if got := clip(strings.Repeat("é", 100), 10); len([]rune(got)) != 10 || !strings.HasSuffix(got, "…") {
		t.Errorf("clip long = %q", got)
	}
}
