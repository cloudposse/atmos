package output

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutor_OutputCacheSeparatesAuthentication(t *testing.T) {
	for _, method := range []string{"output", "options", "all"} {
		for _, source := range []string{"context", "manager"} {
			t.Run(method+"/"+source, func(t *testing.T) {
				ResetOutputsCache()
				t.Cleanup(ResetOutputsCache)
				ctrl := gomock.NewController(t)
				describer := NewMockComponentDescriber(ctrl)
				getter := NewMockStaticRemoteStateGetter(ctrl)
				executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
				config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
				contexts := []*schema.AuthContext{
					{AWS: &schema.AWSAuthContext{Profile: "first"}},
					{AWS: &schema.AWSAuthContext{Profile: "second"}},
				}
				managers := make([]any, 2)
				if source == "manager" {
					contexts = []*schema.AuthContext{nil, nil}
					for i, identity := range []string{"first", "second"} {
						manager := authtypes.NewMockAuthManager(ctrl)
						manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{Identity: identity}).AnyTimes()
						managers[i] = manager
					}
				}
				describer.EXPECT().DescribeComponent(gomock.Any()).Return(map[string]any{}, nil).Times(2)
				gomock.InOrder(
					getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"account": "first"}),
					getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"account": "second"}),
				)
				// A different identity must fetch its own value; revisiting either must hit only its own cache.
				for _, index := range []int{0, 1, 0, 1} {
					var value any
					var err error
					switch method {
					case "all":
						var values map[string]any
						values, err = executor.GetAllOutputs(config, "component", "stack", false, contexts[index], managers[index])
						value = values["account"]
					case "options":
						value, _, err = executor.GetOutputWithOptions(config, "stack", "component", "account", false, contexts[index], managers[index], &OutputOptions{})
					default:
						value, _, err = executor.GetOutput(config, "stack", "component", "account", false, contexts[index], managers[index])
					}
					require.NoError(t, err)
					require.Equal(t, []string{"first", "second"}[index], value)
				}
			})
		}
	}
}

func TestOutputCacheKeyContextIsolation(t *testing.T) {
	first := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "same-name", EndpointURL: "http://first"}}
	copyOfFirst := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "same-name", EndpointURL: "http://first"}}
	second := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "same-name", EndpointURL: "http://second"}}
	key := outputCacheKey("stack", "component", first, nil)
	require.Equal(t, key, outputCacheKey("stack", "component", copyOfFirst, nil))
	require.NotEqual(t, key, outputCacheKey("stack", "component", second, nil))
	require.NotEqual(t, key, outputCacheKey("stack", "component", nil, nil))
	require.NotEqual(t, key, outputCacheKey("other-stack", "component", first, nil))
	require.NotEqual(t, key, outputCacheKey("stack", "other-component", first, nil))
}

func TestOutputCacheKeyManagerIsolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	first := authtypes.NewMockAuthManager(ctrl)
	second := authtypes.NewMockAuthManager(ctrl)
	info := &schema.ConfigAndStacksInfo{Identity: "same-name"}
	first.EXPECT().GetStackInfo().Return(info).AnyTimes()
	second.EXPECT().GetStackInfo().Return(info).AnyTimes()
	key := outputCacheKey("stack", "component", nil, first)
	require.Equal(t, key, outputCacheKey("stack", "component", nil, first))
	require.False(t, key == outputCacheKey("stack", "component", nil, second), "cache keys compare manager identity, not deep structural equality")
	info.Identity = "another-name"
	require.NotEqual(t, key, outputCacheKey("stack", "component", nil, first))
	info.Identity = "same-name"
	info.AuthDisabled = true
	require.NotEqual(t, key, outputCacheKey("stack", "component", nil, first))
	info.AuthDisabled = false
	info.AuthContext = &schema.AuthContext{Azure: &schema.AzureAuthContext{SubscriptionID: "other-account"}}
	require.NotEqual(t, key, outputCacheKey("stack", "component", nil, first))
}

func TestOutputCacheKeyUnencodableContextDoesNotCollide(t *testing.T) {
	context := &schema.AuthContext{GCP: &schema.GCPAuthContext{TokenExpiry: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
	first := outputCacheKey("stack", "component", context, nil)
	require.NotEqual(t, first, outputCacheKey("stack", "component", context, nil))
	require.NotEqual(t, first, outputCacheKey("stack", "component", nil, nil))
	// GetAllOutputs historically accepts opaque manager values. Uncomparable ones
	// must bypass reuse instead of panicking when used as a sync.Map key.
	uncomparable := []string{"opaque"}
	require.NotEqual(t,
		outputCacheKey("stack", "component", nil, uncomparable),
		outputCacheKey("stack", "component", nil, uncomparable))
}
