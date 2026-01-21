package list

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestSourcesCommand tests that the sources command has the correct structure.
func TestSourcesCommand(t *testing.T) {
	assert.Equal(t, "sources [component]", sourcesCmd.Use)
	assert.Contains(t, sourcesCmd.Short, "List components with source configuration")
	assert.NotNil(t, sourcesCmd.RunE)
	assert.NotEmpty(t, sourcesCmd.Example)
}

// TestSourcesCommand_AcceptsOptionalComponent tests that the sources command accepts 0 or 1 args.
func TestSourcesCommand_AcceptsOptionalComponent(t *testing.T) {
	// Should accept no args.
	err := sourcesCmd.Args(sourcesCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")

	// Should accept one arg.
	err = sourcesCmd.Args(sourcesCmd, []string{"vpc"})
	assert.NoError(t, err, "Should accept one argument")

	// Should reject two args.
	err = sourcesCmd.Args(sourcesCmd, []string{"vpc", "eks"})
	assert.Error(t, err, "Should reject two arguments")
}

// TestSourcesCommandFlags tests that the sources command has the expected flags.
func TestSourcesCommandFlags(t *testing.T) {
	// Check flags on the actual command.
	formatFlag := sourcesCmd.Flags().Lookup("format")
	require.NotNil(t, formatFlag, "Expected format flag to exist")
	assert.Equal(t, "", formatFlag.DefValue)

	// Check stack flag exists.
	stackFlag := sourcesCmd.Flags().Lookup("stack")
	require.NotNil(t, stackFlag, "Expected stack flag to exist")
	assert.Equal(t, "", stackFlag.DefValue)
}

// TestSourcesOptions tests the SourcesOptions structure.
func TestSourcesOptions(t *testing.T) {
	opts := &SourcesOptions{
		Format:    "json",
		Stack:     "dev",
		Component: "vpc",
	}

	assert.Equal(t, "json", opts.Format)
	assert.Equal(t, "dev", opts.Stack)
	assert.Equal(t, "vpc", opts.Component)
}

// TestGetSourcesListColumnsForContext tests dynamic column configuration.
func TestGetSourcesListColumnsForContext(t *testing.T) {
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
			wantColumns:   []string{"Stack", "Type", "Component", "Folder", "URI", "Version"},
		},
		{
			name:          "all stacks without folder diff",
			hasStack:      false,
			hasFolderDiff: false,
			wantColumns:   []string{"Stack", "Type", "Component", "URI", "Version"},
		},
		{
			name:          "single stack with folder diff",
			hasStack:      true,
			hasFolderDiff: true,
			wantColumns:   []string{"Type", "Component", "Folder", "URI", "Version"},
		},
		{
			name:          "single stack without folder diff",
			hasStack:      true,
			hasFolderDiff: false,
			wantColumns:   []string{"Type", "Component", "URI", "Version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := getSourcesListColumnsForContext(tt.hasStack, tt.hasFolderDiff)
			var names []string
			for _, c := range columns {
				names = append(names, c.Name)
			}
			assert.Equal(t, tt.wantColumns, names)
		})
	}
}

// TestCheckSourcesFolderDiffers tests folder difference detection.
func TestCheckSourcesFolderDiffers(t *testing.T) {
	// No difference.
	sources := []map[string]any{
		{"component": "vpc", "folder": "vpc"},
		{"component": "eks", "folder": "eks"},
	}
	assert.False(t, checkSourcesFolderDiffers(sources), "should return false when all folders match components")

	// Has difference.
	sources = []map[string]any{
		{"component": "vpc/production", "folder": "vpc"},
		{"component": "eks", "folder": "eks"},
	}
	assert.True(t, checkSourcesFolderDiffers(sources), "should return true when any folder differs from component")

	// Empty sources.
	sources = []map[string]any{}
	assert.False(t, checkSourcesFolderDiffers(sources), "should return false for empty sources")
}

