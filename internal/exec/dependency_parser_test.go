package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
)

func TestNewDependencyParser(t *testing.T) {
	builder := dependency.NewBuilder()
	nodeMap := map[string]string{
		"vpc-dev": "vpc-dev",
	}

	parser := NewDependencyParser(builder, nodeMap)

	assert.NotNil(t, parser)
	assert.NotNil(t, parser.builder)
	assert.NotNil(t, parser.nodeMap)
	assert.Equal(t, 1, len(parser.nodeMap))
}

func TestDependencyParser_ShouldSkipComponent(t *testing.T) {
	parser := &DependencyParser{}

	tests := []struct {
		name       string
		component  map[string]any
		shouldSkip bool
	}{
		{
			name: "abstract component",
			component: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"type": "abstract",
				},
			},
			shouldSkip: true,
		},
		{
			name: "disabled component",
			component: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"enabled": false,
				},
			},
			shouldSkip: true,
		},
		{
			name: "enabled component",
			component: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"enabled": true,
				},
			},
			shouldSkip: false,
		},
		{
			name: "no metadata",
			component: map[string]any{
				"vars": map[string]any{},
			},
			shouldSkip: false,
		},
		{
			name: "non-abstract component",
			component: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"type": "real",
				},
			},
			shouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.shouldSkipComponent(tt.component)
			assert.Equal(t, tt.shouldSkip, result)
		})
	}
}

func TestDependencyParser_ParseComponentDependencies(t *testing.T) {
	tests := []struct {
		name          string
		stackName     string
		componentName string
		component     map[string]any
		nodeMap       map[string]string
		expectDeps    []string
		expectError   bool
	}{
		{
			name:          "no settings section",
			stackName:     "dev",
			componentName: "vpc",
			component: map[string]any{
				"vars": map[string]any{},
			},
			nodeMap:     map[string]string{},
			expectDeps:  []string{},
			expectError: false,
		},
		{
			name:          "no depends_on field",
			stackName:     "dev",
			componentName: "vpc",
			component: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"other": "value",
				},
			},
			nodeMap:     map[string]string{},
			expectDeps:  []string{},
			expectError: false,
		},
		{
			name:          "abstract component skipped",
			stackName:     "dev",
			componentName: "base",
			component: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"type": "abstract",
				},
				cfg.SettingsSectionName: map[string]any{
					"depends_on": []any{
						map[string]any{"component": "vpc"},
					},
				},
			},
			nodeMap:     map[string]string{"vpc-dev": "vpc-dev"},
			expectDeps:  []string{},
			expectError: false,
		},
		{
			name:          "array format dependencies",
			stackName:     "dev",
			componentName: "app",
			component: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"depends_on": []any{
						map[string]any{"component": "vpc"},
						map[string]any{"component": "database"},
					},
				},
			},
			nodeMap: map[string]string{
				"vpc-dev":      "vpc-dev",
				"database-dev": "database-dev",
				"app-dev":      "app-dev",
			},
			expectDeps:  []string{"vpc-dev", "database-dev"},
			expectError: false,
		},
		{
			name:          "map format dependencies",
			stackName:     "dev",
			componentName: "app",
			component: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"depends_on": map[string]any{
						"dep1": map[string]any{"component": "vpc"},
						"dep2": map[string]any{"component": "database"},
					},
				},
			},
			nodeMap: map[string]string{
				"vpc-dev":      "vpc-dev",
				"database-dev": "database-dev",
				"app-dev":      "app-dev",
			},
			expectDeps:  []string{"vpc-dev", "database-dev"},
			expectError: false,
		},
		{
			name:          "cross-stack dependency",
			stackName:     "dev",
			componentName: "app",
			component: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"depends_on": []any{
						map[string]any{
							"component": "vpc",
							"stack":     "prod",
						},
					},
				},
			},
			nodeMap: map[string]string{
				"vpc-prod": "vpc-prod",
				"app-dev":  "app-dev",
			},
			expectDeps:  []string{"vpc-prod"},
			expectError: false,
		},
		{
			name:          "dependency target not found",
			stackName:     "dev",
			componentName: "app",
			component: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"depends_on": []any{
						map[string]any{"component": "missing"},
					},
				},
			},
			nodeMap: map[string]string{
				"app-dev": "app-dev",
			},
			expectDeps:  []string{},
			expectError: false, // Errors are logged but not returned
		},
		{
			name:          "map[any]any format",
			stackName:     "dev",
			componentName: "app",
			component: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"depends_on": map[any]any{
						"dep1": map[any]any{"component": "vpc"},
					},
				},
			},
			nodeMap: map[string]string{
				"vpc-dev": "vpc-dev",
				"app-dev": "app-dev",
			},
			expectDeps:  []string{"vpc-dev"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := dependency.NewBuilder()

			// Add nodes to builder
			for nodeID := range tt.nodeMap {
				_ = builder.AddNode(&dependency.Node{
					ID: nodeID,
				})
			}

			parser := NewDependencyParser(builder, tt.nodeMap)

			err := parser.ParseComponentDependencies(tt.stackName, tt.componentName, tt.component)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Build graph and check dependencies
			graph, _ := builder.Build()
			fromID := tt.componentName + "-" + tt.stackName
			if node, exists := graph.GetNode(fromID); exists {
				assert.Equal(t, len(tt.expectDeps), len(node.Dependencies))
				for _, expectedDep := range tt.expectDeps {
					assert.Contains(t, node.Dependencies, expectedDep)
				}
			}
		})
	}
}

