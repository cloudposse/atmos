package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields the tests rely on.
var _ = schema.MCPServerConfig{Command: "uvx", Args: []string{"docs@latest"}}

// testExecutable returns the path to the running test binary, which impersonates the
// claude CLI when the fake scenario env var is set (see testmain_test.go).
func testExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe
}

// newFakeClient returns a client wired to the fake claude playing the scenario, plus the path of its record file.
func newFakeClient(t *testing.T, scenario string) (*Client, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "fake.log")
	t.Setenv(fakeScenarioEnv, scenario)
	t.Setenv(fakeLogEnv, logPath)
	return &Client{binaryPath: testExecutable(t), model: ProviderName, maxTurns: 3}, logPath
}

// readFakeLog returns every entry the fake recorded, in order.
func readFakeLog(t *testing.T, path string) []fakeLogEntry {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	var entries []fakeLogEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		var e fakeLogEntry
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &e))
		entries = append(entries, e)
	}
	require.NoError(t, scanner.Err())
	return entries
}

// fakeArgs returns the argv the fake received.
func fakeArgs(t *testing.T, entries []fakeLogEntry) []string {
	t.Helper()
	for _, e := range entries {
		if e.Kind == "args" {
			return e.Args
		}
	}
	require.Fail(t, "fake recorded no args")
	return nil
}

// fakeApprover is an approval.Approver driven by a function; it records every request.
type fakeApprover struct {
	mu       sync.Mutex
	requests []approval.Request
	fn       func(ctx context.Context, req approval.Request) (approval.Decision, error)
}

func (f *fakeApprover) Approve(ctx context.Context, req approval.Request) (approval.Decision, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	return f.fn(ctx, req)
}

func (f *fakeApprover) calls() []approval.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]approval.Request(nil), f.requests...)
}

func decide(d approval.Decision) *fakeApprover {
	return &fakeApprover{fn: func(context.Context, approval.Request) (approval.Decision, error) { return d, nil }}
}

func TestExecClaude_PlainMode(t *testing.T) {
	c, logPath := newFakeClient(t, scenarioPlain)

	out, err := c.SendMessage(t.Context(), "what is up?")
	require.NoError(t, err)
	assert.Equal(t, "hello from claude", out)

	entries := readFakeLog(t, logPath)
	args := fakeArgs(t, entries)
	assert.Contains(t, args, "stream-json")
	assert.Contains(t, args, "--verbose")
	assert.NotContains(t, args, "--input-format")
	assert.NotContains(t, args, "--permission-prompt-tool")

	// Without an approver the prompt is plain text on stdin.
	require.Len(t, entries, 2)
	assert.Equal(t, "what is up?", entries[1].Line)
}

