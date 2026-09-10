package parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLabels(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input, key, fallback        string
		hasKey, hasDefault, invalid bool
	}{
		{input: " \t "},
		{input: "runner", key: "runner", hasKey: true},
		{input: "cost-center platform", key: "cost-center", fallback: "platform", hasKey: true, hasDefault: true},
		{input: `example.com/runner "large linux"`, key: "example.com/runner", fallback: "large linux", hasKey: true, hasDefault: true},
		{input: `owner ''`, key: "owner", hasKey: true, hasDefault: true},
		{input: `'a.b' 'team''s runner'`, key: "a.b", fallback: "team's runner", hasKey: true, hasDefault: true},
		{input: `"" default`, invalid: true},
		{input: `runner "unclosed`, invalid: true},
		{input: "runner fallback extra", invalid: true},
		{input: "runner |", invalid: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLabels(tc.input)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.hasKey {
				require.NotNil(t, got.Key)
				require.Equal(t, tc.key, *got.Key)
			} else {
				require.Nil(t, got.Key)
			}
			if tc.hasDefault {
				require.NotNil(t, got.Default)
				require.Equal(t, tc.fallback, *got.Default)
			} else {
				require.Nil(t, got.Default)
			}
		})
	}
}
