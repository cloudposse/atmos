package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestExtractYAMLPositions_Disabled(t *testing.T) {
	yamlContent := `
vars:
  name: test
  count: 42
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	// Extract with enabled=false (should return empty map immediately).
	positions := ExtractYAMLPositions(&node, false)

	assert.Empty(t, positions)
}

func TestExtractYAMLPositions_NilNode(t *testing.T) {
	// Extract from nil node (should not panic).
	positions := ExtractYAMLPositions(nil, true)

	assert.Empty(t, positions)
}

func TestExtractYAMLPositions_SimpleMap(t *testing.T) {
	yamlContent := `vars:
  name: test
  count: 42
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Check that we have positions for the expected paths.
	assert.True(t, HasYAMLPosition(positions, "vars"))
	assert.True(t, HasYAMLPosition(positions, "vars.name"))
	assert.True(t, HasYAMLPosition(positions, "vars.count"))

	// Check line numbers are present and reasonable.
	// Note: Exact line numbers depend on YAML content formatting.
	assert.Greater(t, positions["vars"].Line, 0)
	assert.Greater(t, positions["vars.name"].Line, 0)
	assert.Greater(t, positions["vars.count"].Line, 0)

	// Verify ordering: vars comes before or same as vars.name, which comes before vars.count.
	assert.LessOrEqual(t, positions["vars"].Line, positions["vars.name"].Line)
	assert.Less(t, positions["vars.name"].Line, positions["vars.count"].Line)

	// All should have column > 0.
	assert.Greater(t, positions["vars"].Column, 0)
	assert.Greater(t, positions["vars.name"].Column, 0)
	assert.Greater(t, positions["vars.count"].Column, 0)
}

func TestExtractYAMLPositions_NestedMap(t *testing.T) {
	yamlContent := `vars:
  tags:
    environment: dev
    team: platform
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Check nested paths.
	assert.True(t, HasYAMLPosition(positions, "vars"))
	assert.True(t, HasYAMLPosition(positions, "vars.tags"))
	assert.True(t, HasYAMLPosition(positions, "vars.tags.environment"))
	assert.True(t, HasYAMLPosition(positions, "vars.tags.team"))

	// Check line numbers are reasonable and ordered correctly.
	assert.Greater(t, positions["vars"].Line, 0)
	assert.Greater(t, positions["vars.tags"].Line, 0)
	assert.Greater(t, positions["vars.tags.environment"].Line, 0)
	assert.Greater(t, positions["vars.tags.team"].Line, 0)

	// Verify ordering.
	assert.LessOrEqual(t, positions["vars"].Line, positions["vars.tags"].Line)
	assert.LessOrEqual(t, positions["vars.tags"].Line, positions["vars.tags.environment"].Line)
	assert.Less(t, positions["vars.tags.environment"].Line, positions["vars.tags.team"].Line)
}

func TestExtractYAMLPositions_Array(t *testing.T) {
	yamlContent := `vars:
  zones:
    - us-east-1a
    - us-east-1b
    - us-east-1c
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Check array paths.
	assert.True(t, HasYAMLPosition(positions, "vars"))
	assert.True(t, HasYAMLPosition(positions, "vars.zones"))
	assert.True(t, HasYAMLPosition(positions, "vars.zones[0]"))
	assert.True(t, HasYAMLPosition(positions, "vars.zones[1]"))
	assert.True(t, HasYAMLPosition(positions, "vars.zones[2]"))

	// Check line numbers are reasonable.
	assert.Greater(t, positions["vars"].Line, 0)
	assert.Greater(t, positions["vars.zones"].Line, 0)
	assert.Greater(t, positions["vars.zones[0]"].Line, 0)
	assert.Greater(t, positions["vars.zones[1]"].Line, 0)
	assert.Greater(t, positions["vars.zones[2]"].Line, 0)

	// Verify ordering.
	assert.LessOrEqual(t, positions["vars"].Line, positions["vars.zones"].Line)
	assert.LessOrEqual(t, positions["vars.zones"].Line, positions["vars.zones[0]"].Line)
	assert.Less(t, positions["vars.zones[0]"].Line, positions["vars.zones[1]"].Line)
	assert.Less(t, positions["vars.zones[1]"].Line, positions["vars.zones[2]"].Line)
}

