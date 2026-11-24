package bridge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

// TestListenPausesCallTimeoutOnlyWhileWaitingForCallback proves the browser sign-in wait is
// reported as a user wait: it starts after the redirect URI is ready and ends once the callback
// arrives or fails, so the plugin's own work around it still counts against the call timeout.
func TestListenPausesCallTimeoutOnlyWhileWaitingForCallback(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		receive error
		want    codes.Code
	}{
		{name: "callback received", want: codes.OK},
		{name: "callback canceled", receive: context.Canceled, want: codes.Canceled},
		{name: "interaction refused", receive: errUtils.ErrPluginInteractionRequired, want: codes.FailedPrecondition},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			controller := gomock.NewController(t)
			browser := NewMockBrowser(controller)
			var events []string
			awaitUser := func() func() {
				events = append(events, "await")
				return func() { events = append(events, "resume") }
			}
			server, ctx := bridgeScope(t, &Invocation{Browser: browser, Interactive: true, AwaitUser: awaitUser})
			listener := NewMockCallbackListener(controller)
			browser.EXPECT().Listen(gomock.Any(), "/callback", "state").Return(listener, nil)
			listener.EXPECT().RedirectURI().Return("http://127.0.0.1/callback")
			listener.EXPECT().Receive(gomock.Any()).DoAndReturn(func(context.Context) (api.Callback, error) {
				events = append(events, "receive")
				return api.Callback{Code: "code", State: "state"}, tt.receive
			})
			listener.EXPECT().Close().Return(nil)
			stream := NewMockcallbackStream(controller)
			stream.EXPECT().Context().Return(ctx)
			stream.EXPECT().Send(gomock.Any()).DoAndReturn(func(event *apiv1.CallbackEvent) error {
				if event.GetReady() != nil {
					events = append(events, "ready")
				} else {
					events = append(events, "callback")
				}
				return nil
			}).MinTimes(1)

			err := server.Listen(&apiv1.ListenRequest{Path: "/callback", ExpectedState: "state"}, stream)

			assert.Equal(t, tt.want, status.Code(err))
			want := []string{"ready", "await", "receive", "resume"}
			if tt.receive == nil {
				want = append(want, "callback")
			}
			require.Equal(t, want, events)
		})
	}
}
