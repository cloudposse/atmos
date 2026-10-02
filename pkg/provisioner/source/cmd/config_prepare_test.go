package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSourceCommandDestinationPreparation verifies both mutations use the prepared
// destination and stop before any filesystem/provisioner effect when it is rejected.
func TestSourceCommandDestinationPreparation(t *testing.T) {
	for _, verb := range []string{"pull", "delete"} {
		for _, rejected := range []bool{false, true} {
			name := verb + "/normalized"
			if rejected {
				name = verb + "/rejected"
			}
			t.Run(name, func(t *testing.T) {
				viper.Reset()
				t.Cleanup(viper.Reset)
				originalInit, originalDescribe := initCliConfigFunc, describeComponentFunc
				originalMerge, originalCreate, originalProvision := mergeAuthFunc, createAuthFunc, provisionSourceFunc
				t.Cleanup(func() {
					initCliConfigFunc, describeComponentFunc = originalInit, originalDescribe
					mergeAuthFunc, createAuthFunc, provisionSourceFunc = originalMerge, originalCreate, originalProvision
				})
				root := t.TempDir()
				config := schema.AtmosConfiguration{BasePath: root}
				config.Components.CloudFormation.BasePath = "components/cloudformation"
				section := map[string]any{"source": map[string]any{"uri": "https://example.invalid/template.yaml"}, "atmos_component": "instance", "atmos_stack": "dev"}
				initCliConfigFunc = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) { return config, nil }
				describeComponentFunc = func(string, string) (map[string]any, error) { return section, nil }
				if verb == "pull" {
					ctrl := gomock.NewController(t)
					merger, creator := NewMockAuthMerger(ctrl), NewMockAuthCreator(ctrl)
					merger.EXPECT().MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&schema.AuthConfig{}, nil)
					creator.EXPECT().CreateAuthManager(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
					mergeAuthFunc, createAuthFunc = merger.MergeComponentAuth, creator.CreateAuthManager
				}
				destination, err := workdir.BuildPath(root, "aws/cloudformation", "instance", "dev", section)
				require.NoError(t, err)
				shared := filepath.Join(root, "components", "cloudformation", "instance")
				for _, dir := range []string{shared, destination} {
					require.NoError(t, os.MkdirAll(dir, 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(dir, "canary"), []byte("keep"), 0o600))
				}
				called := false
				provisionSourceFunc = func(_ context.Context, params *source.ProvisionParams) error {
					called = true
					got, err := source.DetermineTargetDirectory(params.AtmosConfig, params.ComponentType, params.Component, params.ComponentConfig)
					require.NoError(t, err)
					assert.Equal(t, destination, got)
					return nil
				}
				cfg := &Config{ComponentType: "aws/cloudformation", TypeLabel: "CloudFormation", PrepareComponentConfig: func(_ *schema.AtmosConfiguration, in map[string]any) (map[string]any, error) {
					if rejected {
						return nil, errUtils.ErrInvalidAwsCloudFormationSettings
					}
					out := make(map[string]any, len(in)+1)
					for k, v := range in {
						out[k] = v
					}
					out["provision"] = map[string]any{"workdir": map[string]any{"enabled": true}}
					return out, nil
				}}
				command := PullCommand(cfg)
				if verb == "delete" {
					command = DeleteCommand(cfg)
				}
				command.SetArgs([]string{"instance", "--stack", "dev", "--force"})
				err = command.Execute()
				if rejected {
					require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, verb == "pull" && !rejected, called)
				assert.FileExists(t, filepath.Join(shared, "canary"))
				if verb == "delete" && !rejected {
					assert.NoDirExists(t, destination)
				} else {
					assert.DirExists(t, destination)
				}
				assert.NotContains(t, section, "provision")
			})
		}
	}
}
