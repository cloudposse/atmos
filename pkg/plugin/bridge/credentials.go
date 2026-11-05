package bridge

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/perf"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

// Get reads opaque private session state without applying access-token expiry.
func (s *Server) Get(ctx context.Context, request *apiv1.GetRequest) (*apiv1.GetResponse, error) {
	defer perf.Track(nil, "bridge.Get")()

	invocation, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer invocation.done()
	if invocation.Keyring == nil {
		return nil, status.Error(codes.Unimplemented, "session storage unavailable")
	}
	session, err := readSession(invocation.Keyring, invocation.Namespace)
	if err != nil {
		return nil, apiError(err)
	}
	value, found := session.Values[request.GetKey()]
	return &apiv1.GetResponse{Found: found, Data: value}, nil
}

// Put persists a replacement session before the plugin publishes new credentials.
func (s *Server) Put(ctx context.Context, request *apiv1.PutRequest) (*emptypb.Empty, error) {
	defer perf.Track(nil, "bridge.Put")()

	invocation, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer invocation.done()
	if invocation.Keyring == nil {
		return nil, status.Error(codes.Unimplemented, "session storage unavailable")
	}
	if len(request.GetData()) > api.MaxHTTPBody {
		return nil, status.Error(codes.ResourceExhausted, "session exceeds limit")
	}
	if request.GetKey() == "" || len(request.GetKey()) > maxSessionKeyBytes {
		return nil, status.Error(codes.InvalidArgument, "invalid session key")
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	session, err := readSession(invocation.Keyring, invocation.Namespace)
	if err != nil {
		return nil, apiError(err)
	}
	session.Values[request.GetKey()] = request.GetData()
	if err := writeSession(invocation.Keyring, invocation.Namespace, session); err != nil {
		return nil, apiError(err)
	}
	return &emptypb.Empty{}, nil
}

// Delete removes session state even after its access token has expired.
func (s *Server) Delete(ctx context.Context, request *apiv1.DeleteRequest) (*emptypb.Empty, error) {
	defer perf.Track(nil, "bridge.Delete")()

	invocation, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer invocation.done()
	if invocation.Keyring == nil {
		return nil, status.Error(codes.Unimplemented, "session storage unavailable")
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	session, err := readSession(invocation.Keyring, invocation.Namespace)
	if err != nil {
		return nil, apiError(err)
	}
	delete(session.Values, request.GetKey())
	if err := writeSession(invocation.Keyring, invocation.Namespace, session); err != nil {
		return nil, apiError(err)
	}
	return &emptypb.Empty{}, nil
}
