package output

import (
	"testing"

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