func TestExecClaude_ApproverModeSendsStreamJSONPrompt(t *testing.T) {
	c, logPath := newFakeClient(t, scenarioPlain)
	c.SetApprover(decide(approval.Decision{Allow: true}))

	out, err := c.SendMessage(t.Context(), "what is up?")
	require.NoError(t, err)
	assert.Equal(t, "hello from claude", out)

	entries := readFakeLog(t, logPath)
	args := fakeArgs(t, entries)
	assert.Contains(t, args, "--input-format")
	assert.Contains(t, args, "--permission-prompt-tool")
	assert.Equal(t, "stdio", args[indexOf(args, "--permission-prompt-tool")+1])

	require.Len(t, entries, 2)
	assert.JSONEq(t,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"what is up?"}]}}`,
		entries[1].Line)
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestExecClaude_ProgressEvents(t *testing.T) {
	c, _ := newFakeClient(t, scenarioTools)
	var events []approval.Event
	c.SetProgressHandler(func(e approval.Event) { events = append(events, e) })

	out, err := c.SendMessage(t.Context(), "list")
	require.NoError(t, err)
	assert.Equal(t, "tools done", out)

	assert.Equal(t, []approval.Event{
		{Kind: approval.ToolStart, Tool: "Bash", Summary: "atmos list stacks"},
		{Kind: approval.ToolDone, Tool: "Bash", Summary: "atmos list stacks"},
		{Kind: approval.ToolStart, Tool: "Read", Summary: "/tmp/a.txt"},
		{Kind: approval.ToolDone, Tool: "Read", Summary: "/tmp/a.txt"},
	}, events)
}

func TestExecClaude_ApprovalDecisions(t *testing.T) {
	tests := []struct {
		name      string
		decision  approval.Decision
		approvErr error
		assertOut func(t *testing.T, out string, err error)
		wantLogs  []string // Substrings expected in the control_response Claude received.
	}{
		{
			name:     "allow echoes the original input",
			decision: approval.Decision{Allow: true},
			assertOut: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Contains(t, out, "behavior=allow")
				assert.Contains(t, out, `"command":"touch x"`)
				assert.Contains(t, out, "interrupt=false")
			},
			wantLogs: []string{`"request_id":"req-1"`},
		},
		{
			name:     "deny continues the run with the message",
			decision: approval.Decision{Message: "Not this one"},
			assertOut: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Contains(t, out, "behavior=deny")
				assert.Contains(t, out, "interrupt=false")
				assert.Contains(t, out, `message="Not this one"`)
			},
		},
		{
			name:     "deny without a message uses a default",
			decision: approval.Decision{},
			assertOut: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Contains(t, out, "behavior=deny")
				assert.Contains(t, out, defaultDenyMessage)
			},
		},
		{
			name:     "interrupt aborts the run",
			decision: approval.Decision{Interrupt: true, Message: "stop"},
			assertOut: func(t *testing.T, out string, err error) {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
				assert.NotErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
				assert.Empty(t, out)
			},
			wantLogs: []string{`"interrupt":true`},
		},
		{
			name:     "non-interactive reports the tool as denied",
			decision: approval.Decision{NonInteractive: true, Message: "no terminal"},
			assertOut: func(t *testing.T, out string, err error) {
				require.ErrorIs(t, err, errUtils.ErrCLIProviderToolDenied)
				assert.Empty(t, out)
				assert.Equal(t, 3, len(cockroachErrors.GetAllHints(err)))
				tool, _ := errUtils.GetContext(err, "tool")
				assert.Equal(t, "Bash", tool)
				assert.Contains(t, cockroachErrors.GetAllDetails(err), "Claude Code needed approval to run: touch x")
			},
			wantLogs: []string{`"message":"no terminal"`},
		},
		{
			name:      "approver error is returned and the tool is denied",
			approvErr: errUtils.ErrUserAborted,
			assertOut: func(t *testing.T, out string, err error) {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
				assert.Empty(t, out)
			},
			wantLogs: []string{`"behavior":"deny"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, logPath := newFakeClient(t, scenarioAsk)
			approver := &fakeApprover{fn: func(context.Context, approval.Request) (approval.Decision, error) {
				return tt.decision, tt.approvErr
			}}
			c.SetApprover(approver)

			out, err := c.SendMessage(t.Context(), "touch it")
			tt.assertOut(t, out, err)

			// The approver saw exactly the request Claude sent.
			calls := approver.calls()
			require.Len(t, calls, 1)
			assert.Equal(t, "Bash", calls[0].ToolName)
			assert.Equal(t, "toolu_1", calls[0].ToolUseID)
			assert.Equal(t, "touch x", calls[0].Input["command"])

			var response string
			for _, e := range readFakeLog(t, logPath) {
				if e.Kind == "stdin_response" {
					response = e.Line
				}
			}
			for _, want := range tt.wantLogs {
				assert.Contains(t, response, want)
			}
		})
	}
}

// TestExecClaude_InterruptSkipsApproverForLaterRequests verifies that once a run is being
// torn down, further parallel tool requests are refused without asking the approver again.
func TestExecClaude_InterruptSkipsApproverForLaterRequests(t *testing.T) {
	c, _ := newFakeClient(t, scenarioAskTwo)
	approver := decide(approval.Decision{Interrupt: true})
	c.SetApprover(approver)

	_, err := c.SendMessage(t.Context(), "two things")
	require.ErrorIs(t, err, errUtils.ErrUserAborted)
	assert.Len(t, approver.calls(), 1)
}