func TestDependencyParser_ParseSingleDependency(t *testing.T) {
	tests := []struct {
		name        string
		dep         any
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid map[string]any",
			dep: map[string]any{
				"component": "vpc",
			},
			expectError: false,
		},
		{
			name: "valid map[any]any",
			dep: map[any]any{
				"component": "vpc",
			},
			expectError: false,
		},
		{
			name:        "string dependency not found",
			dep:         "invalid",
			expectError: true,
			errorMsg:    "dependency target not found",
		},
		{
			name:        "valid string dependency",
			dep:         "vpc",
			expectError: false,
		},
		{
			name:        "unsupported type int",
			dep:         123,
			expectError: true,
			errorMsg:    "unsupported dependency type",
		},
		{
			name:        "unsupported type array",
			dep:         []string{"invalid"},
			expectError: true,
			errorMsg:    "unsupported dependency type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeMap := map[string]string{
				"vpc-dev": "vpc-dev",
				"app-dev": "app-dev",
			}
			builder := dependency.NewBuilder()

			// Add nodes to builder
			for nodeID := range nodeMap {
				_ = builder.AddNode(&dependency.Node{
					ID: nodeID,
				})
			}

			parser := NewDependencyParser(builder, nodeMap)

			err := parser.parseSingleDependency("app-dev", "dev", tt.dep)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDependencyParser_ParseDependencyMapEntry(t *testing.T) {
	tests := []struct {
		name         string
		depMap       map[string]any
		defaultStack string
		nodeMap      map[string]string
		expectToID   string
		expectError  bool
		errorMsg     string
	}{
		{
			name: "valid component same stack",
			depMap: map[string]any{
				"component": "vpc",
			},
			defaultStack: "dev",
			nodeMap: map[string]string{
				"vpc-dev": "vpc-dev",
			},
			expectToID:  "vpc-dev",
			expectError: false,
		},
		{
			name: "valid component different stack",
			depMap: map[string]any{
				"component": "vpc",
				"stack":     "prod",
			},
			defaultStack: "dev",
			nodeMap: map[string]string{
				"vpc-prod": "vpc-prod",
			},
			expectToID:  "vpc-prod",
			expectError: false,
		},
		{
			name: "missing component field",
			depMap: map[string]any{
				"stack": "prod",
			},
			defaultStack: "dev",
			nodeMap:      map[string]string{},
			expectToID:   "",
			expectError:  true,
			errorMsg:     "missing required field",
		},
		{
			name: "component not a string",
			depMap: map[string]any{
				"component": 123,
			},
			defaultStack: "dev",
			nodeMap:      map[string]string{},
			expectToID:   "",
			expectError:  true,
			errorMsg:     "missing required field",
		},
		{
			name: "target not found",
			depMap: map[string]any{
				"component": "missing",
			},
			defaultStack: "dev",
			nodeMap:      map[string]string{},
			expectToID:   "",
			expectError:  true,
			errorMsg:     "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := dependency.NewBuilder()

			// Add source node
			_ = builder.AddNode(&dependency.Node{
				ID: "app-dev",
			})

			// Add nodes from nodeMap
			for nodeID := range tt.nodeMap {
				if nodeID != "app-dev" {
					_ = builder.AddNode(&dependency.Node{
						ID: nodeID,
					})
				}
			}

			parser := NewDependencyParser(builder, tt.nodeMap)

			err := parser.parseDependencyMapEntry("app-dev", tt.defaultStack, tt.depMap)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDependencyParser_AddDependencyIfExists(t *testing.T) {
	tests := []struct {
		name        string
		fromID      string
		toID        string
		nodeMap     map[string]string
		expectError bool
	}{
		{
			name:   "valid dependency",
			fromID: "app-dev",
			toID:   "vpc-dev",
			nodeMap: map[string]string{
				"vpc-dev": "vpc-dev",
				"app-dev": "app-dev",
			},
			expectError: false,
		},
		{
			name:   "target not exists",
			fromID: "app-dev",
			toID:   "missing-dev",
			nodeMap: map[string]string{
				"app-dev": "app-dev",
			},
			expectError: true,
		},
		{
			name:   "self dependency not allowed",
			fromID: "app-dev",
			toID:   "app-dev",
			nodeMap: map[string]string{
				"app-dev": "app-dev",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := dependency.NewBuilder()

			// Add nodes to builder
			for nodeID := range tt.nodeMap {
				_ = builder.AddNode(&dependency.Node{
					ID: nodeID,
				})
			}

			parser := NewDependencyParser(builder, tt.nodeMap)

			err := parser.addDependencyIfExists(tt.fromID, tt.toID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify dependency was added
				graph, _ := builder.Build()
				if node, exists := graph.GetNode(tt.fromID); exists && tt.fromID != tt.toID {
					assert.Contains(t, node.Dependencies, tt.toID)
				}
			}
		})
	}
}
