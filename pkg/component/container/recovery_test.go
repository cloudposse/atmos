package container

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cachepkg "github.com/cloudposse/atmos/pkg/cache"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestContainerListRetryKeepsFallbacks(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, rule := range []string{"false", "true"} {
			t.Run(fmt.Sprintf("cached=%t/rule=%s", cached, rule), func(t *testing.T) {
				recoverytest.Install(t, "container.list.error", rule)
				initListTestIO(t)
				runtime := NewMockRuntime(gomock.NewController(t))
				withListStubs(t, map[string]any{"dev": containerStack(map[string]string{"app": "app:dev"})}, nil, runtime, nil)
				if cached {
					seedUnavailableRuntime(t)
				}
				attempts := 1
				if rule == "true" {
					attempts = 3
				}
				runtime.EXPECT().List(gomock.Any(), ctr.ComponentTypeFilter("container")).Return(nil, errors.New("unavailable")).Times(attempts)
				require.NoError(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}))
			})
		}
	}
}

func seedUnavailableRuntime(t *testing.T) {
	t.Helper()
	// The cached Docker selection must fail without accessing a real daemon.
	t.Setenv("PATH", t.TempDir())
	cache, err := cachepkg.NewFileCache(runtimeCacheSubpath)
	require.NoError(t, err)
	storeCachedRuntime(cache, runtimeSelectionCacheKey(""), ctr.NewDockerRuntime())
}

func TestContainerListRetryPropagatesPolicyAndCancellation(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached=%t", cached), func(t *testing.T) {
			recoverytest.Install(t, "container.list.error", "input.missing.value")
			initListTestIO(t)
			runtime := NewMockRuntime(gomock.NewController(t))
			withListStubs(t, map[string]any{"dev": containerStack(map[string]string{"app": "app:dev"})}, nil, runtime, nil)
			if cached {
				seedUnavailableRuntime(t)
			} else {
				runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("unavailable"))
			}
			require.ErrorIs(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}), errUtils.ErrPolicyEvaluation)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			require.ErrorIs(t, ExecuteList(ctx, &schema.ConfigAndStacksInfo{}), context.Canceled)
		})
	}
}

func TestContainerListRediscoveryPolicyFailure(t *testing.T) {
	recoverytest.Install(t, "container.list.error", "input.error.message == 'unavailable' && input.missing.value")
	initListTestIO(t)
	runtime := NewMockRuntime(gomock.NewController(t))
	withListStubs(t, map[string]any{"dev": containerStack(map[string]string{"app": "app:dev"})}, nil, runtime, nil)
	seedUnavailableRuntime(t)
	runtime.EXPECT().List(gomock.Any(), ctr.ComponentTypeFilter("container")).Return(nil, errors.New("unavailable"))
	require.ErrorIs(t, ExecuteList(t.Context(), &schema.ConfigAndStacksInfo{}), errUtils.ErrPolicyEvaluation)
}
