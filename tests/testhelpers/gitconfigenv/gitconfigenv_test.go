package gitconfigenv

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppend(t *testing.T) {
	tests := []struct {
		name          string
		base          []string
		entries       []GitConfigEntry
		expectedCount string
		expectedKeys  []string
		expectedVals  []string
	}{
		{
			name: "no existing entries, new entries appended from index 0",
			base: []string{"PATH=/usr/bin", "HOME=/home/test"},
			entries: []GitConfigEntry{
				{Key: "credential.helper", Value: ""},
			},
			expectedCount: "1",
			expectedKeys:  []string{"credential.helper"},
			expectedVals:  []string{""},
		},
		{
			name: "existing entries preserved, new entries appended after",
			base: []string{
				"GIT_CONFIG_COUNT=2",
				"GIT_CONFIG_KEY_0=url.file:///mirror/cloudposse/.insteadOf",
				"GIT_CONFIG_VALUE_0=https://github.com/cloudposse/",
				"GIT_CONFIG_KEY_1=url.file:///mirror/cloudposse/.insteadOf",
				"GIT_CONFIG_VALUE_1=ssh://git@github.com/cloudposse/",
			},
			entries: []GitConfigEntry{
				{Key: "credential.helper", Value: ""},
				{Key: "http.https://github.com/.extraheader", Value: "AUTHORIZATION: basic dGVzdA=="},
			},
			expectedCount: "4",
			expectedKeys: []string{
				"url.file:///mirror/cloudposse/.insteadOf",
				"url.file:///mirror/cloudposse/.insteadOf",
				"credential.helper",
				"http.https://github.com/.extraheader",
			},
			expectedVals: []string{
				"https://github.com/cloudposse/",
				"ssh://git@github.com/cloudposse/",
				"",
				"AUTHORIZATION: basic dGVzdA==",
			},
		},
		{
			name:          "no entries at all leaves target with count zero",
			base:          nil,
			entries:       nil,
			expectedCount: "0",
			expectedKeys:  nil,
			expectedVals:  nil,
		},
		{
			name: "malformed existing count is treated as absent",
			base: []string{"GIT_CONFIG_COUNT=not-a-number"},
			entries: []GitConfigEntry{
				{Key: "credential.helper", Value: ""},
			},
			expectedCount: "1",
			expectedKeys:  []string{"credential.helper"},
			expectedVals:  []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := map[string]string{}
			Append(target, tt.base, tt.entries...)

			require.Equal(t, tt.expectedCount, target["GIT_CONFIG_COUNT"])
			for i, key := range tt.expectedKeys {
				idx := strconv.Itoa(i)
				require.Equal(t, key, target["GIT_CONFIG_KEY_"+idx])
				require.Equal(t, tt.expectedVals[i], target["GIT_CONFIG_VALUE_"+idx])
			}
		})
	}
}

// TestAppend_Idempotent verifies that calling Append twice with the same base
// and entries against fresh targets produces byte-identical results -- callers
// rebuild the merged set on every invocation rather than mutating shared state.
func TestAppend_Idempotent(t *testing.T) {
	base := []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url.file:///mirror/cloudposse/.insteadOf",
		"GIT_CONFIG_VALUE_0=https://github.com/cloudposse/",
	}
	entries := []GitConfigEntry{{Key: "credential.helper", Value: ""}}

	first := map[string]string{}
	Append(first, base, entries...)

	second := map[string]string{}
	Append(second, base, entries...)

	require.Equal(t, first, second)
}

// TestAppend_SourceIsolation verifies Append does not mutate its base slice or
// leak references between calls: mutating the entries slice after calling
// Append must not affect the already-written target.
func TestAppend_SourceIsolation(t *testing.T) {
	entries := []GitConfigEntry{{Key: "credential.helper", Value: ""}}
	target := map[string]string{}
	Append(target, nil, entries...)

	entries[0].Value = "mutated"

	require.Equal(t, "", target["GIT_CONFIG_VALUE_0"], "target must not observe post-call mutation of caller's entries slice")
}