// TestExecClaude_DenyAllowsLaterRequests is the negative path of the test above: a plain deny
// must keep consulting the approver for the next request.
func TestExecClaude_DenyAllowsLaterRequests(t *testing.T) {
	c, _ := newFakeClient(t, scenarioAskTwo)
	n := 0
	approver := &fakeApprover{fn: func(context.Context, approval.Request) (approval.Decision, error) {
		n++
		if n == 1 {
			return approval.Decision{Message: "first no"}, nil
		}
		return approval.Decision{Allow: true}, nil
	}}
	c.SetApprover(approver)

	out, err := c.SendMessage(t.Context(), "two things")
	require.NoError(t, err)
	assert.Equal(t, `r1=deny/false/"first no";r2=allow/false/""`, out)
	require.Len(t, approver.calls(), 2)
	assert.Equal(t, "touch two", approver.calls()[1].Input["command"])
}

func TestExecClaude_ResultErrors(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		wantErr  error
		wantText string // Substring of err.Error() or of its context.
	}{
		{"empty result", scenarioEmpty, errUtils.ErrCLIProviderEmptyResponse, ""},
		{"max turns", scenarioMaxTurns, errUtils.ErrCLIProviderMaxTurns, "6"},
		{"is_error result", scenarioIsError, errUtils.ErrCLIProviderExecFailed, "Authentication expired"},
		{"permission denials without approver", scenarioDenials, errUtils.ErrCLIProviderToolDenied, "Bash"},
		{"process crash", scenarioCrash, errUtils.ErrCLIProviderExecFailed, "boom: claude crashed"},
		{"no result message", scenarioNoResult, errUtils.ErrCLIProviderParseResponse, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeClient(t, tt.scenario)
			out, err := c.SendMessage(t.Context(), "hi")
			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, out)
			if tt.wantText != "" {
				assert.Contains(t, err.Error()+" "+contextValues(err), tt.wantText)
			}
		})
	}
}

// contextValues returns the builder context values attached to err.
func contextValues(err error) string {
	var values []string
	for _, key := range []string{"tool", "turns", "provider"} {
		if v, ok := errUtils.GetContext(err, key); ok {
			values = append(values, v)
		}
	}
	return strings.Join(values, " ")
}

// TestExecClaude_ForeignDenialsStillReportedWithApprover is the negative path of the user-denial
// handling: a denial the approver did not make (Claude refused on its own) is still an error.
func TestExecClaude_ForeignDenialsStillReportedWithApprover(t *testing.T) {
	c, _ := newFakeClient(t, scenarioDenials)
	approver := decide(approval.Decision{Allow: true})
	c.SetApprover(approver)

	_, err := c.SendMessage(t.Context(), "hi")
	require.ErrorIs(t, err, errUtils.ErrCLIProviderToolDenied)
	assert.Empty(t, approver.calls())
}

func TestExecClaude_DenialsNeverReturnModelProse(t *testing.T) {
	c, _ := newFakeClient(t, scenarioDenials)
	out, err := c.SendMessage(t.Context(), "hi")
	require.ErrorIs(t, err, errUtils.ErrCLIProviderToolDenied)
	assert.NotContains(t, out, "approval")
	assert.Len(t, cockroachErrors.GetAllHints(err), 3)
}

func TestExecClaude_MalformedLinesAreSkipped(t *testing.T) {
	c, _ := newFakeClient(t, scenarioGarbage)
	out, err := c.SendMessage(t.Context(), "hi")
	require.NoError(t, err)
	assert.Equal(t, "survived garbage", out)
}

func TestExecClaude_HugeLines(t *testing.T) {
	c, _ := newFakeClient(t, scenarioHuge)
	out, err := c.SendMessage(t.Context(), strings.Repeat("p", hugeSize))
	require.NoError(t, err)
	require.Len(t, out, hugeSize)
	assert.Equal(t, "y", out[:1])
	assert.Equal(t, "y", out[len(out)-1:])
}

func TestExecClaude_TimeoutFires(t *testing.T) {
	c, _ := newFakeClient(t, scenarioHang)
	c.SetTimeout(300 * time.Millisecond)

	start := time.Now()
	_, err := c.SendMessage(t.Context(), "hi")
	require.ErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 30*time.Second)
}

