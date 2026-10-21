// Package bridge serves Atmos API services to native plugins within live invocations.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

// InvocationMetadata is set by Plugin SDK calls back into the Atmos API.
const InvocationMetadata = "atmos-invocation-id"

// Invocation scopes API services to a single active call. Namespace includes the
// package, configured provider, realm, and configuration fingerprint.
type Invocation struct {
	Context     context.Context
	Browser     api.Browser
	HTTP        api.HTTP
	Keyring     keyring.Keyring
	Namespace   string
	Interactive bool
}

// Server rejects API requests after their invocation has completed.
type Server struct {
	apiv1.UnimplementedHTTPServer
	apiv1.UnimplementedBrowserServer
	apiv1.UnimplementedCredentialsServer
	storageMu sync.Mutex
	mu        sync.RWMutex
	calls     map[string]*activeInvocation
}

// New creates an isolated API server for one native plugin process.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func New() *Server { return &Server{calls: make(map[string]*activeInvocation)} }

// Register attaches all API services to one broker connection.
func (s *Server) Register(server *grpc.Server) {
	defer perf.Track(nil, "bridge.Server.Register")()

	apiv1.RegisterHTTPServer(server, s)
	apiv1.RegisterBrowserServer(server, s)
	apiv1.RegisterCredentialsServer(server, s)
}

// Begin grants API access until the returned cleanup function is called.
//
//nolint:gocritic // Copy the invocation scope so request context changes cannot alias another call.
func (s *Server) Begin(id string, invocation Invocation) (func(), error) {
	defer perf.Track(nil, "bridge.Server.Begin")()

	if id == "" || invocation.Context == nil || invocation.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid invocation scope")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.calls[id]; exists {
		return nil, status.Error(codes.AlreadyExists, "invocation already active")
	}
	ctx, cancel := context.WithCancel(invocation.Context)
	invocation.Context = ctx
	active := &activeInvocation{Invocation: invocation}
	s.calls[id] = active
	return func() {
		cancel()
		s.mu.Lock()
		delete(s.calls, id)
		s.mu.Unlock()
		active.operations.Wait()
	}, nil
}

type authorizedCall struct {
	Invocation
	done func()
}

func (s *Server) authorize(ctx context.Context) (*authorizedCall, error) {
	values := metadata.ValueFromIncomingContext(ctx, InvocationMetadata)
	if len(values) != 1 {
		return nil, status.Error(codes.PermissionDenied, "missing invocation")
	}
	s.mu.RLock()
	active, exists := s.calls[values[0]]
	if !exists || active.Context.Err() != nil {
		s.mu.RUnlock()
		return nil, status.Error(codes.PermissionDenied, "inactive invocation")
	}
	active.operations.Add(1)
	s.mu.RUnlock()
	call, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(active.Context, cancel)
	scope := active.Invocation
	scope.Context = call
	return &authorizedCall{Invocation: scope, done: func() { stop(); cancel(); active.operations.Done() }}, nil
}

type activeInvocation struct {
	Invocation
	operations sync.WaitGroup
}

func namespacePrefix(namespace string) string {
	sum := sha256.Sum256([]byte(namespace))
	return "atmos_plugin_" + hex.EncodeToString(sum[:]) + "_"
}