// TestFilterSourcesByComponent tests filtering sources by component name or folder.
func TestFilterSourcesByComponent(t *testing.T) {
	sources := []map[string]any{
		{"component": "vpc/production", "folder": "vpc", "type": "terraform"},
		{"component": "eks", "folder": "eks", "type": "terraform"},
		{"component": "vpc/staging", "folder": "vpc", "type": "terraform"},
		{"component": "nginx", "folder": "nginx", "type": "helmfile"},
	}

	// Filter by folder name.
	filtered := filterSourcesByComponent(sources, "vpc")
	assert.Len(t, filtered, 2, "should match 2 sources with folder 'vpc'")

	// Filter by component name.
	filtered = filterSourcesByComponent(sources, "eks")
	assert.Len(t, filtered, 1, "should match 1 source with component 'eks'")

	// Filter by exact component name.
	filtered = filterSourcesByComponent(sources, "vpc/production")
	assert.Len(t, filtered, 1, "should match 1 source with exact component name")

	// No match.
	filtered = filterSourcesByComponent(sources, "nonexistent")
	assert.Len(t, filtered, 0, "should match 0 sources")

	// Empty filter returns all.
	filtered = filterSourcesByComponent(sources, "")
	assert.Len(t, filtered, 4, "empty filter should return all sources")
}

// TestExtractSourceComponentFolder tests folder extraction helper.
func TestExtractSourceComponentFolder(t *testing.T) {
	// With metadata.component.
	componentMap := map[string]any{
		"metadata": map[string]any{
			"component": "base-vpc",
		},
	}
	folder := extractSourceComponentFolder(componentMap, "vpc/instance")
	assert.Equal(t, "base-vpc", folder, "should extract folder from metadata.component")

	// Without metadata.component.
	componentMap = map[string]any{
		"vars": map[string]any{"enabled": true},
	}
	folder = extractSourceComponentFolder(componentMap, "vpc")
	assert.Equal(t, "vpc", folder, "should default to component name")

	// With empty metadata.component.
	componentMap = map[string]any{
		"metadata": map[string]any{
			"component": "",
		},
	}
	folder = extractSourceComponentFolder(componentMap, "vpc")
	assert.Equal(t, "vpc", folder, "should default to component name when metadata.component is empty")

	// With metadata but no component field.
	componentMap = map[string]any{
		"metadata": map[string]any{
			"type": "real",
		},
	}
	folder = extractSourceComponentFolder(componentMap, "vpc")
	assert.Equal(t, "vpc", folder, "should default to component name when metadata.component not set")
}

// TestExtractAllSourcesFromStack tests extracting components with source from all component types.
func TestExtractAllSourcesFromStack(t *testing.T) {
	tests := []struct {
		name           string
		stacksMap      map[string]any
		stack          string
		wantCount      int
		wantComponents []string
		wantTypes      []string
	}{
		{
			name: "terraform and helmfile components with source",
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
						},
						"helmfile": map[string]any{
							"nginx": map[string]any{
								"source": "github.com/cloudposse/helmfile-charts//nginx?ref=1.0.0",
							},
						},
					},
				},
			},
			stack:          "dev",
			wantCount:      2,
			wantComponents: []string{"nginx", "vpc"},
			wantTypes:      []string{"helmfile", "terraform"},
		},
		{
			name: "only terraform components with source",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"source": map[string]any{
									"uri":     "github.com/example/module",
									"version": "1.0.0",
								},
							},
							"eks": map[string]any{
								"source": "github.com/example/eks?ref=2.0.0",
							},
						},
						"helmfile": map[string]any{
							"no-source": map[string]any{
								"vars": map[string]any{"enabled": true},
							},
						},
					},
				},
			},
			stack:          "dev",
			wantCount:      2,
			wantComponents: []string{"eks", "vpc"},
			wantTypes:      []string{"terraform", "terraform"},
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
			wantCount:      0,
			wantComponents: nil,
			wantTypes:      nil,
		},
		{
			name:           "empty stack",
			stacksMap:      map[string]any{},
			stack:          "dev",
			wantCount:      0,
			wantComponents: nil,
			wantTypes:      nil,
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
			wantCount:      0,
			wantComponents: nil,
			wantTypes:      nil,
		},
		{
			name: "all component types with source",
			stacksMap: map[string]any{
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
						"helmfile": map[string]any{
							"nginx": map[string]any{
								"source": map[string]any{
									"uri":     "github.com/example/nginx",
									"version": "2.0.0",
								},
							},
						},
						"packer": map[string]any{
							"ami": map[string]any{
								"source": map[string]any{
									"uri":     "github.com/example/ami",
									"version": "3.0.0",
								},
							},
						},
					},
				},
			},
			stack:          "dev",
			wantCount:      3,
			wantComponents: []string{"nginx", "ami", "vpc"},
			wantTypes:      []string{"helmfile", "packer", "terraform"},
		},
		{
			name: "stack with no components key",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"vars": map[string]any{"region": "us-east-1"},
				},
			},
			stack:          "dev",
			wantCount:      0,
			wantComponents: nil,
			wantTypes:      nil,
		},
		{
			name: "stack data is not a map",
			stacksMap: map[string]any{
				"dev": "invalid-data",
			},
			stack:          "dev",
			wantCount:      0,
			wantComponents: nil,
			wantTypes:      nil,
		},
		{
			name: "component data is not a map",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": "invalid-component-data",
						},
					},
				},
			},
			stack:          "dev",
			wantCount:      0,
			wantComponents: nil,
			wantTypes:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := extractAllSourcesFromStack(tt.stacksMap, tt.stack)

			assert.Len(t, sources, tt.wantCount)

			if tt.wantComponents != nil {
				var componentNames []string
				var componentTypes []string
				for _, s := range sources {
					componentNames = append(componentNames, s["component"].(string))
					componentTypes = append(componentTypes, s["type"].(string))
				}
				assert.Equal(t, tt.wantComponents, componentNames)
				assert.Equal(t, tt.wantTypes, componentTypes)
			}
		})
	}
}

