package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// approvalClient records the optional provider approval and timeout configuration.
type approvalClient struct {
	mockAIClient
	approver approval.Approver
	timeout  time.Duration
}

type approvalTerminal struct {
	released, restored     int
	releaseErr, restoreErr error
}

func (t *approvalTerminal) ReleaseTerminal() error { t.released++; return t.releaseErr }
func (t *approvalTerminal) RestoreTerminal() error { t.restored++; return t.restoreErr }

type approvalPrompter struct {
	allowed, cached bool
	err             error
	calls           int
}

func (p *approvalPrompter) Prompt(context.Context, permission.Tool, map[string]interface{}) (bool, error) {
	p.calls++
	return p.allowed, p.err
}

func (p *approvalPrompter) HasCachedDecision(permission.Tool) bool { return p.cached }

func TestChatApprovalTerminalHandoff(t *testing.T) {
	for _, tt := range []struct {
		name                              string
		cached                            bool
		promptErr, releaseErr, restoreErr error
		wantAllowed                       bool
		calls, released, restored         int
	}{
		{name: "allow", wantAllowed: true, calls: 1, released: 1, restored: 1},
		{name: "cancel restores terminal", promptErr: context.Canceled, calls: 1, released: 1, restored: 1},
		{name: "cached decision keeps chat active", cached: true, wantAllowed: true, calls: 1},
		{name: "release failure prevents prompt", releaseErr: context.Canceled, released: 1},
		{name: "restore failure denies", restoreErr: context.Canceled, calls: 1, released: 1, restored: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			terminal := &approvalTerminal{releaseErr: tt.releaseErr, restoreErr: tt.restoreErr}
			inner := &approvalPrompter{allowed: tt.promptErr == nil, cached: tt.cached, err: tt.promptErr}
			prompter := &chatPrompter{inner: inner, terminal: func() chatTerminal { return terminal }}
			allowed, err := prompter.Prompt(context.Background(), nil, nil)
			assert.Equal(t, tt.wantAllowed, allowed)
			if wantErr := errors.Join(tt.promptErr, tt.releaseErr, tt.restoreErr); wantErr != nil {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.calls, inner.calls)
			assert.Equal(t, tt.released, terminal.released)
			assert.Equal(t, tt.restored, terminal.restored)
		})
	}
}

func TestChatTimeoutOwnership(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.AI.TimeoutSeconds = 7
	for _, managed := range []bool{false, true} {
		model := &ChatModel{atmosConfig: config, client: &mockAIClient{}}
		if managed {
			model.client = &approvalClient{}
		}
		ctx, cancel := model.requestContext()
		_, hasDeadline := ctx.Deadline()
		assert.Equal(t, !managed, hasDeadline, "managed providers pause their own timeout for approvals")
		cancel()
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	}
}

func (c *approvalClient) SetApprover(a approval.Approver)  { c.approver = a }
func (c *approvalClient) SetTimeout(timeout time.Duration) { c.timeout = timeout }

func TestChatProviderPermissions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mode    string
		blocked []string
		allowed bool
	}{
		{name: "blocked", mode: "allow", blocked: []string{"mcp__field_test__write_marker"}},
		{name: "allow", mode: "allow", allowed: true},
		{name: "yolo", mode: "yolo", blocked: []string{"mcp__field_test__write_marker"}, allowed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			config.AI.Tools.Mode = tt.mode
			config.AI.Tools.Blocked = tt.blocked
			config.AI.TimeoutSeconds = 7
			client := &approvalClient{}
			_, err := NewChatModel(ChatModelParams{Client: client, AtmosConfig: config})
			require.NoError(t, err)
			require.NotNil(t, client.approver, "chat must attach the same policy as ask/exec")
			decision, err := client.approver.Approve(context.Background(), approval.Request{ToolName: "mcp__field_test__write_marker", Input: map[string]any{"label": "test"}})
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, decision.Allow)
			assert.Equal(t, 7*time.Second, client.timeout)
		})
	}
}
