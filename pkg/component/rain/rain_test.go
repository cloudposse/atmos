package rain

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "components/rain", config.BasePath)
	assert.Equal(t, "rain", config.Command)
}

func TestComponentProviderListAndValidate(t *testing.T) {
	provider := &ComponentProvider{}
	stackConfig := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.RainComponentType: map[string]any{
				"api": map[string]any{},
				"db":  map[string]any{},
			},
		},
	}

	components, err := provider.ListComponents(t.Context(), "ue1-dev", stackConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"api", "db"}, components)

	require.NoError(t, provider.ValidateComponent(map[string]any{
		"template": "template.yaml",
		"name":     "app-api-dev",
		"params":   map[string]any{"Environment": "dev"},
		"tags":     map[string]any{"ManagedBy": "atmos"},
	}))

	require.Error(t, provider.ValidateComponent(map[string]any{"template": []string{"template.yaml"}}))
	require.Error(t, provider.ValidateComponent(map[string]any{"name": []string{"app-api-dev"}}))
}

func TestBuildArgsDeployUsesTemplateAndName(t *testing.T) {
	componentPath := filepath.Join(t.TempDir(), "components", "rain", "api")
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "deploy",
		ComponentFromArg: "api",
		ComponentSection: map[string]any{
			"template": "template.yaml",
			"name":     "app-api-dev",
			"config":   "rain-config.yaml",
		},
		ComponentSettingsSection: schema.AtmosSectionMapType{
			cfg.RainSectionName: map[string]any{
				"region":    "us-east-1",
				"s3_bucket": "my-rain-artifacts",
			},
		},
	}

	args, cleanup, err := buildArgs(&info, componentPath)
	if cleanup != nil {
		defer cleanup()
	}

	require.NoError(t, err)
	assert.Equal(t, []string{
		"deploy",
		"--config", filepath.Join(componentPath, "rain-config.yaml"),
		"--region", "us-east-1",
		"--s3-bucket", "my-rain-artifacts",
		filepath.Join(componentPath, "template.yaml"),
		"app-api-dev",
	}, args)
}

func TestBuildArgsStackAndTemplateTargetCommands(t *testing.T) {
	componentPath := filepath.Join(t.TempDir(), "components", "rain", "api")

	rmInfo := schema.ConfigAndStacksInfo{
		SubCommand:       "rm",
		ComponentFromArg: "api",
		ComponentSection: map[string]any{
			"name":   "app-api-dev",
			"config": "rain-config.yaml",
		},
	}
	rmArgs, cleanup, err := buildArgs(&rmInfo, componentPath)
	if cleanup != nil {
		defer cleanup()
	}
	require.NoError(t, err)
	assert.Equal(t, []string{"rm", "app-api-dev"}, rmArgs)

	pkgInfo := schema.ConfigAndStacksInfo{
		SubCommand:       "pkg",
		ComponentFromArg: "api",
		ComponentSection: map[string]any{"template": "nested/template.yaml"},
	}
	pkgArgs, cleanup, err := buildArgs(&pkgInfo, componentPath)
	if cleanup != nil {
		defer cleanup()
	}
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg", filepath.Join(componentPath, "nested", "template.yaml")}, pkgArgs)
}

func TestBuildArgsRequiresNameForStackTargetCommands(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "deploy",
		ComponentFromArg: "api",
		ComponentSection: map[string]any{"template": "template.yaml"},
	}

	_, _, err := buildArgs(&info, t.TempDir())

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrRainStackNameMissing))
	assert.Contains(t, err.Error(), "components.rain.api.name is required")
}

func TestBuildArgsDoesNotInterpolateName(t *testing.T) {
	componentPath := t.TempDir()
	name := "{literal-stack-name}"
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "deploy",
		ComponentFromArg: "api",
		ComponentSection: map[string]any{
			"template": "template.yaml",
			"name":     name,
		},
	}

	args, cleanup, err := buildArgs(&info, componentPath)
	if cleanup != nil {
		defer cleanup()
	}

	require.NoError(t, err)
	assert.Equal(t, name, args[len(args)-1])
}