// TestExtractAllSourcesFromStack_WithFolder tests folder extraction from metadata.component.
func TestExtractAllSourcesFromStack_WithFolder(t *testing.T) {
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

	sources := extractAllSourcesFromStack(stacksMap, "dev")

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

// TestExtractAllSourcesFromAllStacks tests multi-stack extraction.
func TestExtractAllSourcesFromAllStacks(t *testing.T) {
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
				"helmfile": map[string]any{
					"nginx": map[string]any{
						"source": map[string]any{
							"uri":     "github.com/example/nginx",
							"version": "4.0.0",
						},
					},
				},
			},
		},
	}

	sources := extractAllSourcesFromAllStacks(stacksMap)

	// Should have 4 sources total.
	require.Len(t, sources, 4)

	// Verify stack field is present on all.
	for _, s := range sources {
		assert.Contains(t, s, "stack", "stack field should be present")
		assert.Contains(t, s, "component", "component field should be present")
		assert.Contains(t, s, "folder", "folder field should be present")
		assert.Contains(t, s, "type", "type field should be present")
	}

	// Verify sorting by stack, type, then component.
	assert.Equal(t, "dev", sources[0]["stack"], "first should be dev stack")
	assert.Equal(t, "prod", sources[1]["stack"], "second should be prod stack")
	assert.Equal(t, "prod", sources[2]["stack"], "third should be prod stack")
	assert.Equal(t, "prod", sources[3]["stack"], "fourth should be prod stack")

	// Within prod, should be sorted by type then component.
	assert.Equal(t, "helmfile", sources[1]["type"], "first prod should be helmfile")
	assert.Equal(t, "terraform", sources[2]["type"], "second prod should be terraform")
	assert.Equal(t, "terraform", sources[3]["type"], "third prod should be terraform")
	assert.Equal(t, "eks", sources[2]["component"], "eks should come before vpc in terraform")
	assert.Equal(t, "vpc", sources[3]["component"], "vpc should come after eks in terraform")
}

// TestExtractAllSourcesFromStack_Sorting tests that the extracted sources are sorted correctly.
func TestExtractAllSourcesFromStack_Sorting(t *testing.T) {
	// Create stack data with components in non-alphabetical order.
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"zulu": map[string]any{
						"source": map[string]any{"uri": "github.com/example/zulu", "version": "1.0.0"},
					},
					"alpha": map[string]any{
						"source": map[string]any{"uri": "github.com/example/alpha", "version": "1.0.0"},
					},
				},
				"helmfile": map[string]any{
					"mike": map[string]any{
						"source": map[string]any{"uri": "github.com/example/mike", "version": "1.0.0"},
					},
					"bravo": map[string]any{
						"source": map[string]any{"uri": "github.com/example/bravo", "version": "1.0.0"},
					},
				},
			},
		},
	}

	sources := extractAllSourcesFromStack(stacksMap, "dev")

	require.Len(t, sources, 4)

	// Results should be sorted by type first, then component name.
	expectedOrder := []struct {
		componentType string
		component     string
	}{
		{"helmfile", "bravo"},
		{"helmfile", "mike"},
		{"terraform", "alpha"},
		{"terraform", "zulu"},
	}

	for i, expected := range expectedOrder {
		assert.Equal(t, expected.componentType, sources[i]["type"], "Type mismatch at index %d", i)
		assert.Equal(t, expected.component, sources[i]["component"], "Component mismatch at index %d", i)
	}
}

