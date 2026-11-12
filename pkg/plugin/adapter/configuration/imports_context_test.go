package configuration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestImportInProgressCancellation(t *testing.T) {
	for _, phase := range []string{"loader", "discovery"} {
		for _, origin := range []string{"session", "caller", "deadline"} {
			t.Run(phase+"/"+origin, func(t *testing.T) {
				sessionCtx, cancelSession := context.WithCancel(t.Context())
				defer cancelSession()
				callerCtx, cancelCaller := context.WithCancel(t.Context())
				defer cancelCaller()
				// A generous caller deadline also verifies that composition retains the deadline.
				deadline := time.Now().Add(time.Minute)
				if origin == "deadline" {
					deadline = time.Now().Add(time.Second)
				}
				callerCtx, cancelDeadline := context.WithDeadline(callerCtx, deadline)
				defer cancelDeadline()
				started := make(chan context.Context, 1)
				block := func(ctx context.Context) error {
					started <- ctx
					<-ctx.Done()
					return ctx.Err()
				}
				provider := NewMockProvider(gomock.NewController(t))
				if phase == "discovery" {
					provider.EXPECT().ComponentType().Return("terraform")
					provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
						return nil, block(ctx)
					})
				}
				session, err := New(sessionCtx, importOnlyConfig(t), "list", func(ctx context.Context, _ []string) (map[string]Provider, error) {
					if phase == "loader" {
						return nil, block(ctx)
					}
					return map[string]Provider{"terragrunt": provider}, nil
				})
				require.NoError(t, err)
				done := make(chan error, 1)
				go func() {
					_, _, err := session.ImportComponents(callerCtx, "terragrunt://live")
					done <- err
				}()
				select {
				case ctx := <-started:
					got, ok := ctx.Deadline()
					require.True(t, ok)
					require.Equal(t, deadline, got)
				case <-time.After(5 * time.Second):
					t.Fatal("import did not reach provider")
				}
				switch origin {
				case "session":
					cancelSession()
				case "caller":
					cancelCaller()
				}
				select {
				case err := <-done:
					want := context.Canceled
					if origin == "deadline" {
						want = context.DeadlineExceeded
					}
					require.ErrorIs(t, err, want)
				case <-time.After(5 * time.Second):
					t.Fatal("cancellation did not unblock import")
				}
				require.Empty(t, session.importResults, "failed imports must not be cached")
			})
		}
	}
}
