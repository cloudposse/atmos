package registry

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time guard: a rename of GOOS / GOARCH would silently corrupt downstream
// lockfile keys, so fail the build instead. Mirrors the CLAUDE.md sentinel pattern.
var _ = Platform{GOOS: "linux", GOARCH: "amd64"}

func TestPlatformString(t *testing.T) {
	assert.Equal(t, "linux_amd64", Platform{GOOS: "linux", GOARCH: "amd64"}.String())
	assert.Equal(t, "darwin_arm64", Platform{GOOS: "darwin", GOARCH: "arm64"}.String())
}

func TestSupportedPlatforms(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
		want []Platform
	}{
		{
			name: "no envs, no overrides → common platform set",
			tool: Tool{},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "amd64"},
				{GOOS: "darwin", GOARCH: "arm64"},
				{GOOS: "linux", GOARCH: "amd64"},
				{GOOS: "linux", GOARCH: "arm64"},
				{GOOS: "windows", GOARCH: "amd64"},
				{GOOS: "windows", GOARCH: "arm64"},
			},
		},
		{
			name: "explicit goos/goarch entries",
			tool: Tool{
				SupportedEnvs: []string{"linux/amd64", "darwin/arm64"},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "arm64"},
				{GOOS: "linux", GOARCH: "amd64"},
			},
		},
		{
			name: "os-only entry fans out to every common arch for that OS",
			tool: Tool{
				SupportedEnvs: []string{"linux"},
			},
			want: []Platform{
				{GOOS: "linux", GOARCH: "amd64"},
				{GOOS: "linux", GOARCH: "arm64"},
			},
		},
		{
			name: "arch-only entry fans out to every common OS for that arch",
			tool: Tool{
				SupportedEnvs: []string{"amd64"},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "amd64"},
				{GOOS: "linux", GOARCH: "amd64"},
				{GOOS: "windows", GOARCH: "amd64"},
			},
		},
		{
			name: `"all" expands to the common set`,
			tool: Tool{
				SupportedEnvs: []string{"all"},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "amd64"},
				{GOOS: "darwin", GOARCH: "arm64"},
				{GOOS: "linux", GOARCH: "amd64"},
				{GOOS: "linux", GOARCH: "arm64"},
				{GOOS: "windows", GOARCH: "amd64"},
				{GOOS: "windows", GOARCH: "arm64"},
			},
		},
		{
			name: "overrides contribute platforms even without SupportedEnvs",
			tool: Tool{
				Overrides: []Override{
					{GOOS: "linux", GOARCH: "amd64"},
					{GOOS: "darwin", GOARCH: "arm64"},
				},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "arm64"},
				{GOOS: "linux", GOARCH: "amd64"},
			},
		},
		{
			name: "override.Envs follows the same fan-out rules as supported_envs",
			tool: Tool{
				Overrides: []Override{
					{Envs: []string{"darwin"}}, // No goos/goarch on override itself.
				},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "amd64"},
				{GOOS: "darwin", GOARCH: "arm64"},
			},
		},
		{
			name: "duplicate entries are deduped",
			tool: Tool{
				SupportedEnvs: []string{"linux/amd64", "linux/amd64", "linux"},
				Overrides: []Override{
					{GOOS: "linux", GOARCH: "amd64"},
				},
			},
			want: []Platform{
				{GOOS: "linux", GOARCH: "amd64"},
				{GOOS: "linux", GOARCH: "arm64"},
			},
		},
		{
			name: "mixed: supported_envs + overrides combine",
			tool: Tool{
				SupportedEnvs: []string{"darwin/amd64"},
				Overrides: []Override{
					{GOOS: "linux", GOARCH: "arm64"},
				},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "amd64"},
				{GOOS: "linux", GOARCH: "arm64"},
			},
		},
		{
			name: "case-insensitive entries normalize to lowercase",
			tool: Tool{
				SupportedEnvs: []string{"Linux/AMD64", " DARWIN/Arm64 "},
			},
			want: []Platform{
				{GOOS: "darwin", GOARCH: "arm64"},
				{GOOS: "linux", GOARCH: "amd64"},
			},
		},
		{
			name: "unknown OS-only / arch-only entries are dropped silently",
			tool: Tool{
				// "haiku" is neither a known GOOS nor a known GOARCH → drop.
				// "linux/amd64" is the explicit form and is always preserved.
				SupportedEnvs: []string{"haiku", "linux/amd64"},
			},
			want: []Platform{
				{GOOS: "linux", GOARCH: "amd64"},
			},
		},
		{
			name: "explicit goos/goarch form is preserved even for unknown values (registry-tolerant)",
			// The registry is the source of truth. If aqua advertises `<x>/<y>`, atmos
			// records it — even if the OS/arch aren't ones Go itself knows about.
			tool: Tool{
				SupportedEnvs: []string{"plan9/amd64"},
			},
			want: []Platform{
				{GOOS: "plan9", GOARCH: "amd64"},
			},
		},
		{
			name: "empty entry in supported_envs is dropped",
			tool: Tool{
				SupportedEnvs: []string{"", "linux/amd64"},
			},
			want: []Platform{
				{GOOS: "linux", GOARCH: "amd64"},
			},
		},
		{
			name: "freebsd OS-only is recognized",
			tool: Tool{
				SupportedEnvs: []string{"freebsd/amd64"},
			},
			want: []Platform{
				{GOOS: "freebsd", GOARCH: "amd64"},
			},
		},
		{
			name: "every entry unknown AND no explicit form → fall back to common set",
			// "haiku" is OS-only-unknown and gets dropped, leaving an empty set →
			// SupportedPlatforms returns the common set so callers always have something.
			tool: Tool{
				SupportedEnvs: []string{"haiku"},
			},
			want: append([]Platform(nil), commonPlatforms...),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tool.SupportedPlatforms()

			// Per CLAUDE.md slice rule: assert element contents, not just length.
			// Sort both sides for stable comparison (the function already sorts but we
			// don't want this test to depend on that contract — assert on contents only).
			sortPlatforms(got)
			want := append([]Platform(nil), tc.want...)
			sortPlatforms(want)
			assert.Equal(t, want, got)
		})
	}
}

