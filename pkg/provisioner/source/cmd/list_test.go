package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestListCommand tests that ListCommand creates a valid cobra command.
func TestListCommand(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := ListCommand(cfg)

	require.NotNil(t, cmd)
	assert.Equal(t, "list [component]", cmd.Use)
	assert.Contains(t, cmd.Short, "Terraform")
	assert.Contains(t, cmd.Short, "source")
}

// TestListCommand_AcceptsOptionalComponent tests that the list command accepts 0 or 1 args.
func TestListCommand_AcceptsOptionalComponent(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := ListCommand(cfg)

	// Should accept no args.
	err := cmd.Args(cmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")

	// Should accept one arg.
	err = cmd.Args(cmd, []string{"vpc"})
	assert.NoError(t, err, "Should accept one argument")

	// Should reject two args.
	err = cmd.Args(cmd, []string{"vpc", "eks"})
	assert.Error(t, err, "Should reject two arguments")
}

// TestExecuteList_ConfigError tests that executeList handles config init errors.
func TestExecuteList_ConfigError(t *testing.T) {
	// Save original and restore after test.
	origInitConfig := initCliConfigForPrompt
	defer func() { initCliConfigForPrompt = origInitConfig }()

	// Mock config init to fail.
	initCliConfigForPrompt = func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errUtils.Build(errUtils.ErrFailedToInitConfig).
			WithExplanation("mock config error").
			Err()
	}

	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithStringFlag("format", "f", "table", "Output format"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	err = executeList(cmd, []string{}, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

// TestExecuteList_DescribeStacksError tests that executeList handles describe stacks errors.
func TestExecuteList_DescribeStacksError(t *testing.T) {
	// Save originals and restore after test.
	origInitConfig := initCliConfigForPrompt
	origDescribeStacks := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitConfig
		executeDescribeStacksFunc = origDescribeStacks
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to fail.
	executeDescribeStacksFunc = func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
		componentTypes []string,
		sections []string,
		ignoreMissingFiles bool,
		processTemplates bool,
		processYamlFunctions bool,
		includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
	) (map[string]any, error) {
		return nil, errUtils.Build(errUtils.ErrExecuteDescribeStacks).
			WithExplanation("mock describe stacks error").
			Err()
	}

	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithStringFlag("format", "f", "table", "Output format"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	err = executeList(cmd, []string{}, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExecuteDescribeStacks)
}

// TestExtractSourcesFromStack tests extracting components with source from stack data.
func TestExtractSourcesFromStack(t *testing.T) {
	tests := []struct {
		name           string
		stacksMap      map[string]any
		stack          string
		componentType  string
		wantCount      int
		wantComponents []string
	}{
		{
			name: "components with source",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"source": map[string]any{
									"uri":     "github.com/cloudposse/terraform-aws-components//modules/vpc",
									"version": "1.450.0",
								},
							},
							"eks": map[string]any{
								"source": "github.com/cloudposse/terraform-aws-components//modules/eks?ref=1.450.0",
							},
							"no-source": map[string]any{
								"vars": map[string]any{"enabled": true},
							},
						},
					},
				},
			},
			stack:          "dev",
			componentType:  "terraform",
			wantCount:      2,
			wantComponents: []string{"eks", "vpc"},
		},
		{
			name: "no components with source",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"component1": map[string]any{
								"vars": map[string]any{"foo": "bar"},
							},
						},
					},
				},
			},
			stack:          "dev",
			componentType:  "terraform",
			wantCount:      0,
			wantComponents: nil,
		},
		{
			name:           "empty stack",
			stacksMap:      map[string]any{},
			stack:          "dev",
			componentType:  "terraform",
			wantCount:      0,
			wantComponents: nil,
		},
		{
			name: "wrong stack name",
			stacksMap: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"source": "github.com/example/repo//module",
							},
						},
					},
				},
			},
			stack:          "dev",
			componentType:  "terraform",
			wantCount:      0,
			wantComponents: nil,
		},
		{
			name: "wrong component type",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"source": "github.com/example/repo//module",
							},
						},
					},
				},
			},
			stack:          "dev",
			componentType:  "helmfile",
			wantCount:      0,
			wantComponents: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := extractSourcesFromStack(tt.stacksMap, tt.stack, tt.componentType)

			assert.Len(t, sources, tt.wantCount)

			if tt.wantComponents != nil {
				var componentNames []string
				for _, s := range sources {
					componentNames = append(componentNames, s["component"].(string))
				}
				assert.Equal(t, tt.wantComponents, componentNames)
			}
		})
	}
}

