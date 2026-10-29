package bridge

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

// Open launches the browser configured for this invocation.
func (s *Server) Open(ctx context.Context, request *apiv1.OpenRequest) (*emptypb.Empty, error) {
	defer perf.Track(nil, "bridge.Open")()

	invocation, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer invocation.done()
	if invocation.Browser == nil {
		return nil, status.Error(codes.Unimplemented, "browser unavailable")
	}
	if !invocation.Interactive {
		return nil, apiError(errUtils.ErrPluginInteractionRequired)
	}
	if err := invocation.Browser.Open(invocation.Context, request.GetUrl()); err != nil {
		return nil, apiError(err)
	}
	return &emptypb.Empty{}, nil
}

// Listen sends readiness before waiting for exactly one matching callback.
func (s *Server) Listen(request *apiv1.ListenRequest, stream grpc.ServerStreamingServer[apiv1.CallbackEvent]) error {
	defer perf.Track(nil, "bridge.Listen")()

	invocation, err := s.authorize(stream.Context())
	if err != nil {
		return err
	}
	defer invocation.done()
	if invocation.Browser == nil {
		return status.Error(codes.Unimplemented, "browser unavailable")
	}
	if !invocation.Interactive {
		return apiError(errUtils.ErrPluginInteractionRequired)
	}
	listener, err := invocation.Browser.Listen(invocation.Context, request.GetPath(), request.GetExpectedState())
	if err != nil {
		return apiError(err)
	}
	defer listener.Close()
	if err := stream.Send(&apiv1.CallbackEvent{Event: &apiv1.CallbackEvent_Ready{Ready: &apiv1.CallbackReady{RedirectUri: listener.RedirectURI()}}}); err != nil {
		return err
	}
	callback, err := listener.Receive(invocation.Context)
	if err != nil {
		return apiError(err)
	}
	return stream.Send(&apiv1.CallbackEvent{Event: &apiv1.CallbackEvent_Callback{Callback: &apiv1.Callback{Code: callback.Code, State: callback.State, Error: callback.Error}}})
}

// Do not return transport errors that might include token-bearing URLs or response bodies.
func apiError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "operation canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "operation timed out")
	case errors.Is(err, errUtils.ErrPluginInteractionRequired):
		return status.Error(codes.FailedPrecondition, "INTERACTION_REQUIRED")
	case errors.Is(err, errUtils.ErrPluginInvalidConfig):
		return status.Error(codes.InvalidArgument, "invalid API request")
	case errors.Is(err, errUtils.ErrPluginPayloadTooLarge):
		return status.Error(codes.ResourceExhausted, "payload exceeds limit")
	default:
		return status.Error(codes.Unavailable, "API operation failed")
	}
}
