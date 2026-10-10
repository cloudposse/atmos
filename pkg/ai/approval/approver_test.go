package approval

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the schema fields used below.
var _ = schema.AIToolSettings{Mode: "allow", Allowed: nil, Blocked: nil}

// stubPrompter implements permission.Prompter with a canned result and records what it was asked.
type stubPrompter struct {
	mu     sync.Mutex
	allow  bool
	err    error
	tools  []permission.Tool
	params []map[string]interface{}
}

func (s *stubPrompter) Prompt(_ context.Context, tool permission.Tool, params map[string]interface{}) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, tool)
	s.params = append(s.params, params)
	return s.allow, s.err
}

func (s *stubPrompter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tools)
}

func newApprover(t *testing.T, tools *schema.AIToolSettings, prompter permission.Prompter, opts ...permission.Option) *PermissionApprover {
	t.Helper()
	cfg := &schema.AtmosConfiguration{
		BasePath: filepath.Join(t.TempDir(), "base"),
		AI:       schema.AISettings{Tools: *tools},
	}
	opts = append([]permission.Option{permission.WithPrompter(prompter)}, opts...)
	checker, err := permission.NewFromConfig(cfg, opts...)
	require.NoError(t, err)
	return NewPermissionApprover(checker)
}

var bashRequest = Request{
	ToolName:  "Bash",
	ToolUseID: "toolu_1",
	Input:     map[string]any{"command": "atmos list stacks", "description": "List stacks"},
}

func TestPermissionApprover_DecisionMapping(t *testing.T) {
	tests := []struct {
		name         string
		tools        schema.AIToolSettings
		prompter     *stubPrompter
		wantDecision Decision
		wantPrompts  int
		msgContains  []string
	}{
		{name: "yolo allows", tools: schema.AIToolSettings{Mode: "yolo"}, prompter: &stubPrompter{}, wantDecision: Decision{Allow: true}},
		{name: "yolo allows even when blocked", tools: schema.AIToolSettings{Mode: "yolo", Blocked: []string{"Bash"}}, prompter: &stubPrompter{}, wantDecision: Decision{Allow: true}},
		{name: "allow mode allows", tools: schema.AIToolSettings{Mode: "allow"}, prompter: &stubPrompter{}, wantDecision: Decision{Allow: true}},
		{name: "allowed list allows without prompt", tools: schema.AIToolSettings{Allowed: []string{"Bash"}}, prompter: &stubPrompter{}, wantDecision: Decision{Allow: true}},
		{name: "prompt allow", tools: schema.AIToolSettings{}, prompter: &stubPrompter{allow: true}, wantDecision: Decision{Allow: true}, wantPrompts: 1},
		{name: "prompt deny", tools: schema.AIToolSettings{}, prompter: &stubPrompter{allow: false}, wantDecision: Decision{Message: "Denied by user."}, wantPrompts: 1},
		{
			name:         "blocked denies with explanation",
			tools:        schema.AIToolSettings{Mode: "allow", Blocked: []string{"Bash"}},
			prompter:     &stubPrompter{},
			wantDecision: Decision{},
			msgContains:  []string{"Bash", "ai.tools.blocked"},
		},
		{
			name:         "explicit execution denied error",
			tools:        schema.AIToolSettings{},
			prompter:     &stubPrompter{err: errUtils.ErrAIToolExecutionDenied},
			wantDecision: Decision{Message: "Denied by user."},
			wantPrompts:  1,
		},
		{
			name:         "user abort interrupts",
			tools:        schema.AIToolSettings{},
			prompter:     &stubPrompter{err: errUtils.ErrUserAborted},
			wantDecision: Decision{Interrupt: true, Message: "Aborted by user."},
			wantPrompts:  1,
		},
		{
			name:         "wrapped user abort interrupts",
			tools:        schema.AIToolSettings{},
			prompter:     &stubPrompter{err: fmt.Errorf("wrapped: %w", errUtils.ErrUserAborted)},
			wantDecision: Decision{Interrupt: true, Message: "Aborted by user."},
			wantPrompts:  1,
		},
		{
			name:         "no terminal is non-interactive",
			tools:        schema.AIToolSettings{},
			prompter:     &stubPrompter{err: errUtils.ErrInteractiveNotAvailable},
			wantDecision: Decision{NonInteractive: true},
			wantPrompts:  1,
			msgContains:  []string{"ai.tools.mode", "yolo", "allow", "ai.providers.claude-code.allowed_tools"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approver := newApprover(t, &tt.tools, tt.prompter)

			got, err := approver.Approve(context.Background(), bashRequest)
			require.NoError(t, err)

			assert.Equal(t, tt.wantDecision.Allow, got.Allow)
			assert.Equal(t, tt.wantDecision.Interrupt, got.Interrupt)
			assert.Equal(t, tt.wantDecision.NonInteractive, got.NonInteractive)
			if tt.wantDecision.Message != "" {
				assert.Equal(t, tt.wantDecision.Message, got.Message)
			}
			for _, want := range tt.msgContains {
				assert.Contains(t, got.Message, want)
			}
			assert.Equal(t, tt.wantPrompts, tt.prompter.callCount())
		})
	}
}

