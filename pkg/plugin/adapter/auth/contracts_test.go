package auth

import (
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"google.golang.org/grpc"
)

//go:generate go run go.uber.org/mock/mockgen -source=identity.go -destination=mock_materializer_test.go -package=auth
//go:generate go run go.uber.org/mock/mockgen -source=contracts_test.go -destination=mock_stream_test.go -package=auth

type credentialStream interface {
	grpc.ServerStreamingClient[pluginv1.AuthenticateEvent]
}

var _ credentialStream = (*MockcredentialStream)(nil)