// TestExtractAllSourcesFromStack_ExtractedFields tests that all expected fields are extracted.
func TestExtractAllSourcesFromStack_ExtractedFields(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"source": map[string]any{
							"uri":     "github.com/cloudposse/terraform-aws-components//modules/vpc",
							"version": "1.450.0",
						},
					},
				},
			},
		},
	}

	sources := extractAllSourcesFromStack(stacksMap, "dev")

	require.Len(t, sources, 1)

	source := sources[0]
	assert.Equal(t, "dev", source["stack"])
	assert.Equal(t, "terraform", source["type"])
	assert.Equal(t, "vpc", source["component"])
	assert.Equal(t, "vpc", source["folder"])
	assert.Equal(t, "github.com/cloudposse/terraform-aws-components//modules/vpc", source["uri"])
	assert.Equal(t, "1.450.0", source["version"])
}

// TestBuildSourcesSorters tests the sorter building function.
func TestBuildSourcesSorters(t *testing.T) {
	// With stack column.
	sorters := buildSourcesSorters(true)
	require.Len(t, sorters, 3)
	assert.Equal(t, "Stack", sorters[0].Column)
	assert.Equal(t, "Type", sorters[1].Column)
	assert.Equal(t, "Component", sorters[2].Column)

	// Without stack column.
	sorters = buildSourcesSorters(false)
	require.Len(t, sorters, 2)
	assert.Equal(t, "Type", sorters[0].Column)
	assert.Equal(t, "Component", sorters[1].Column)
}

// TestSortSourcesByTypeComponent tests sorting by type and component.
func TestSortSourcesByTypeComponent(t *testing.T) {
	sources := []map[string]any{
		{"type": "terraform", "component": "zulu"},
		{"type": "helmfile", "component": "bravo"},
		{"type": "terraform", "component": "alpha"},
		{"type": "helmfile", "component": "alpha"},
	}

	sortSourcesByTypeComponent(sources)

	// Should be sorted by type first, then component.
	assert.Equal(t, "helmfile", sources[0]["type"])
	assert.Equal(t, "alpha", sources[0]["component"])
	assert.Equal(t, "helmfile", sources[1]["type"])
	assert.Equal(t, "bravo", sources[1]["component"])
	assert.Equal(t, "terraform", sources[2]["type"])
	assert.Equal(t, "alpha", sources[2]["component"])
	assert.Equal(t, "terraform", sources[3]["type"])
	assert.Equal(t, "zulu", sources[3]["component"])
}

// TestSortSourcesByStackTypeComponent tests sorting by stack, type, and component.
func TestSortSourcesByStackTypeComponent(t *testing.T) {
	sources := []map[string]any{
		{"stack": "prod", "type": "terraform", "component": "vpc"},
		{"stack": "dev", "type": "helmfile", "component": "nginx"},
		{"stack": "dev", "type": "terraform", "component": "eks"},
		{"stack": "prod", "type": "helmfile", "component": "app"},
	}

	sortSourcesByStackTypeComponent(sources)

	// Should be sorted by stack, then type, then component.
	assert.Equal(t, "dev", sources[0]["stack"])
	assert.Equal(t, "helmfile", sources[0]["type"])
	assert.Equal(t, "nginx", sources[0]["component"])

	assert.Equal(t, "dev", sources[1]["stack"])
	assert.Equal(t, "terraform", sources[1]["type"])
	assert.Equal(t, "eks", sources[1]["component"])

	assert.Equal(t, "prod", sources[2]["stack"])
	assert.Equal(t, "helmfile", sources[2]["type"])
	assert.Equal(t, "app", sources[2]["component"])

	assert.Equal(t, "prod", sources[3]["stack"])
	assert.Equal(t, "terraform", sources[3]["type"])
	assert.Equal(t, "vpc", sources[3]["component"])
}

// TestExtractSourcesFromComponentType tests extraction from a specific component type.
func TestExtractSourcesFromComponentType(t *testing.T) {
	components := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"source": map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
			},
			"no-source": map[string]any{
				"vars": map[string]any{"enabled": true},
			},
		},
		"helmfile": map[string]any{
			"nginx": map[string]any{
				"source": map[string]any{"uri": "github.com/example/nginx", "version": "2.0.0"},
			},
		},
	}

	// Extract terraform components.
	sources := extractSourcesFromComponentType("dev", "terraform", components)
	require.Len(t, sources, 1)
	assert.Equal(t, "vpc", sources[0]["component"])

	// Extract helmfile components.
	sources = extractSourcesFromComponentType("dev", "helmfile", components)
	require.Len(t, sources, 1)
	assert.Equal(t, "nginx", sources[0]["component"])

	// Extract non-existent type.
	sources = extractSourcesFromComponentType("dev", "packer", components)
	assert.Len(t, sources, 0)
}

