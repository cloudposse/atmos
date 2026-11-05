package permission

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakePrompter records calls and returns a canned answer.
type fakePrompter struct {
	mu     sync.Mutex
	allow  bool
	err    error
	calls  int
	events *[]string
}

func (f *fakePrompter) Prompt(_ context.Context, _ Tool, _ map[string]interface{}) (bool, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.events != nil {
		*f.events = append(*f.events, "prompt")
	}
	return f.allow, f.err
}

func testConfig(t *testing.T, tools *schema.AIToolSettings) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{
		BasePath: filepath.Join(t.TempDir(), "base"),
		AI:       schema.AISettings{Tools: *tools},
	}
}

//nolint:staticcheck // Deprecated aliases are still honored for backwards compatibility.
func TestNewFromConfig_Modes(t *testing.T) {
	tests := []struct {
		name        string
		tools       schema.AIToolSettings
		toolName    string
		wantAllowed bool
		wantErr     error
		wantPrompts int
	}{
		{name: "require_confirmation prompts and allows", tools: schema.AIToolSettings{Mode: "require_confirmation"}, toolName: "Bash", wantAllowed: true, wantPrompts: 1},
		{name: "default prompts", tools: schema.AIToolSettings{}, toolName: "Bash", wantAllowed: true, wantPrompts: 1},
		{name: "allow never prompts", tools: schema.AIToolSettings{Mode: "allow"}, toolName: "Bash", wantAllowed: true},
		{name: "allow honors blocked", tools: schema.AIToolSettings{Mode: "allow", Blocked: []string{"Bash"}}, toolName: "Bash", wantErr: errUtils.ErrAIToolBlocked},
		{name: "yolo bypasses blocked", tools: schema.AIToolSettings{Mode: "yolo", Blocked: []string{"Bash"}}, toolName: "Bash", wantAllowed: true},
		{name: "allowed list skips prompt", tools: schema.AIToolSettings{Allowed: []string{"Bash"}}, toolName: "Bash", wantAllowed: true},
		{name: "unlisted tool prompts", tools: schema.AIToolSettings{Allowed: []string{"Read"}}, toolName: "Bash", wantAllowed: true, wantPrompts: 1},
		{name: "deprecated yolo_mode", tools: schema.AIToolSettings{YOLOMode: true, Blocked: []string{"Bash"}}, toolName: "Bash", wantAllowed: true},
		{name: "deprecated require_confirmation false", tools: schema.AIToolSettings{RequireConfirmation: boolRef(false)}, toolName: "Bash", wantAllowed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompter := &fakePrompter{allow: true}
			checker, err := NewFromConfig(testConfig(t, &tt.tools), WithPrompter(prompter))
			require.NoError(t, err)

			allowed, err := checker.CheckPermission(context.Background(), plainFakeTool{name: tt.toolName}, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantAllowed, allowed)
			assert.Equal(t, tt.wantPrompts, prompter.calls)
		})
	}
}

func TestNewFromConfig_InvalidMode(t *testing.T) {
	checker, err := NewFromConfig(testConfig(t, &schema.AIToolSettings{Mode: "bogus"}))
	require.Error(t, err)
	assert.Nil(t, checker)
	assert.ErrorIs(t, err, errUtils.ErrAIToolsInvalidMode)
}

func TestNewFromConfig_DefaultPrompterIsCacheBacked(t *testing.T) {
	cfg := testConfig(t, &schema.AIToolSettings{})

	// Pre-seed the cache the default prompter will load.
	cache, err := NewPermissionCache(cfg.BasePath)
	require.NoError(t, err)
	require.NoError(t, cache.AddAllow("Bash(atmos list stacks)"))

	checker, err := NewFromConfig(cfg)
	require.NoError(t, err)

	allowed, err := checker.CheckPermission(context.Background(), scopedFakeTool{name: "Bash", key: "Bash(atmos list stacks)"}, nil)
	require.NoError(t, err)
	assert.True(t, allowed, "cached decision should answer without a prompt")
}

