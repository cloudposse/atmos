package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestLogFinalizationBoundsSendAndClose(t *testing.T) {
	for _, queued := range []int{0, api.DefaultLogQueue} {
		t.Run(fmt.Sprint(queued), func(t *testing.T) {
			started := make(chan struct{})
			stall := grpc.UnaryInterceptor(func(ctx context.Context, _ any, _ *grpc.UnaryServerInfo, _ grpc.UnaryHandler) (any, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			})
			provider := New(nil, WithAPI(apiClient(t, &logReceiver{}, stall)))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			_, logs := provider.withLogs(ctx, &pluginv1.Invocation{Id: "stalled", LogLevel: "debug"}, t.TempDir())
			assert.Equal(t, 5*time.Second, logs.flush)
			logs.flush = 100 * time.Millisecond
			// Keep one write in flight, then fill the queue before finalizing buffered detail.
			logs.stream.Send(&pluginv1.LogEntry{Message: "progress"})
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("log write never started")
			}
			for range queued {
				logs.stream.Send(&pluginv1.LogEntry{Message: "progress"})
			}
			for range 2 {
				require.NoError(t, logs.sink.Hook().Fire(&logrus.Entry{Level: log.DebugLevel.ToLogrusLevel(), Message: "buffered detail"}))
			}
			result := make(chan []*pluginv1.LogEntry, 1)
			go func() { result <- logs.entries() }()
			select {
			case entries := <-result:
				assert.Equal(t, []string{fmt.Sprintf("%d log events were not shown", queued+3)}, messagesOf(entries))
				assert.NoError(t, ctx.Err(), "log finalization must not cancel the provider call")
				assert.Equal(t, entries, logs.entries(), "finalization must not repeat delivery")
			case <-time.After(3 * time.Second):
				cancel()
				<-result
				t.Fatal("log finalization exceeded its configured bound")
			}
		})
	}
}
