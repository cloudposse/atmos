package toolchain

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildToolListWithSkipped_Advisory closes #2256 — `.tool-versions` is a shared
// ecosystem file (asdf / mise / setup-node), so atmos must coexist by collecting
// names it can't resolve rather than failing the whole batch.
func TestBuildToolListWithSkipped_Advisory(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform":           {"hashicorp", "terraform"},
			"hashicorp/terraform": {"hashicorp", "terraform"},
			"opentofu":            {"opentofu", "opentofu"},
			// "nodejs" and "python" are intentionally absent — they're shared-ecosystem entries.
		},
	}
	installer := NewInstallerWithResolver(mockResolver, filepath.Join(tempDir, "bin"))

	tests := []struct {
		name          string
		input         map[string][]string
		wantToolCount int
		wantSkipped   []string
	}{
		{
			name: "issue 2256: nodejs + python skipped, terraform installed",
			input: map[string][]string{
				"nodejs":    {"25.8.1"},
				"python":    {"3.12.0"},
				"terraform": {"1.11.4"},
			},
			wantToolCount: 1,
			wantSkipped:   []string{"nodejs", "python"},
		},
		{
			name: "all entries resolvable -> empty skipped",
			input: map[string][]string{
				"terraform": {"1.11.4"},
				"opentofu":  {"1.10.0"},
			},
			wantToolCount: 2,
			wantSkipped:   nil,
		},
		{
			name: "every entry unresolvable -> empty list, all skipped",
			input: map[string][]string{
				"nodejs": {"25.8.1"},
				"java":   {"21.0.0"},
			},
			wantToolCount: 0,
			wantSkipped:   []string{"java", "nodejs"},
		},
		{
			name:          "empty input -> empty list, empty skipped",
			input:         map[string][]string{},
			wantToolCount: 0,
			wantSkipped:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tv := &ToolVersions{Tools: tc.input}
			toolList, skipped := buildToolListWithSkipped(installer, tv)

			assert.Len(t, toolList, tc.wantToolCount)
			// Map iteration is non-deterministic; sort before asserting the contents.
			sort.Strings(skipped)
			assert.Equal(t, tc.wantSkipped, skipped)
		})
	}
}

// TestBuildToolList_BackCompat verifies the original buildToolList signature still
// works as a wrapper around buildToolListWithSkipped — no caller needs to change.
func TestBuildToolList_BackCompat(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver, filepath.Join(tempDir, "bin"))

	tv := &ToolVersions{Tools: map[string][]string{
		"terraform": {"1.11.4"},
		"nodejs":    {"25.8.1"}, // Will be dropped; ensures the back-compat wrapper still filters.
	}}
	got := buildToolList(installer, tv)
	require.Len(t, got, 1)
	assert.Equal(t, "terraform", got[0].repo)
}

// TestWarnSkippedAdvisoryTools_Stable verifies the warning is stable across runs
// — important because Go map iteration is non-deterministic and a flaky warning
// would surface as test churn / snapshot churn for any downstream check.
func TestWarnSkippedAdvisoryTools_Stable(t *testing.T) {
	// We can't easily intercept ui.Warningf without ui-init plumbing, so this test
	// just exercises the no-skipped path (zero-allocation, no warning emitted) and
	// the sorted path (no panic, deterministic order). Snapshot/output assertions
	// live in the integration-level test once that exists.

	// No-op path: empty list must not warn or panic.
	warnSkippedAdvisoryTools(nil)
	warnSkippedAdvisoryTools([]string{})

	// Sorted path: must not panic with a long list of names in arbitrary order.
	warnSkippedAdvisoryTools([]string{"zsh", "python", "java", "nodejs", "go", "rust"})
}
