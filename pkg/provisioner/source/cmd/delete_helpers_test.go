package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

// testDeleteRequest builds a forced delete request for a stack named dev.
func testDeleteRequest(componentType, component string, componentConfig map[string]any) deleteRequest {
	return deleteRequest{
		componentType:   componentType,
		component:       component,
		stack:           "dev",
		componentConfig: componentConfig,
		force:           true,
	}
}

// markProvisioned writes the provenance marker the source provisioner leaves in dir.
func markProvisioned(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, source.WriteProvenance(dir, &source.Provenance{Component: "test", Source: "https://example.invalid/x"}))
}

// stubDescribeStacks replaces the stacks lookup used by the delete guard. The components argument
// is the `components.<type>` map returned for stack dev for every component type; nil yields none.
func stubDescribeStacks(t *testing.T, components map[string]any) {
	t.Helper()
	original, originalInit := executeDescribeStacksFunc, initCliConfigForPrompt
	t.Cleanup(func() { executeDescribeStacksFunc, initCliConfigForPrompt = original, originalInit })
	initCliConfigForPrompt = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	executeDescribeStacksFunc = func(_ *schema.AtmosConfiguration, _ string, _, componentTypes, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		typed := map[string]any{}
		for _, componentType := range componentTypes {
			typed[componentType] = components
		}
		return map[string]any{"dev": map[string]any{"components": typed}}, nil
	}
}