func TestNewFromConfig_NilConfig(t *testing.T) {
	checker, err := NewFromConfig(nil, WithPrompter(&fakePrompter{allow: true}))
	require.NoError(t, err)
	require.NotNil(t, checker)
}

func TestNewFromConfig_PromptHooks(t *testing.T) {
	tests := []struct {
		name       string
		tools      schema.AIToolSettings
		prompter   *fakePrompter
		wantEvents []string
		wantErr    error
	}{
		{
			name:       "hooks bracket a real prompt",
			tools:      schema.AIToolSettings{},
			prompter:   &fakePrompter{allow: true},
			wantEvents: []string{"before", "prompt", "after"},
		},
		{
			name:       "after fires when the prompt errors",
			tools:      schema.AIToolSettings{},
			prompter:   &fakePrompter{err: errUtils.ErrUserAborted},
			wantEvents: []string{"before", "prompt", "after"},
			wantErr:    errUtils.ErrUserAborted,
		},
		{name: "no hooks in allow mode", tools: schema.AIToolSettings{Mode: "allow"}, prompter: &fakePrompter{}, wantEvents: nil},
		{name: "no hooks in yolo mode", tools: schema.AIToolSettings{Mode: "yolo"}, prompter: &fakePrompter{}, wantEvents: nil},
		{name: "no hooks for allowed-list tool", tools: schema.AIToolSettings{Allowed: []string{"Bash"}}, prompter: &fakePrompter{}, wantEvents: nil},
		{name: "no hooks for blocked tool", tools: schema.AIToolSettings{Blocked: []string{"Bash"}}, prompter: &fakePrompter{}, wantEvents: nil, wantErr: errUtils.ErrAIToolBlocked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			tt.prompter.events = &events

			checker, err := NewFromConfig(
				testConfig(t, &tt.tools),
				WithPrompter(tt.prompter),
				WithPromptHooks(func() { events = append(events, "before") }, func() { events = append(events, "after") }),
			)
			require.NoError(t, err)

			_, err = checker.CheckPermission(context.Background(), plainFakeTool{name: "Bash"}, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantEvents, events)
		})
	}
}

func TestNewFromConfig_PromptHooks_NilSafe(t *testing.T) {
	var afterOnly []string
	checker, err := NewFromConfig(
		testConfig(t, &schema.AIToolSettings{}),
		WithPrompter(&fakePrompter{allow: true}),
		WithPromptHooks(nil, func() { afterOnly = append(afterOnly, "after") }),
	)
	require.NoError(t, err)

	_, err = checker.CheckPermission(context.Background(), plainFakeTool{name: "Bash"}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"after"}, afterOnly)
}

// cachedFakePrompter reports a cached decision so hooks must be skipped.
type cachedFakePrompter struct {
	fakePrompter
	cached bool
}

func (c *cachedFakePrompter) HasCachedDecision(Tool) bool { return c.cached }

func TestHookedPrompter_SkipsHooksOnCachedDecision(t *testing.T) {
	tests := []struct {
		name       string
		cached     bool
		wantEvents []string
	}{
		{name: "cache hit skips hooks", cached: true, wantEvents: nil},
		{name: "cache miss fires hooks", cached: false, wantEvents: []string{"before", "after"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			inner := &cachedFakePrompter{cached: tt.cached}
			inner.allow = true
			h := &hookedPrompter{
				inner:  inner,
				before: func() { events = append(events, "before") },
				after:  func() { events = append(events, "after") },
			}

			allowed, err := h.Prompt(context.Background(), plainFakeTool{name: "Bash"}, nil)
			require.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, tt.wantEvents, events)
			assert.Equal(t, 1, inner.calls)
		})
	}
}

func TestHookedPrompter_ErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	h := &hookedPrompter{inner: &fakePrompter{err: boom}}

	_, err := h.Prompt(context.Background(), plainFakeTool{name: "Bash"}, nil)
	assert.ErrorIs(t, err, boom)
}
