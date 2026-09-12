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

func TestIsInsteadOfEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry GitConfigEntry
		want  bool
	}{
		{
			name:  "mirror insteadOf rule",
			entry: GitConfigEntry{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "https://github.com/cloudposse/"},
			want:  true,
		},
		{
			name:  "credential.helper is not an insteadOf rule",
			entry: GitConfigEntry{Key: "credential.helper", Value: ""},
			want:  false,
		},
		{
			name:  "extraheader is not an insteadOf rule",
			entry: GitConfigEntry{Key: "http.https://github.com/.extraheader", Value: "AUTHORIZATION: basic dGVzdA=="},
			want:  false,
		},
		{
			// Git config variable names are case-insensitive; the key's variable-name segment
			// (here "insteadof") must still match regardless of how it was cased when written.
			name:  "lowercase insteadof still matches (git config keys are case-insensitive)",
			entry: GitConfigEntry{Key: "url.file:///mirror/cloudposse/.insteadof", Value: "https://github.com/cloudposse/"},
			want:  true,
		},
		{
			name:  "mixed-case InsteadOf still matches (git config keys are case-insensitive)",
			entry: GitConfigEntry{Key: "url.file:///mirror/cloudposse/.InsteadOf", Value: "https://github.com/cloudposse/"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsInsteadOfEntry(tt.entry))
		})
	}
}

func TestIsExtraHeaderEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry GitConfigEntry
		want  bool
	}{
		{
			name:  "github extraheader",
			entry: GitConfigEntry{Key: "http.https://github.com/.extraheader", Value: "AUTHORIZATION: basic dGVzdA=="},
			want:  true,
		},
		{
			name:  "credential.helper is not an extraheader",
			entry: GitConfigEntry{Key: "credential.helper", Value: ""},
			want:  false,
		},
		{
			name:  "insteadOf is not an extraheader",
			entry: GitConfigEntry{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "https://github.com/cloudposse/"},
			want:  false,
		},
		{
			// Git config variable names are case-insensitive; the key's variable-name segment
			// (here "extraheader") must still match regardless of how it was cased when written.
			name:  "mixed-case extraHeader still matches (git config keys are case-insensitive)",
			entry: GitConfigEntry{Key: "http.https://github.com/.extraHeader", Value: "AUTHORIZATION: basic dGVzdA=="},
			want:  true,
		},
		{
			name:  "uppercase EXTRAHEADER still matches (git config keys are case-insensitive)",
			entry: GitConfigEntry{Key: "http.https://github.com/.EXTRAHEADER", Value: "AUTHORIZATION: basic dGVzdA=="},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsExtraHeaderEntry(tt.entry))
		})
	}
}

// TestWithout verifies Without removes only entries matching predicate, preserving order and
// the rest of the entries untouched -- e.g. stripping the mirror's insteadOf rules from a
// live-GitHub canary's git config while keeping credential.helper/extraheader entries.
func TestWithout(t *testing.T) {
	entries := []GitConfigEntry{
		{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "https://github.com/cloudposse/"},
		{Key: "credential.helper", Value: ""},
		{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "ssh://git@github.com/cloudposse/"},
		{Key: "http.https://github.com/.extraheader", Value: "AUTHORIZATION: basic dGVzdA=="},
	}

	got := Without(entries, IsInsteadOfEntry)

	require.Len(t, got, 2)
	require.Equal(t, GitConfigEntry{Key: "credential.helper", Value: ""}, got[0])
	require.Equal(t, GitConfigEntry{Key: "http.https://github.com/.extraheader", Value: "AUTHORIZATION: basic dGVzdA=="}, got[1])
}

// TestWithout_NoMatches verifies Without leaves entries unchanged (as a distinct slice) when
// nothing matches the predicate.
func TestWithout_NoMatches(t *testing.T) {
	entries := []GitConfigEntry{{Key: "credential.helper", Value: ""}}

	got := Without(entries, IsInsteadOfEntry)

	require.Equal(t, entries, got)
}

// TestReadEntries_RoundTrip verifies ReadEntries correctly parses back what AppendEntries wrote,
// including preserving order across a mix of pre-existing and newly appended entries.
func TestReadEntries_RoundTrip(t *testing.T) {
	target := map[string]string{}
	AppendEntries(target, []GitConfigEntry{
		{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "https://github.com/cloudposse/"},
	}, GitConfigEntry{Key: "credential.helper", Value: ""})

	env := make([]string, 0, len(target))
	for k, v := range target {
		env = append(env, k+"="+v)
	}

	got := ReadEntries(env)
	require.Len(t, got, 2)
	require.Equal(t, GitConfigEntry{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "https://github.com/cloudposse/"}, got[0])
	require.Equal(t, GitConfigEntry{Key: "credential.helper", Value: ""}, got[1])
}

// TestAppendEntries_SourceIsolation mirrors TestAppend_SourceIsolation for the lower-level
// AppendEntries entry point: mutating the existing/entries slices after the call must not affect
// the already-written target.
func TestAppendEntries_SourceIsolation(t *testing.T) {
	existing := []GitConfigEntry{{Key: "url.file:///mirror/cloudposse/.insteadOf", Value: "https://github.com/cloudposse/"}}
	entries := []GitConfigEntry{{Key: "credential.helper", Value: ""}}

	target := map[string]string{}
	AppendEntries(target, existing, entries...)

	existing[0].Value = "mutated-existing"
	entries[0].Value = "mutated-entries"

	require.Equal(t, "https://github.com/cloudposse/", target["GIT_CONFIG_VALUE_0"])
	require.Equal(t, "", target["GIT_CONFIG_VALUE_1"])
}