// TestSupportedPlatforms_IsSorted is the *contract* test that callers can depend on
// stable ordering for golden-snapshot-friendly lockfile output.
func TestSupportedPlatforms_IsSorted(t *testing.T) {
	tool := Tool{
		SupportedEnvs: []string{"windows/amd64", "darwin/arm64", "linux/amd64"},
	}
	got := tool.SupportedPlatforms()
	require := assert.New(t)
	require.Len(got, 3)
	require.Equal(Platform{GOOS: "darwin", GOARCH: "arm64"}, got[0])
	require.Equal(Platform{GOOS: "linux", GOARCH: "amd64"}, got[1])
	require.Equal(Platform{GOOS: "windows", GOARCH: "amd64"}, got[2])
}

// TestSupportedPlatforms_ReturnedSliceIsIndependent guards against an accidental shared
// backing array — callers (e.g. `atmos toolchain lock`) iterate the result and may need
// to mutate it for filtering. The result must not alias internal state.
func TestSupportedPlatforms_ReturnedSliceIsIndependent(t *testing.T) {
	tool := Tool{} // Hits the commonPlatforms branch.
	a := tool.SupportedPlatforms()
	b := tool.SupportedPlatforms()
	a[0] = Platform{GOOS: "altered", GOARCH: "altered"}

	// b must be unaffected.
	assert.Equal(t, Platform{GOOS: "darwin", GOARCH: "amd64"}, b[0])
	// And the package-level commonPlatforms must be unaffected too.
	assert.Equal(t, Platform{GOOS: "darwin", GOARCH: "amd64"}, commonPlatforms[0])
}

func sortPlatforms(ps []Platform) {
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].GOOS != ps[j].GOOS {
			return ps[i].GOOS < ps[j].GOOS
		}
		return ps[i].GOARCH < ps[j].GOARCH
	})
}
