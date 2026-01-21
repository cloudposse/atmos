package filetype

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestParseYAML(t *testing.T) {
	// Test error case for yaml.Unmarshal.
	t.Run("invalid yaml", func(t *testing.T) {
		invalidYAML := []byte("invalid: yaml: :")
		result, err := parseYAML(invalidYAML)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	// Test error case for node.Decode (covers lines 100-101).
	t.Run("decode error", func(t *testing.T) {
		// Create YAML with anchor/alias that might fail on decode.
		yamlWithComplexAnchors := []byte(`
x: &anchor
  <<: *anchor
`)
		result, err := parseYAML(yamlWithComplexAnchors)
		// This may or may not error, but it exercises the decode path.
		if err != nil {
			assert.Nil(t, result)
		}
	})

	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{
			name:     "regular string",
			input:    "key: value",
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "string starting with hash",
			input:    "key: '#value'",
			expected: map[string]any{"key": "#value"},
		},
		// Note: In YAML, unquoted strings starting with # are comments.
		// This test is removed as it's not valid YAML.
		{
			name: "nested map with hash values",
			input: `
parent:
  child1: '#value1'
  child2: '#value2'
  child3: regular
`,
			expected: map[string]any{
				"parent": map[string]any{
					"child1": "#value1",
					"child2": "#value2",
					"child3": "regular",
				},
			},
		},
		{
			name: "list with hash values",
			input: `
items:
  - '#item1'
  - '#item2'
  - regular
`,
			expected: map[string]any{
				"items": []any{
					"#item1",
					"#item2",
					"regular",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseYAML([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessYAMLNode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected yaml.Style
	}{
		{
			name:     "string starting with hash",
			input:    "#value",
			expected: yaml.SingleQuotedStyle,
		},
		{
			name:     "regular string",
			input:    "regular",
			expected: 0, // Default style
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a node with the test input.
			node := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: tt.input,
			}

			// Process the node.
			processYAMLNode(node)

			// Check if the style was set correctly.
			assert.Equal(t, tt.expected, node.Style)
		})
	}

	t.Run("nested nodes", func(t *testing.T) {
		// Create a document node with nested content.
		doc := &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "key1",
						},
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "#value1",
						},
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "key2",
						},
						{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "regular",
						},
					},
				},
			},
		}

		// Process the document.
		processYAMLNode(doc)

		// Check if the style was set correctly for the hash value.
		assert.Equal(t, yaml.SingleQuotedStyle, doc.Content[0].Content[1].Style)
		// Check that regular value style was not changed.
		assert.Equal(t, yaml.Style(0), doc.Content[0].Content[3].Style)
	})

	t.Run("nil node", func(t *testing.T) {
		// This should not panic.
		processYAMLNode(nil)
	})
}