func TestPermissionApprover_UnexpectedErrorIsReturned(t *testing.T) {
	boom := errors.New("prompt exploded")
	approver := newApprover(t, &schema.AIToolSettings{}, &stubPrompter{err: boom})

	got, err := approver.Approve(context.Background(), bashRequest)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.False(t, got.Allow)
}

func TestPermissionApprover_NoPrompterIsAnError(t *testing.T) {
	checker := permission.NewChecker(&permission.Config{Mode: permission.ModePrompt}, nil)
	approver := NewPermissionApprover(checker)

	_, err := approver.Approve(context.Background(), bashRequest)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAINoPrompter)
}

func TestPermissionApprover_PromptReceivesToolAndParams(t *testing.T) {
	prompter := &stubPrompter{allow: true}
	approver := newApprover(t, &schema.AIToolSettings{}, prompter)

	req := Request{
		ToolName: "Bash",
		Input: map[string]any{
			"command":     "atmos list stacks",
			"description": "List stacks",
			"timeout":     30,
			"env":         map[string]any{"A": "b"},
		},
	}
	_, err := approver.Approve(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, 1, prompter.callCount())

	tool := prompter.tools[0]
	assert.Equal(t, "Bash", tool.Name())
	assert.Equal(t, "List stacks", tool.Description())
	assert.False(t, tool.IsRestricted())
	scoped, ok := tool.(permission.ScopedTool)
	require.True(t, ok)
	assert.Equal(t, "Bash(atmos list stacks)", scoped.CacheKey())

	assert.Equal(t, map[string]interface{}{
		"command":     "atmos list stacks",
		"description": "List stacks",
		"timeout":     "30",
		"env":         `{"A":"b"}`,
	}, prompter.params[0])
}

func TestRequestTool_CacheKeyAndDescription(t *testing.T) {
	tests := []struct {
		name    string
		req     Request
		key     string
		descr   string
		bareKey bool
	}{
		{name: "bash command", req: Request{ToolName: "Bash", Input: map[string]any{"command": "ls -la"}}, key: "Bash(ls -la)", descr: "Claude Code tool request"},
		{name: "bash ignores file_path", req: Request{ToolName: "Bash", Input: map[string]any{"command": "ls", "file_path": "/x"}}, key: "Bash(ls)", descr: "Claude Code tool request"},
		{name: "bash without command is bare", req: Request{ToolName: "Bash", Input: map[string]any{"description": "d"}}, key: "Bash", descr: "d"},
		{name: "read file_path", req: Request{ToolName: "Read", Input: map[string]any{"file_path": "/repo/go.mod"}}, key: "Read(/repo/go.mod)", descr: "Claude Code tool request"},
		{name: "notebook path", req: Request{ToolName: "NotebookEdit", Input: map[string]any{"notebook_path": "/n.ipynb"}}, key: "NotebookEdit(/n.ipynb)", descr: "Claude Code tool request"},
		{name: "grep path wins over pattern", req: Request{ToolName: "Grep", Input: map[string]any{"path": "/src", "pattern": "TODO"}}, key: "Grep(/src)", descr: "Claude Code tool request"},
		{name: "glob pattern", req: Request{ToolName: "Glob", Input: map[string]any{"pattern": "**/*.go"}}, key: "Glob(**/*.go)", descr: "Claude Code tool request"},
		{name: "web fetch url", req: Request{ToolName: "WebFetch", Input: map[string]any{"url": "https://atmos.tools"}}, key: "WebFetch(https://atmos.tools)", descr: "Claude Code tool request"},
		{name: "no specifier falls back to bare name", req: Request{ToolName: "Task", Input: map[string]any{"prompt": "x"}}, key: "Task", descr: "Claude Code tool request"},
		{name: "nil input", req: Request{ToolName: "Read"}, key: "Read", descr: "Claude Code tool request"},
		{name: "non-string specifier ignored", req: Request{ToolName: "Bash", Input: map[string]any{"command": 5}}, key: "Bash", descr: "Claude Code tool request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := newRequestTool(tt.req)
			assert.Equal(t, tt.key, tool.CacheKey())
			assert.Equal(t, tt.descr, tool.Description())
			assert.Equal(t, tt.req.ToolName, tool.Name())
		})
	}
}

