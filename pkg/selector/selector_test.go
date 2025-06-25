package selector

import "testing"

func TestParseAndMatch(t *testing.T) {
	labels := map[string]string{
		"env":     "prod",
		"tier":    "backend",
		"version": "v1",
	}

	cases := []struct {
		selector string
		wantOK   bool
	}{
		{"env=prod", true},
		{"env=dev", false},
		{"env!=dev", true},
		{"env!=prod", false},
		{"tier in (backend,frontend)", true},
		{"tier in (data,cache)", false},
		{"tier notin (cache)", true},
		{"tier notin (backend)", false},
		{"env", true},
		{"!missing", true},
		{"!env", false},
		{"env=prod,tier=backend", true},
		{"env=prod,tier=frontend", false},
	}

	for _, tc := range cases {
		reqs, err := Parse(tc.selector)
		if err != nil {
			t.Fatalf("unexpected parse error for '%s': %v", tc.selector, err)
		}
		got := Matches(labels, reqs)
		if got != tc.wantOK {
			t.Errorf("selector '%s' -> %v, want %v", tc.selector, got, tc.wantOK)
		}
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"",
		"env in ()",
		"foo <> bar",
		"missing )",
	}

	for _, sel := range bad {
		if _, err := Parse(sel); err == nil {
			t.Errorf("expected error for selector '%s'", sel)
		}
	}
}
