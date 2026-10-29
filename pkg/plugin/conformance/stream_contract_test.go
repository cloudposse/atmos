package conformance

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

//go:generate go run go.uber.org/mock/mockgen -source=stream_contract_test.go -destination=mock_stream_test.go -package=conformance

type credentialStream interface {
	grpc.ServerStreamingClient[pluginv1.AuthenticateEvent]
}

type reportStreams interface {
	iolib.Streams
}

var (
	_ credentialStream = (*MockcredentialStream)(nil)
	_ reportStreams    = (*MockreportStreams)(nil)
)

func TestCredentialStreamRejectsTrailingAndMissingResults(t *testing.T) {
	t.Parallel()
	result := &pluginv1.Credentials{Type: "oidc/v1"}
	event := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: result}}
	for _, tt := range []struct {
		name     string
		events   []*pluginv1.AuthenticateEvent
		terminal error
		expected error
	}{
		{"progress then result", []*pluginv1.AuthenticateEvent{{}, event}, io.EOF, nil},
		{"missing result", nil, io.EOF, io.EOF},
		{"trailing result", []*pluginv1.AuthenticateEvent{event, event}, nil, errUtils.ErrPluginProtocol},
		{"unbounded progress", make([]*pluginv1.AuthenticateEvent, maxCredentialEvents), nil, errUtils.ErrPluginProtocol},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := NewMockcredentialStream(gomock.NewController(t))
			calls := make([]any, 0, len(tt.events)+1)
			for _, entry := range tt.events {
				calls = append(calls, stream.EXPECT().Recv().Return(entry, nil))
			}
			if tt.terminal != nil {
				calls = append(calls, stream.EXPECT().Recv().Return(nil, tt.terminal))
			}
			gomock.InOrder(calls...)
			actual, err := receiveCredentials(stream)
			if tt.expected != nil {
				require.ErrorIs(t, err, tt.expected)
				assert.Nil(t, actual)
			} else {
				require.NoError(t, err)
				assert.Equal(t, result, actual)
			}
		})
	}
}