// TestPermissionApprover_ScopedCacheDoesNotLeak exercises the real CLI prompter cache
// through the approver using a cache-backed checker and no real prompt.
func TestPermissionApprover_ScopedCacheDoesNotLeak(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "base")
	cache, err := permission.NewPermissionCache(basePath)
	require.NoError(t, err)
	require.NoError(t, cache.AddAllow("Bash(atmos list stacks)"))
	require.NoError(t, cache.AddDeny("Bash(atmos destroy)"))

	cfg := &schema.AtmosConfiguration{BasePath: basePath}
	checker, err := permission.NewFromConfig(cfg)
	require.NoError(t, err)
	approver := NewPermissionApprover(checker)

	got, err := approver.Approve(context.Background(), bashRequest)
	require.NoError(t, err)
	assert.True(t, got.Allow, "remembered command is allowed without prompting")

	got, err = approver.Approve(context.Background(), Request{ToolName: "Bash", Input: map[string]any{"command": "atmos destroy"}})
	require.NoError(t, err)
	assert.False(t, got.Allow)
	assert.Equal(t, "Denied by user.", got.Message)
}

func TestPermissionApprover_SerializesApprovals(t *testing.T) {
	var active, maxActive int32
	prompter := &funcPrompter{fn: func() (bool, error) {
		cur := atomic.AddInt32(&active, 1)
		for {
			prev := atomic.LoadInt32(&maxActive)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return true, nil
	}}
	approver := newApprover(t, &schema.AIToolSettings{}, prompter)

	const callers = 6
	var wg sync.WaitGroup
	decisions := make([]Decision, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decisions[i], errs[i] = approver.Approve(context.Background(), bashRequest)
		}()
	}
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		assert.True(t, decisions[i].Allow)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&maxActive), "prompts must never overlap")
}

func TestPermissionApprover_ContextCanceledWhileWaiting(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	prompter := &funcPrompter{fn: func() (bool, error) {
		close(entered)
		<-release
		return true, nil
	}}
	approver := newApprover(t, &schema.AIToolSettings{}, prompter)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = approver.Approve(context.Background(), bashRequest)
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := approver.Approve(ctx, bashRequest)
	assert.ErrorIs(t, err, context.Canceled)

	close(release)
	<-done
}

func TestPermissionApprover_PromptHooks(t *testing.T) {
	tests := []struct {
		name       string
		tools      schema.AIToolSettings
		wantEvents []string
	}{
		{name: "hooks bracket a prompt", tools: schema.AIToolSettings{}, wantEvents: []string{"before", "after"}},
		{name: "no hooks in yolo", tools: schema.AIToolSettings{Mode: "yolo"}},
		{name: "no hooks in allow", tools: schema.AIToolSettings{Mode: "allow"}},
		{name: "no hooks for allowed-list tool", tools: schema.AIToolSettings{Allowed: []string{"Bash"}}},
		{name: "no hooks for blocked tool", tools: schema.AIToolSettings{Mode: "allow", Blocked: []string{"Bash"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			approver := newApprover(
				t, &tt.tools, &stubPrompter{allow: true},
				permission.WithPromptHooks(func() { events = append(events, "before") }, func() { events = append(events, "after") }),
			)

			_, err := approver.Approve(context.Background(), bashRequest)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEvents, events)
		})
	}
}

// funcPrompter adapts a function to permission.Prompter.
type funcPrompter struct {
	fn func() (bool, error)
}

func (f *funcPrompter) Prompt(context.Context, permission.Tool, map[string]interface{}) (bool, error) {
	return f.fn()
}