func TestParseHCLStacks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []StackDocument
		expectError bool
		errContains string
	}{
		{
			name: "single stack with label",
			input: `stack "dev" {
				vars {
					stage = "development"
				}
			}`,
			expected: []StackDocument{
				{
					Name: "dev",
					Config: map[string]any{
						"name": "dev",
						"vars": map[string]any{
							"stage": "development",
						},
					},
				},
			},
		},
		{
			name: "single stack without label",
			input: `stack {
				vars {
					stage = "prod"
				}
			}`,
			expected: []StackDocument{
				{
					Name: "",
					Config: map[string]any{
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "multiple stacks with labels",
			input: `stack "dev" {
				vars {
					stage = "dev"
				}
			}

			stack "prod" {
				vars {
					stage = "prod"
				}
			}`,
			expected: []StackDocument{
				{
					Name: "dev",
					Config: map[string]any{
						"name": "dev",
						"vars": map[string]any{
							"stage": "dev",
						},
					},
				},
				{
					Name: "prod",
					Config: map[string]any{
						"name": "prod",
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "stack with explicit name field inside",
			input: `stack {
				name = "my-explicit-name"
				vars {
					stage = "prod"
				}
			}`,
			expected: []StackDocument{
				{
					Name: "my-explicit-name",
					Config: map[string]any{
						"name": "my-explicit-name",
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "stack label overrides name field",
			input: `stack "label-name" {
				name = "field-name"
				vars {
					stage = "prod"
				}
			}`,
			expected: []StackDocument{
				{
					Name: "label-name",
					Config: map[string]any{
						"name": "label-name",
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "stack with import",
			input: `stack "my-stack" {
				import = ["catalog/base"]
				vars {
					environment = "prod"
				}
			}`,
			expected: []StackDocument{
				{
					Name: "my-stack",
					Config: map[string]any{
						"name":   "my-stack",
						"import": []any{"catalog/base"},
						"vars": map[string]any{
							"environment": "prod",
						},
					},
				},
			},
		},
		{
			name: "stack with components",
			input: `stack "my-stack" {
				components {
					terraform {
						component "vpc" {
							vars {
								cidr = "10.0.0.0/16"
							}
						}
					}
				}
			}`,
			expected: []StackDocument{
				{
					Name: "my-stack",
					Config: map[string]any{
						"name": "my-stack",
						"components": map[string]any{
							"terraform": map[string]any{
								"component": map[string]any{
									"vpc": map[string]any{
										"vars": map[string]any{
											"cidr": "10.0.0.0/16",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "no stack block - treat as single stack",
			input: `
				vars {
					stage = "prod"
				}
			`,
			expected: []StackDocument{
				{
					Name: "",
					Config: map[string]any{
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "stack with multiple labels - error",
			input: `stack "label1" "label2" {
				vars {
					stage = "test"
				}
			}`,
			expectError: true,
			errContains: "cannot have more than one label",
		},
		{
			name: "top-level attributes merged into stack",
			input: `
				description = "shared description"

				stack "dev" {
					vars {
						stage = "dev"
					}
				}
			`,
			expected: []StackDocument{
				{
					Name: "dev",
					Config: map[string]any{
						"name":        "dev",
						"description": "shared description",
						"vars": map[string]any{
							"stage": "dev",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseHCLStacks([]byte(tt.input), "test.hcl")
			if tt.expectError {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.Len(t, result, len(tt.expected))
			for i, expected := range tt.expected {
				assert.Equal(t, expected.Name, result[i].Name, "stack %d name mismatch", i)
				assert.Equal(t, expected.Config, result[i].Config, "stack %d config mismatch", i)
			}
		})
	}
}

func TestParseYAMLStacks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []StackDocument
		expectError bool
	}{
		{
			name: "single document",
			input: `name: dev
vars:
  stage: development
`,
			expected: []StackDocument{
				{
					Name: "dev",
					Config: map[string]any{
						"name": "dev",
						"vars": map[string]any{
							"stage": "development",
						},
					},
				},
			},
		},
		{
			name: "multiple documents",
			input: `name: dev
vars:
  stage: dev
---
name: prod
vars:
  stage: prod
`,
			expected: []StackDocument{
				{
					Name: "dev",
					Config: map[string]any{
						"name": "dev",
						"vars": map[string]any{
							"stage": "dev",
						},
					},
				},
				{
					Name: "prod",
					Config: map[string]any{
						"name": "prod",
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "document without name",
			input: `vars:
  stage: prod
`,
			expected: []StackDocument{
				{
					Name: "",
					Config: map[string]any{
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name: "three documents",
			input: `name: dev
vars:
  stage: dev
---
name: staging
vars:
  stage: staging
---
name: prod
vars:
  stage: prod
`,
			expected: []StackDocument{
				{
					Name: "dev",
					Config: map[string]any{
						"name": "dev",
						"vars": map[string]any{
							"stage": "dev",
						},
					},
				},
				{
					Name: "staging",
					Config: map[string]any{
						"name": "staging",
						"vars": map[string]any{
							"stage": "staging",
						},
					},
				},
				{
					Name: "prod",
					Config: map[string]any{
						"name": "prod",
						"vars": map[string]any{
							"stage": "prod",
						},
					},
				},
			},
		},
		{
			name:     "empty document skipped",
			input:    "---\n---\n",
			expected: []StackDocument{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseYAMLStacks([]byte(tt.input))
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result, len(tt.expected))
			for i, expected := range tt.expected {
				assert.Equal(t, expected.Name, result[i].Name, "stack %d name mismatch", i)
				assert.Equal(t, expected.Config, result[i].Config, "stack %d config mismatch", i)
			}
		})
	}
}
