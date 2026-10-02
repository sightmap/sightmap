package sightmap

import "testing"

func TestValidate_ComponentPrivacy(t *testing.T) {
	for _, tc := range []struct {
		privacy string
		want    bool
	}{
		{"", false}, {"block", false}, {"mask", false}, {"unmask", false},
		{"maks", true}, {"BLOCK", true},
	} {
		c := &Corpus{GlobalComponents: []ComponentDef{{Name: "C", Selectors: []string{"div"}, Privacy: tc.privacy}}}
		var got bool
		for _, e := range Validate(c) {
			if e.Code == "invalid-privacy" {
				got = true
			}
		}
		if got != tc.want {
			t.Errorf("privacy %q: invalid-privacy reported = %v, want %v", tc.privacy, got, tc.want)
		}
	}
}
