package bridge

import (
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
	"google.golang.org/grpc"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_api_test.go -package=bridge github.com/cloudposse/atmos/pkg/api HTTP,Browser,CallbackListener
//go:generate go run go.uber.org/mock/mockgen -source=contracts_test.go -destination=mock_callback_test.go -package=bridge

type callbackStream interface {
	grpc.ServerStreamingServer[apiv1.CallbackEvent]
}

var _ callbackStream = (*MockcallbackStream)(nil)