func TestExtractYAMLPositions_ArrayOfMaps(t *testing.T) {
	yamlContent := `items:
  - name: item1
    value: 100
  - name: item2
    value: 200
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Check array of maps paths.
	assert.True(t, HasYAMLPosition(positions, "items"))
	assert.True(t, HasYAMLPosition(positions, "items[0]"))
	assert.True(t, HasYAMLPosition(positions, "items[0].name"))
	assert.True(t, HasYAMLPosition(positions, "items[0].value"))
	assert.True(t, HasYAMLPosition(positions, "items[1]"))
	assert.True(t, HasYAMLPosition(positions, "items[1].name"))
	assert.True(t, HasYAMLPosition(positions, "items[1].value"))

	// Check line numbers are reasonable.
	assert.Greater(t, positions["items"].Line, 0)
	assert.Greater(t, positions["items[0]"].Line, 0)
	assert.Greater(t, positions["items[0].name"].Line, 0)
	assert.Greater(t, positions["items[0].value"].Line, 0)
	assert.Greater(t, positions["items[1]"].Line, 0)
	assert.Greater(t, positions["items[1].name"].Line, 0)
	assert.Greater(t, positions["items[1].value"].Line, 0)

	// Verify ordering.
	assert.LessOrEqual(t, positions["items"].Line, positions["items[0]"].Line)
	assert.LessOrEqual(t, positions["items[0]"].Line, positions["items[0].name"].Line)
	assert.Less(t, positions["items[0].value"].Line, positions["items[1]"].Line)
}

func TestExtractYAMLPositions_ComplexStructure(t *testing.T) {
	yamlContent := `component:
  vars:
    name: vpc
    cidr: 10.0.0.0/16
    availability_zones:
      - us-east-1a
      - us-east-1b
    tags:
      environment: dev
      team: platform
  settings:
    enabled: true
    count: 3
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Spot check some paths.
	assert.True(t, HasYAMLPosition(positions, "component"))
	assert.True(t, HasYAMLPosition(positions, "component.vars"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.name"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.cidr"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.availability_zones"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.availability_zones[0]"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.availability_zones[1]"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.tags"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.tags.environment"))
	assert.True(t, HasYAMLPosition(positions, "component.vars.tags.team"))
	assert.True(t, HasYAMLPosition(positions, "component.settings"))
	assert.True(t, HasYAMLPosition(positions, "component.settings.enabled"))
	assert.True(t, HasYAMLPosition(positions, "component.settings.count"))

	// Verify all positions have valid line numbers.
	for path, pos := range positions {
		assert.Greater(t, pos.Line, 0, "path %s should have line > 0", path)
		assert.Greater(t, pos.Column, 0, "path %s should have column > 0", path)
	}
}

func TestExtractYAMLPositions_EmptyMap(t *testing.T) {
	yamlContent := `vars: {}`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Should have position for the empty map.
	assert.True(t, HasYAMLPosition(positions, "vars"))
	assert.Equal(t, 1, positions["vars"].Line)
}

func TestExtractYAMLPositions_EmptyArray(t *testing.T) {
	yamlContent := `items: []`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Should have position for the empty array.
	assert.True(t, HasYAMLPosition(positions, "items"))
	assert.Equal(t, 1, positions["items"].Line)
}

func TestExtractYAMLPositions_MultipleDocuments(t *testing.T) {
	// YAML supports multiple documents in one file, separated by ---.
	// We should handle the first document.
	yamlContent := `---
vars:
  name: test
---
other:
  value: 42
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Should extract from first document.
	assert.True(t, HasYAMLPosition(positions, "vars"))
	assert.True(t, HasYAMLPosition(positions, "vars.name"))

	// The second document is not parsed by a single Unmarshal.
	// This is expected behavior.
}

func TestGetYAMLPosition(t *testing.T) {
	positions := PositionMap{
		"vars.name": {Line: 10, Column: 5},
		"vars.tags": {Line: 15, Column: 3},
	}

	tests := []struct {
		name     string
		path     string
		expected Position
	}{
		{
			name:     "existing path",
			path:     "vars.name",
			expected: Position{Line: 10, Column: 5},
		},
		{
			name:     "another existing path",
			path:     "vars.tags",
			expected: Position{Line: 15, Column: 3},
		},
		{
			name:     "non-existing path",
			path:     "nonexistent",
			expected: Position{Line: 0, Column: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetYAMLPosition(positions, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetYAMLPosition_NilMap(t *testing.T) {
	result := GetYAMLPosition(nil, "vars.name")
	assert.Equal(t, Position{}, result)
}

func TestHasYAMLPosition(t *testing.T) {
	positions := PositionMap{
		"vars.name": {Line: 10, Column: 5},
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "existing path",
			path:     "vars.name",
			expected: true,
		},
		{
			name:     "non-existing path",
			path:     "nonexistent",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasYAMLPosition(positions, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasYAMLPosition_NilMap(t *testing.T) {
	result := HasYAMLPosition(nil, "vars.name")
	assert.False(t, result)
}

func TestExtractYAMLPositions_RealWorldExample(t *testing.T) {
	// Realistic Atmos stack configuration.
	yamlContent := `terraform:
  vars:
    enabled: true
    name: vpc
    ipv4_primary_cidr_block: 10.0.0.0/16
    availability_zones:
      - us-east-1a
      - us-east-1b
      - us-east-1c
    tags:
      Environment: dev
      ManagedBy: atmos
      Team: platform

settings:
  spacelift:
    workspace_enabled: true
    autodeploy: false
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	positions := ExtractYAMLPositions(&node, true)

	// Verify we can track deeply nested values.
	assert.True(t, HasYAMLPosition(positions, "terraform.vars.ipv4_primary_cidr_block"))
	assert.True(t, HasYAMLPosition(positions, "terraform.vars.availability_zones[2]"))
	assert.True(t, HasYAMLPosition(positions, "terraform.vars.tags.ManagedBy"))
	assert.True(t, HasYAMLPosition(positions, "settings.spacelift.autodeploy"))

	// Verify line numbers are reasonable.
	// Note: Exact line numbers depend on YAML parser behavior.
	cidrPos := positions["terraform.vars.ipv4_primary_cidr_block"]
	assert.Greater(t, cidrPos.Line, 0)
	assert.Greater(t, cidrPos.Column, 0)
}
