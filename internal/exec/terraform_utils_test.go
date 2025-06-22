package exec

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Helper function to create a bool pointer for testing.
func boolPtr(b bool) *bool {
	return &b
}

func TestIsWorkspacesEnabled(t *testing.T) {
	// Test cases for isWorkspacesEnabled function.
	tests := []struct {
		name              string
		backendType       string
		workspacesEnabled *bool
		expectedEnabled   bool
		expectWarning     bool
	}{
		{
			name:              "Default behavior (no explicit setting, non-HTTP backend)",
			backendType:       "s3",
			workspacesEnabled: nil,
			expectedEnabled:   true,
			expectWarning:     false,
		},
		{
			name:              "HTTP backend automatically disables workspaces",
			backendType:       "http",
			workspacesEnabled: nil,
			expectedEnabled:   false,
			expectWarning:     false,
		},
		{
			name:              "Explicitly disabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(false),
			expectedEnabled:   false,
			expectWarning:     false,
		},
		{
			name:              "Explicitly enabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(true),
			expectedEnabled:   true,
			expectWarning:     false,
		},
		{
			name:              "HTTP backend ignores explicitly enabled workspaces with warning",
			backendType:       "http",
			workspacesEnabled: boolPtr(true),
			expectedEnabled:   false,
			expectWarning:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config.
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						WorkspacesEnabled: tc.workspacesEnabled,
					},
				},
			}

			info := &schema.ConfigAndStacksInfo{
				ComponentBackendType: tc.backendType,
				Component:            "test-component",
			}

			// Test function.
			result := isWorkspacesEnabled(atmosConfig, info)

			// Assert results.
			assert.Equal(t, tc.expectedEnabled, result, "Expected workspace enabled status to match")
		})
	}
}

func TestExecuteTerraformAffectedWithDependents(t *testing.T) {
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/terraform-apply-affected"
	if err = os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	oldStd := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	stack := "prod"

	info := schema.ConfigAndStacksInfo{
		Stack:         stack,
		ComponentType: "terraform",
		SubCommand:    "plan",
		Affected:      true,
		DryRun:        true,
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		t.Fatalf("Failed to execute 'InitCliConfig': %v", err)
	}

	a := DescribeAffectedCmdArgs{
		CLIConfig:         &atmosConfig,
		Stack:             stack,
		IncludeDependents: true,
	}

	err = ExecuteTerraformAffected(&a, &info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraformAffected': %v", err)
	}

	w.Close()
	os.Stderr = oldStd

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()

	assert.NotEmptyf(t, output, "Expected output to be non-empty, got %q", output)
}

func TestExecuteTerraformQuery(t *testing.T) {
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/terraform-apply-affected"
	if err = os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	oldStd := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	stack := "prod"

	info := schema.ConfigAndStacksInfo{
		Stack:         stack,
		ComponentType: "terraform",
		SubCommand:    "plan",
		Affected:      true,
		DryRun:        true,
		Query:         ".vars.tags.team == \"eks\"",
	}

	err = ExecuteTerraformQuery(&info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraformAffected': %v", err)
	}

	w.Close()
	os.Stderr = oldStd

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()

	assert.Contains(t, output, "Executing command=\"atmos terraform plan eks/cluster -s prod\"")
	assert.Contains(t, output, "Executing command=\"atmos terraform plan eks/external-dns -s prod\"")
	assert.Contains(t, output, "Executing command=\"atmos terraform plan eks/karpenter -s prod\"")
	assert.Contains(t, output, "Executing command=\"atmos terraform plan eks/karpenter-node-pool -s prod\"")
	assert.Contains(t, output, "Skipping the component because the query criteria not satisfied command=\"atmos terraform plan vpc -s prod\"")
	assert.Contains(t, output, "Skipping the component because the query criteria not satisfied command=\"atmos terraform plan eks/istio/base -s prod\"")
	assert.Contains(t, output, "Skipping the component because the query criteria not satisfied command=\"atmos terraform plan eks/istio/istiod -s prod\"")
	assert.Contains(t, output, "Skipping the component because the query criteria not satisfied command=\"atmos terraform plan eks/istio/test-app -s prod\"")
}