// TestExtractSourcesFromStack_WithFolder tests folder extraction from metadata.component.
func TestExtractSourcesFromStack_WithFolder(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc/production": map[string]any{
						"metadata": map[string]any{
							"component": "vpc", // Different from instance name.
						},
						"source": map[string]any{
							"uri":     "github.com/example/vpc",
							"version": "1.0.0",
						},
					},
					"eks": map[string]any{
						// No metadata.component - folder should equal component name.
						"source": map[string]any{
							"uri":     "github.com/example/eks",
							"version": "2.0.0",
						},
					},
				},
			},
		},
	}

	sources := extractSourcesFromStack(stacksMap, "dev", "terraform")

	require.Len(t, sources, 2)

	// Find vpc/production source.
	var vpcSource, eksSource map[string]any
	for _, s := range sources {
		if s["component"] == "vpc/production" {
			vpcSource = s
		}
		if s["component"] == "eks" {
			eksSource = s
		}
	}

	// vpc/production should have folder "vpc".
	require.NotNil(t, vpcSource, "vpc/production source not found")
	assert.Equal(t, "vpc", vpcSource["folder"], "folder should be extracted from metadata.component")
	assert.Equal(t, "dev", vpcSource["stack"], "stack should be set")

	// eks should have folder "eks" (same as component name).
	require.NotNil(t, eksSource, "eks source not found")
	assert.Equal(t, "eks", eksSource["folder"], "folder should default to component name")
}

// TestExtractSourcesFromAllStacks tests multi-stack extraction.
func TestExtractSourcesFromAllStacks(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"source": map[string]any{
							"uri":     "github.com/example/vpc",
							"version": "1.0.0",
						},
					},
				},
			},
		},
		"prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"source": map[string]any{
							"uri":     "github.com/example/vpc",
							"version": "2.0.0",
						},
					},
					"eks": map[string]any{
						"source": map[string]any{
							"uri":     "github.com/example/eks",
							"version": "3.0.0",
						},
					},
				},
			},
		},
	}

	sources := extractSourcesFromAllStacks(stacksMap, "terraform")

	// Should have 3 sources total.
	require.Len(t, sources, 3)

	// Verify stack field is present on all.
	for _, s := range sources {
		assert.Contains(t, s, "stack", "stack field should be present")
		assert.Contains(t, s, "component", "component field should be present")
		assert.Contains(t, s, "folder", "folder field should be present")
	}

	// Verify sorting by stack, then component.
	assert.Equal(t, "dev", sources[0]["stack"], "first should be dev stack")
	assert.Equal(t, "prod", sources[1]["stack"], "second should be prod stack")
	assert.Equal(t, "prod", sources[2]["stack"], "third should be prod stack")
	assert.Equal(t, "eks", sources[1]["component"], "eks should come before vpc in prod")
	assert.Equal(t, "vpc", sources[2]["component"], "vpc should come after eks in prod")
}

