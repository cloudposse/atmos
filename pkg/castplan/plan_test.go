package castplan

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandAssertions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		action  Action
		text    string
		code    int
		failure string
	}{
		{name: "literal", action: Action{Expect: []Matcher{{Text: "ok"}}, Reject: []Matcher{{Text: "secret"}}}, text: "ok"},
		{name: "regex", action: Action{Expect: []Matcher{{Text: "ok-[0-9]+", Regex: true}}}, text: "ok-42"},
		{name: "missing", action: Action{Expect: []Matcher{{Text: "ok"}}}, failure: "missing"},
		{name: "rejected", action: Action{Reject: []Matcher{{Text: "secret"}}}, text: "secret", failure: "rejected"},
		{name: "exit", action: Action{ExitCode: 3}, failure: "exit status"},
		{name: "invalid regex", action: Action{Expect: []Matcher{{Text: "[", Regex: true}}}, failure: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.action.Check(tc.text, tc.code)
			if tc.failure == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.failure)
			}
		})
	}
}

func TestFingerprintPortableReplacements(t *testing.T) {
	p := &Plan{Configuration: map[string]any{"title": "demo"}, Replacements: map[string]string{"/first": "/repo"}}
	first, err := p.Fingerprint()
	require.NoError(t, err)
	p.Replacements = map[string]string{"/second": "/repo"}
	second, err := p.Fingerprint()
	require.NoError(t, err)
	require.Equal(t, first, second)
	p.Actions = []Action{{Kind: "run", Fields: map[string]any{"command": "next"}}}
	changed, err := p.Fingerprint()
	require.NoError(t, err)
	require.NotEqual(t, first, changed)
	p.Configuration["invalid"] = make(chan int)
	_, err = p.Fingerprint()
	require.Error(t, err)
	require.Contains(t, (Marker{Kind: "complete", Fingerprint: first}).Encode(), MarkerPrefix)
}