func TestExecClaude_TimeoutFiresWithApprover(t *testing.T) {
	c, _ := newFakeClient(t, scenarioHang)
	c.SetApprover(decide(approval.Decision{Allow: true}))
	c.SetTimeout(300 * time.Millisecond)

	_, err := c.SendMessage(t.Context(), "hi")
	require.ErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestExecClaude_TimeoutPausedDuringApproval verifies that human time in the approver does
// not count toward the timeout.
func TestExecClaude_TimeoutPausedDuringApproval(t *testing.T) {
	// The timeout is generous because the fake claude is the full test binary, which takes a
	// moment to start; the approver then blocks for longer than the whole timeout.
	const timeout = 4 * time.Second
	c, _ := newFakeClient(t, scenarioAsk)
	c.SetTimeout(timeout)
	c.SetApprover(&fakeApprover{fn: func(context.Context, approval.Request) (approval.Decision, error) {
		time.Sleep(timeout + 500*time.Millisecond)
		return approval.Decision{Allow: true}, nil
	}})

	out, err := c.SendMessage(t.Context(), "touch it")
	require.NoError(t, err)
	assert.Contains(t, out, "behavior=allow")
}

func TestExecClaude_ContextCancelKillsProcess(t *testing.T) {
	c, _ := newFakeClient(t, scenarioHang)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(300*time.Millisecond, cancel)

	start := time.Now()
	_, err := c.SendMessage(ctx, "hi")
	require.ErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 30*time.Second)
}

func TestExecClaude_ApproverContextCancelled(t *testing.T) {
	c, _ := newFakeClient(t, scenarioAsk)
	ctx, cancel := context.WithCancel(t.Context())
	c.SetApprover(&fakeApprover{fn: func(ctx context.Context, _ approval.Request) (approval.Decision, error) {
		cancel()
		<-ctx.Done()
		return approval.Decision{}, ctx.Err()
	}})

	_, err := c.SendMessage(ctx, "touch it")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestExecClaude_ResultThenHangIsKilledAfterGrace(t *testing.T) {
	old := finishGrace
	finishGrace = 300 * time.Millisecond
	t.Cleanup(func() { finishGrace = old })

	c, _ := newFakeClient(t, scenarioResultThenHang)
	out, err := c.SendMessage(t.Context(), "hi")
	require.NoError(t, err)
	assert.Equal(t, "done but lingering", out)
}

func TestExecClaude_MCPConfigIsWrittenPerInvocationAndRemoved(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{"aws-docs": {Command: "uvx", Args: []string{"docs@latest"}}}
	c, logPath := newFakeClient(t, scenarioPlain)
	c.mcpServers = servers

	for i := 0; i < 2; i++ {
		out, err := c.SendMessage(t.Context(), "hi")
		require.NoError(t, err)
		assert.Equal(t, "hello from claude", out)
		assert.Empty(t, c.mcpConfigPath, "no config path is kept after the run")
	}

	var checks []fakeLogEntry
	for _, e := range readFakeLog(t, logPath) {
		if e.Kind == "mcp" {
			checks = append(checks, e)
		}
	}
	require.Len(t, checks, 2)
	for _, check := range checks {
		assert.True(t, check.Exists, "the MCP config must exist while claude runs")
		_, statErr := os.Stat(check.Path)
		assert.True(t, os.IsNotExist(statErr), "the MCP config must be removed once the run ends: %s", check.Path)
	}
	assert.NotEqual(t, checks[0].Path, checks[1].Path, "every invocation gets its own file")
}

func TestExecClaude_NoMCPServersWritesNoConfig(t *testing.T) {
	c, logPath := newFakeClient(t, scenarioPlain)

	_, err := c.SendMessage(t.Context(), "hi")
	require.NoError(t, err)

	for _, e := range readFakeLog(t, logPath) {
		assert.NotEqual(t, "mcp", e.Kind, "no --mcp-config without MCP servers")
	}
}

func TestStartFailure(t *testing.T) {
	c := &Client{binaryPath: filepath.Join(t.TempDir(), "does-not-exist"), maxTurns: 1}
	_, err := c.SendMessage(t.Context(), "hi")
	assert.ErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
}
