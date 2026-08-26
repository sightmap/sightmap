package main

import (
	"slices"
	"testing"
)

func TestStripStartFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		strip []string
		want  []string
	}{
		{"bool flag alone", []string{"--detach"}, []string{"detach", "log-file"}, nil},
		{"bool =true form", []string{"--detach=true"}, []string{"detach", "log-file"}, nil},
		{"single dash", []string{"-detach"}, []string{"detach", "log-file"}, nil},
		{
			"keeps other flags",
			[]string{"--headless", "--detach", "--port", "9"},
			[]string{"detach", "log-file"},
			[]string{"--headless", "--port", "9"},
		},
		{
			"value flag with separate value token",
			[]string{"--log-file", "/tmp/x", "--url", "http://y"},
			[]string{"detach", "log-file"},
			[]string{"--url", "http://y"},
		},
		{
			"value flag =form",
			[]string{"--log-file=/tmp/x", "--headless"},
			[]string{"detach", "log-file"},
			[]string{"--headless"},
		},
		{
			"both stripped",
			[]string{"--detach", "--log-file", "/tmp/x", "--headless"},
			[]string{"detach", "log-file"},
			[]string{"--headless"},
		},
		{
			"does not eat the token after a bool flag",
			[]string{"--detach", "--port", "7890"},
			[]string{"detach", "log-file"},
			[]string{"--port", "7890"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stripStartFlags(tc.args, tc.strip...)
			if !slices.Equal(got, tc.want) {
				t.Errorf("stripStartFlags(%v, %v) = %v, want %v", tc.args, tc.strip, got, tc.want)
			}
		})
	}
}
