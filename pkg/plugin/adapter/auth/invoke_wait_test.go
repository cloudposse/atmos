package auth

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// TestCredentialStreamWaitsForUserFromPromptUntilResult pins when the call timeout pauses: from
// the moment the prompt is shown until the credential result arrives. Progress the plugin sends
// while it polls does not end the wait, and a refused prompt never starts one.
func TestCredentialStreamWaitsForUserFromPromptUntilResult(t *testing.T) {
	t.Parallel()
	prompt := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Interaction{Interaction: deviceInteraction()}}
	progress := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Progress{Progress: &pluginv1.Progress{}}}
	credentials := &pluginv1.Credentials{Type: "oidc/v1"}
	result := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: credentials}}
	accept := func(*pluginv1.Interaction) error { return nil }

	t.Run("wait spans the prompt and polling", func(t *testing.T) {
		t.Parallel()
		var events []string
		stream := NewMockcredentialStream(gomock.NewController(t))
		record := func(event *pluginv1.AuthenticateEvent, name string) func() (*pluginv1.AuthenticateEvent, error) {
			return func() (*pluginv1.AuthenticateEvent, error) {
				events = append(events, name)
				return event, nil
			}
		}
		gomock.InOrder(
			stream.EXPECT().Recv().DoAndReturn(record(prompt, "prompt")),
			stream.EXPECT().Recv().DoAndReturn(record(progress, "progress")),
			stream.EXPECT().Recv().DoAndReturn(record(result, "result")),
			stream.EXPECT().Recv().Return(nil, io.EOF),
		)
		awaitUser := func() func() {
			events = append(events, "await")
			return func() { events = append(events, "resume") }
		}

		actual, err := receiveCredentials(stream, accept, awaitUser)

		require.NoError(t, err)
		require.Equal(t, credentials, actual)
		// The runtime's resume is idempotent, so the deferred second call is harmless.
		require.Equal(t, []string{"prompt", "await", "progress", "result", "resume", "resume"}, events,
			"the wait ends when the result arrives, not on progress")
	})

	t.Run("refused prompt never waits", func(t *testing.T) {
		t.Parallel()
		stream := NewMockcredentialStream(gomock.NewController(t))
		stream.EXPECT().Recv().Return(prompt, nil)
		awaited := false

		_, err := receiveCredentials(stream, func(*pluginv1.Interaction) error { return errUtils.ErrPluginInteractionRequired },
			func() func() { awaited = true; return func() {} })

		require.ErrorIs(t, err, errUtils.ErrPluginInteractionRequired)
		require.False(t, awaited)
	})

	t.Run("stream failure during the wait still ends it", func(t *testing.T) {
		t.Parallel()
		stream := NewMockcredentialStream(gomock.NewController(t))
		gomock.InOrder(
			stream.EXPECT().Recv().Return(prompt, nil),
			stream.EXPECT().Recv().Return(nil, context.Canceled),
		)
		resumed := false

		_, err := receiveCredentials(stream, accept, func() func() { return func() { resumed = true } })

		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
		require.True(t, resumed)
	})
}