// TestExtractSourceEntry tests extraction of a single source entry.
func TestExtractSourceEntry(t *testing.T) {
	// Valid source entry.
	componentData := map[string]any{
		"source": map[string]any{
			"uri":     "github.com/example/vpc",
			"version": "1.0.0",
		},
		"metadata": map[string]any{
			"component": "base-vpc",
		},
	}

	entry := extractSourceEntry("dev", "terraform", "vpc/production", componentData)
	require.NotNil(t, entry)
	assert.Equal(t, "dev", entry["stack"])
	assert.Equal(t, "terraform", entry["type"])
	assert.Equal(t, "vpc/production", entry["component"])
	assert.Equal(t, "base-vpc", entry["folder"])
	assert.Equal(t, "github.com/example/vpc", entry["uri"])
	assert.Equal(t, "1.0.0", entry["version"])

	// Component data is not a map.
	entry = extractSourceEntry("dev", "terraform", "vpc", "invalid")
	assert.Nil(t, entry)

	// No source configured.
	componentData = map[string]any{
		"vars": map[string]any{"enabled": true},
	}
	entry = extractSourceEntry("dev", "terraform", "vpc", componentData)
	assert.Nil(t, entry)

	// Invalid source (nil source spec from ExtractSource).
	componentData = map[string]any{
		"source": 12345, // Invalid type.
	}
	entry = extractSourceEntry("dev", "terraform", "vpc", componentData)
	assert.Nil(t, entry)
}

// TestExtractSourcesFromStackData tests extraction from a single stack's data.
func TestExtractSourcesFromStackData(t *testing.T) {
	// Valid stack data with multiple component types.
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"source": map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
				},
			},
			"helmfile": map[string]any{
				"nginx": map[string]any{
					"source": map[string]any{"uri": "github.com/example/nginx", "version": "2.0.0"},
				},
			},
			"packer": map[string]any{
				"ami": map[string]any{
					"source": map[string]any{"uri": "github.com/example/ami", "version": "3.0.0"},
				},
			},
		},
	}

	sources := extractSourcesFromStackData("dev", stackData)
	require.Len(t, sources, 3)

	// Stack data is not a map.
	sources = extractSourcesFromStackData("dev", "invalid")
	assert.Len(t, sources, 0)

	// Stack data has no components key.
	stackData = map[string]any{
		"vars": map[string]any{"region": "us-east-1"},
	}
	sources = extractSourcesFromStackData("dev", stackData)
	assert.Len(t, sources, 0)

	// Components is not a map.
	stackData = map[string]any{
		"components": "invalid",
	}
	sources = extractSourcesFromStackData("dev", stackData)
	assert.Len(t, sources, 0)
}

// TestWrapSourcesConfigError tests error wrapping for configuration errors.
func TestWrapSourcesConfigError(t *testing.T) {
	tests := []struct {
		name        string
		errMsg      string
		stack       string
		wantErrType error
	}{
		{
			name:        "import failure error",
			errMsg:      "failed to find import: some-stack.yaml",
			stack:       "",
			wantErrType: errUtils.ErrNoStacksFound,
		},
		{
			name:        "no files match error",
			errMsg:      "no files match the pattern",
			stack:       "dev",
			wantErrType: errUtils.ErrNoStacksFound,
		},
		{
			name:        "stacks directory does not exist",
			errMsg:      "stacks directory does not exist: /path/to/stacks",
			stack:       "",
			wantErrType: errUtils.ErrMissingAtmosConfig,
		},
		{
			name:        "atmos.yaml error",
			errMsg:      "cannot find atmos.yaml in any of the paths",
			stack:       "",
			wantErrType: errUtils.ErrMissingAtmosConfig,
		},
		{
			name:        "generic config error without stack",
			errMsg:      "some other configuration error",
			stack:       "",
			wantErrType: errUtils.ErrFailedToInitConfig,
		},
		{
			name:        "generic config error with stack",
			errMsg:      "some other configuration error",
			stack:       "dev",
			wantErrType: errUtils.ErrFailedToInitConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an error with the message in the error itself.
			inputErr := errors.New(tt.errMsg)

			wrappedErr := wrapSourcesConfigError(inputErr, tt.stack)

			require.Error(t, wrappedErr)
			assert.ErrorIs(t, wrappedErr, tt.wantErrType)
		})
	}
}