// TestFilterByComponent tests filtering sources by component name or folder.
func TestFilterByComponent(t *testing.T) {
	sources := []map[string]any{
		{"component": "vpc/production", "folder": "vpc"},
		{"component": "eks", "folder": "eks"},
		{"component": "vpc/staging", "folder": "vpc"},
		{"component": "rds", "folder": "rds"},
	}

	// Filter by folder name.
	filtered := filterByComponent(sources, "vpc")
	assert.Len(t, filtered, 2, "should match 2 sources with folder 'vpc'")

	// Filter by component name.
	filtered = filterByComponent(sources, "eks")
	assert.Len(t, filtered, 1, "should match 1 source with component 'eks'")

	// Filter by exact component name.
	filtered = filterByComponent(sources, "vpc/production")
	assert.Len(t, filtered, 1, "should match 1 source with exact component name")

	// No match.
	filtered = filterByComponent(sources, "nonexistent")
	assert.Len(t, filtered, 0, "should match 0 sources")

	// Empty filter returns all.
	filtered = filterByComponent(sources, "")
	assert.Len(t, filtered, 4, "empty filter should return all sources")
}

// TestGetSourceListColumnsForContext tests dynamic column configuration.
func TestGetSourceListColumnsForContext(t *testing.T) {
	tests := []struct {
		name          string
		hasStack      bool
		hasFolderDiff bool
		wantColumns   []string
	}{
		{
			name:          "all stacks with folder diff",
			hasStack:      false,
			hasFolderDiff: true,
			wantColumns:   []string{"Stack", "Component", "Folder", "URI", "Version"},
		},
		{
			name:          "all stacks without folder diff",
			hasStack:      false,
			hasFolderDiff: false,
			wantColumns:   []string{"Stack", "Component", "URI", "Version"},
		},
		{
			name:          "single stack with folder diff",
			hasStack:      true,
			hasFolderDiff: true,
			wantColumns:   []string{"Component", "Folder", "URI", "Version"},
		},
		{
			name:          "single stack without folder diff",
			hasStack:      true,
			hasFolderDiff: false,
			wantColumns:   []string{"Component", "URI", "Version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := getSourceListColumnsForContext(tt.hasStack, tt.hasFolderDiff)
			var names []string
			for _, c := range columns {
				names = append(names, c.Name)
			}
			assert.Equal(t, tt.wantColumns, names)
		})
	}
}

// TestCheckFolderDiffers tests folder difference detection.
func TestCheckFolderDiffers(t *testing.T) {
	// No difference.
	sources := []map[string]any{
		{"component": "vpc", "folder": "vpc"},
		{"component": "eks", "folder": "eks"},
	}
	assert.False(t, checkFolderDiffers(sources), "should return false when all folders match components")

	// Has difference.
	sources = []map[string]any{
		{"component": "vpc/production", "folder": "vpc"},
		{"component": "eks", "folder": "eks"},
	}
	assert.True(t, checkFolderDiffers(sources), "should return true when any folder differs from component")

	// Empty sources.
	sources = []map[string]any{}
	assert.False(t, checkFolderDiffers(sources), "should return false for empty sources")
}

// TestExtractComponentFolder tests folder extraction helper.
func TestExtractComponentFolder(t *testing.T) {
	// With metadata.component.
	componentMap := map[string]any{
		"metadata": map[string]any{
			"component": "base-vpc",
		},
	}
	folder := extractComponentFolder(componentMap, "vpc/instance")
	assert.Equal(t, "base-vpc", folder, "should extract folder from metadata.component")

	// Without metadata.component.
	componentMap = map[string]any{
		"vars": map[string]any{"enabled": true},
	}
	folder = extractComponentFolder(componentMap, "vpc")
	assert.Equal(t, "vpc", folder, "should default to component name")

	// With empty metadata.component.
	componentMap = map[string]any{
		"metadata": map[string]any{
			"component": "",
		},
	}
	folder = extractComponentFolder(componentMap, "vpc")
	assert.Equal(t, "vpc", folder, "should default to component name when metadata.component is empty")

	// With metadata but no component field.
	componentMap = map[string]any{
		"metadata": map[string]any{
			"type": "real",
		},
	}
	folder = extractComponentFolder(componentMap, "vpc")
	assert.Equal(t, "vpc", folder, "should default to component name when metadata.component not set")
}
