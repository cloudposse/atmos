package permission

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scopedFakeTool is a Tool that also implements ScopedTool.
type scopedFakeTool struct {
	name string
	key  string
}

func (s scopedFakeTool) Name() string        { return s.name }
func (s scopedFakeTool) Description() string { return "scoped fake" }
func (s scopedFakeTool) IsRestricted() bool  { return false }
func (s scopedFakeTool) CacheKey() string    { return s.key }

// plainFakeTool is a Tool without a CacheKey.
type plainFakeTool struct{ name string }

func (p plainFakeTool) Name() string        { return p.name }
func (p plainFakeTool) Description() string { return "plain fake" }
func (p plainFakeTool) IsRestricted() bool  { return false }

func newTestCache(t *testing.T) *PermissionCache {
	t.Helper()
	cache, err := NewPermissionCache(filepath.Join(t.TempDir(), "base"))
	require.NoError(t, err)
	return cache
}

func TestScopedCache_Allow(t *testing.T) {
	const stored = "Bash(atmos list stacks)"

	tests := []struct {
		name  string
		tool  Tool
		found bool
	}{
		{name: "same command matches", tool: scopedFakeTool{name: "Bash", key: stored}, found: true},
		{name: "different command does not match", tool: scopedFakeTool{name: "Bash", key: "Bash(rm -rf /)"}, found: false},
		{name: "prefix of stored command does not match", tool: scopedFakeTool{name: "Bash", key: "Bash(atmos list)"}, found: false},
		{name: "scoped tool with bare key does not match parenthesized entry", tool: scopedFakeTool{name: "Bash", key: "Bash"}, found: false},
		{name: "non-scoped tool keeps the legacy Name(anything) rule (never used for Claude requests)", tool: plainFakeTool{name: "Bash"}, found: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newTestCache(t)
			require.NoError(t, cache.AddAllow(stored))
			p := NewCLIPrompterWithCache(cache)

			decision, found := p.checkCachedForTool(tt.tool)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.found, p.HasCachedDecision(tt.tool))
			if found {
				assert.True(t, decision)
			}
		})
	}
}

func TestScopedCache_Deny(t *testing.T) {
	cache := newTestCache(t)
	require.NoError(t, cache.AddDeny("Bash(atmos destroy)"))
	p := NewCLIPrompterWithCache(cache)

	decision, found := p.checkCachedForTool(scopedFakeTool{name: "Bash", key: "Bash(atmos destroy)"})
	assert.True(t, found)
	assert.False(t, decision)

	_, found = p.checkCachedForTool(scopedFakeTool{name: "Bash", key: "Bash(atmos list stacks)"})
	assert.False(t, found, "deny must not leak to a different command")

	_, found = p.checkCachedForTool(scopedFakeTool{name: "Bash", key: "Bash"})
	assert.False(t, found, "deny must not leak to the bare tool name")
}

func TestScopedCache_BareKeyMatchesOnlyBareEntry(t *testing.T) {
	cache := newTestCache(t)
	require.NoError(t, cache.AddAllow("Read"))
	p := NewCLIPrompterWithCache(cache)

	// A scoped tool that falls back to its bare name matches the bare entry only.
	_, found := p.checkCachedForTool(scopedFakeTool{name: "Read", key: "Read"})
	assert.True(t, found)

	// But a bare entry does not authorize a specific scoped key.
	_, found = p.checkCachedForTool(scopedFakeTool{name: "Read", key: "Read(/etc/passwd)"})
	assert.False(t, found)
}

func TestLegacyCache_BareNameBehaviorUnchanged(t *testing.T) {
	cache := newTestCache(t)
	require.NoError(t, cache.AddAllow("atmos_describe_component"))
	require.NoError(t, cache.AddDeny("atmos_list_stacks(stack:dev)"))
	p := NewCLIPrompterWithCache(cache)

	decision, found := p.checkCachedForTool(plainFakeTool{name: "atmos_describe_component"})
	assert.True(t, found)
	assert.True(t, decision)

	// Legacy Name(anything) entries still match the bare tool name.
	decision, found = p.checkCachedForTool(plainFakeTool{name: "atmos_list_stacks"})
	assert.True(t, found)
	assert.False(t, decision)

	_, found = p.checkCachedForTool(plainFakeTool{name: "other_tool"})
	assert.False(t, found)
}

func TestMatchesCachePattern_ScopedKeyNeverMatchesLoosely(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		pattern  string
		matches  bool
	}{
		{name: "scoped key equals entry", toolName: "Bash(ls)", pattern: "Bash(ls)", matches: true},
		{name: "scoped key vs different scoped entry", toolName: "Bash(ls)", pattern: "Bash(pwd)", matches: false},
		{name: "scoped key vs bare entry", toolName: "Bash(ls)", pattern: "Bash", matches: false},
		{name: "bare key vs scoped entry keeps legacy rule", toolName: "Bash", pattern: "Bash(ls)", matches: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, matchesCachePattern(tt.toolName, tt.pattern))
		})
	}
}

func TestExactLookups_IgnoreLegacyRule(t *testing.T) {
	cache := newTestCache(t)
	require.NoError(t, cache.AddAllow("Bash(atmos list stacks)"))
	require.NoError(t, cache.AddDeny("Bash(atmos destroy)"))

	assert.True(t, cache.IsAllowedExact("Bash(atmos list stacks)"))
	assert.False(t, cache.IsAllowedExact("Bash"))
	assert.False(t, cache.IsAllowedExact("Bash(other)"))
	assert.True(t, cache.IsDeniedExact("Bash(atmos destroy)"))
	assert.False(t, cache.IsDeniedExact("Bash"))
}

func TestCacheKeyFor(t *testing.T) {
	assert.Equal(t, "Bash(ls)", cacheKeyFor(scopedFakeTool{name: "Bash", key: "Bash(ls)"}))
	assert.Equal(t, "plain", cacheKeyFor(plainFakeTool{name: "plain"}))
}
