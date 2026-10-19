package interactive

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// fullClient implements every opt-in interface.
type fullClient struct {
	approver approval.Approver
	handler  func(approval.Event)
	timeout  time.Duration
}

func (c *fullClient) SetApprover(a approval.Approver)                 { c.approver = a }
func (c *fullClient) SetProgressHandler(handler func(approval.Event)) { c.handler = handler }
func (c *fullClient) SetTimeout(d time.Duration)                      { c.timeout = d }

var (
	_ approval.Approvable       = (*fullClient)(nil)
	_ approval.ProgressReporter = (*fullClient)(nil)
	_ approval.TimeoutManaged   = (*fullClient)(nil)
)

func configWithMode(t *testing.T, mode string) *schema.AtmosConfiguration {
	t.Helper()
	cfg := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	cfg.AI.Tools.Mode = mode
	return cfg
}

func TestAttach_FullyInteractiveClient(t *testing.T) {
	client := &fullClient{}

	session, err := Attach(configWithMode(t, "yolo"), client, "Thinking…", 90*time.Second)
	require.NoError(t, err)

	require.NotNil(t, session.Progress)
	assert.True(t, session.TimeoutManaged)
	assert.Equal(t, 90*time.Second, client.timeout)
	assert.NotNil(t, client.handler, "progress events must be subscribed")
	require.NotNil(t, client.approver, "approver must be installed")

	// The installed approver honors ai.tools.mode: yolo allows without prompting.
	decision, err := client.approver.Approve(context.Background(), approval.Request{
		ToolName: "Bash",
		Input:    map[string]any{"command": "atmos list stacks"},
	})
	require.NoError(t, err)
	assert.True(t, decision.Allow)
}

func TestAttach_ClientWithoutOptIn(t *testing.T) {
	session, err := Attach(configWithMode(t, ""), struct{}{}, "Thinking…", time.Minute)
	require.NoError(t, err)

	require.NotNil(t, session.Progress, "every client gets the spinner")
	assert.False(t, session.TimeoutManaged, "the caller keeps owning the timeout")
}

func TestAttach_UnattendedPermissions(t *testing.T) {
	if terminal.New().IsTTY(terminal.Stdin) {
		t.Skip("stdin is a TTY; default approval would prompt")
	}
	for _, tt := range []struct {
		name           string
		mode           string
		blocked        []string
		allow          bool
		nonInteractive bool
	}{
		{name: "default requires a terminal", nonInteractive: true},
		{name: "explicit allow", mode: "allow", allow: true},
		{name: "allow retains blocked tools", mode: "allow", blocked: []string{"Bash"}},
		{name: "yolo bypasses blocked tools", mode: "yolo", blocked: []string{"Bash"}, allow: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configWithMode(t, tt.mode)
			cfg.AI.Tools.Blocked = tt.blocked
			client := &fullClient{}
			// exec attaches with an empty progress message; permissions still apply.
			_, err := Attach(cfg, client, "", time.Minute)
			require.NoError(t, err)
			decision, err := client.approver.Approve(context.Background(), approval.Request{
				ToolName: "Bash",
				Input:    map[string]any{"command": "atmos list stacks"},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.allow, decision.Allow)
			assert.Equal(t, tt.nonInteractive, decision.NonInteractive)
		})
	}
}

func TestAttach_InvalidToolMode(t *testing.T) {
	session, err := Attach(configWithMode(t, "reckless"), &fullClient{}, "Thinking…", time.Minute)

	require.ErrorIs(t, err, errUtils.ErrAIToolsInvalidMode)
	assert.Nil(t, session)
}

func TestAttach_InvalidToolModeIgnoredWithoutApprovalSupport(t *testing.T) {
	// Providers that do their own permission handling never consult ai.tools.mode here;
	// the tool executor reports the bad value for them.
	_, err := Attach(configWithMode(t, "reckless"), struct{}{}, "Thinking…", time.Minute)
	require.NoError(t, err)
}

func TestSession_NewContext(t *testing.T) {
	tests := []struct {
		name         string
		managed      bool
		wantDeadline bool
	}{
		{name: "provider-managed timeout has no deadline", managed: true, wantDeadline: false},
		{name: "other providers get a deadline", managed: false, wantDeadline: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &Session{TimeoutManaged: tt.managed}

			ctx, cancel := session.NewContext(time.Minute)
			defer cancel()

			_, hasDeadline := ctx.Deadline()
			assert.Equal(t, tt.wantDeadline, hasDeadline)

			cancel()
			assert.ErrorIs(t, ctx.Err(), context.Canceled, "cancel must always work, it is how ctrl+c stops the request")
		})
	}
}
