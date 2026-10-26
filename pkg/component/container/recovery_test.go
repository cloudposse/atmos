package container

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
)

func TestContainerUpRetryDoesNotRepeatCreation(t *testing.T) {
	recoverytest.Install(t, "container.up.error", "input.error.message.contains('temporarily unavailable')")
	runtime := NewMockRuntime(gomock.NewController(t))
	withStubs(t, map[string]any{"image": "localhost:5001/app:1"}, nil, runtime)
	gomock.InOrder(
		runtime.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "app")).Return([]ctr.Info{}, nil),
		runtime.EXPECT().Create(gomock.Any(), gomock.Any()).Return("id", nil),
		runtime.EXPECT().Start(gomock.Any(), "id").Return(errors.New("temporarily unavailable")),
		runtime.EXPECT().Start(gomock.Any(), "id").Return(nil),
	)
	require.NoError(t, ExecuteUp(context.Background(), infoFor("app")))
}
