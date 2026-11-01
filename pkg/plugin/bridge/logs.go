package bridge

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_logs_test.go -package=bridge

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// maxLogBatch bounds the entries one Write call may carry, so a plugin cannot hold the terminal with one call.
const maxLogBatch = 1024

// LogSink shows the log entries a plugin delivers during its invocation. Write returns after the entries are
// shown, which is what lets a plugin order its entries before its result. It must not fail the invocation.
type LogSink interface {
	WriteLogs(context.Context, []*pluginv1.LogEntry)
}

// Write shows entries delivered by the active invocation. An invocation without a sink, or an unknown or
// finished one, delivers nothing; the plugin then returns the entries in its response instead.
func (s *Server) Write(ctx context.Context, request *pluginv1.LogRequest) (*emptypb.Empty, error) {
	defer perf.Track(nil, "bridge.Server.Write")()

	call, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer call.done()
	if call.Logs == nil {
		return nil, status.Error(codes.PermissionDenied, "log delivery not granted")
	}
	if len(request.GetEntries()) > maxLogBatch {
		return nil, status.Error(codes.InvalidArgument, "too many log entries")
	}
	call.Logs.WriteLogs(call.Context, request.GetEntries())
	return &emptypb.Empty{}, nil
}
