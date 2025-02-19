package list

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestListVendors(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "vendor_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create vendor directory structure
	vendorDir := filepath.Join(tmpDir, "vendor.d")
	err = os.MkdirAll(vendorDir, 0o755)
	require.NoError(t, err)

	// Create atmos.yaml with vendor configuration
	atmosConfig := `
base_path: ""
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  excluded_paths:
    - "**/_defaults.yaml"
vendor:
  base_path: "vendor.d"
  list:
    columns:
      # Template variables available for vendor list columns:
      # .atmos_component    - The name of the component (e.g., "vpc/v1", "eks/cluster")
      # .atmos_vendor_type  - The type of vendor configuration (e.g., "Vendor Manifest")
      # .atmos_vendor_file  - The path to the vendor manifest file (e.g., "vendor.d/vendor.yaml")
      # .atmos_vendor_target- The target directory where the component is vendored
      # Additional variables:
      # .Component  - Same as .atmos_component
      # .Version   - The version/ref of the component
      # .Source    - The source URI of the component
      # .Tags      - List of tags associated with the component
      - name: Component
        value: '{{ .atmos_component }}'
      - name: Type
        value: '{{ .atmos_vendor_type }}'
      - name: Manifest
        value: '{{ .atmos_vendor_file }}'
      - name: Folder
        value: '{{ .atmos_vendor_target }}'
`
	err = os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create stacks directory and a sample stack file
	stacksDir := filepath.Join(tmpDir, "stacks")
	err = os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	// Create a sample stack file
	stackConfig := `
components:
  terraform:
    vpc:
      component: vpc/v1
    eks:
      component: eks/cluster
    ecs:
      component: ecs/cluster
`
	err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(stackConfig), 0o644)
	require.NoError(t, err)

	// Create a component manifest file
	componentManifestFile := filepath.Join(vendorDir, "component.yaml")
	componentManifest := schema.AtmosVendorConfig{
		ApiVersion: "atmos/v1",
		Kind:       "AtmosVendorConfig",
		Metadata: schema.AtmosVendorMetadata{
			Name:        "component-test",
			Description: "Test component manifest",
		},
		Spec: schema.AtmosVendorSpec{
			Sources: []schema.AtmosVendorSource{
				{
					Component: "vpc/v1",
					Source:    "github.com/test/repo.git//components/vpc?ref=main",
					Version:   "main",
					Targets:   []string{"components/terraform/vpc/v1"},
				},
			},
		},
	}
	componentManifestBytes, err := yaml.Marshal(componentManifest)
	require.NoError(t, err)
	err = os.WriteFile(componentManifestFile, componentManifestBytes, 0o644)
	require.NoError(t, err)

	// Create a vendor manifest file
	vendorManifestFile := filepath.Join(vendorDir, "vendor.yaml")
	vendorManifest := schema.AtmosVendorConfig{
		ApiVersion: "atmos/v1",
		Kind:       "AtmosVendorConfig",
		Metadata: schema.AtmosVendorMetadata{
			Name:        "vendor-test",
			Description: "Test vendor manifest",
		},
		Spec: schema.AtmosVendorSpec{
			Sources: []schema.AtmosVendorSource{
				{
					Component: "eks/cluster",
					Source:    "github.com/test/repo.git//components/eks?ref=main",
					Version:   "main",
					Targets:   []string{"components/terraform/eks/cluster"},
				},
				{
					Component: "ecs/cluster",
					Source:    "github.com/test/repo.git//components/ecs?ref=main",
					Version:   "main",
					Targets:   []string{"components/terraform/ecs/cluster"},
				},
			},
		},
	}
	vendorManifestBytes, err := yaml.Marshal(vendorManifest)
	require.NoError(t, err)
	err = os.WriteFile(vendorManifestFile, vendorManifestBytes, 0o644)
	require.NoError(t, err)

	// Change to the temporary directory for testing
	currentDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		if err := os.Chdir(currentDir); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	tests := []struct {
		name        string
		config      schema.ListConfig
		format      string
		delimiter   string
		wantErr     bool
		contains    []string
		notContains []string
		validate    func(t *testing.T, output string)
	}{
		{
			name: "discover all vendors",
			config: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Component", Value: "{{ .atmos_component }}"},
					{Name: "Type", Value: "{{ .atmos_vendor_type }}"},
					{Name: "Manifest", Value: "{{ .atmos_vendor_file }}"},
					{Name: "Folder", Value: "{{ .atmos_vendor_target }}"},
				},
			},
			format:    "",
			delimiter: "\t",
			wantErr:   false,
			contains: []string{
				"vpc/v1", "Vendor Manifest",
				"eks/cluster", "Vendor Manifest",
				"ecs/cluster", "Vendor Manifest",
			},
		},
		{
			name:    "json format with multiple vendors",
			config:  schema.ListConfig{},
			format:  "json",
			wantErr: false,
			validate: func(t *testing.T, output string) {
				var vendors []schema.AtmosVendorSource
				err := json.Unmarshal([]byte(output), &vendors)
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, len(vendors), 3)

				// Find and validate vendors
				var foundComponent bool
				var foundEKS bool
				var foundECS bool
				for _, v := range vendors {
					if v.Component == "vpc/v1" {
						foundComponent = true
						assert.Equal(t, "vendor.d/component.yaml", v.File)
					}
					if v.Component == "eks/cluster" {
						foundEKS = true
						assert.Equal(t, "vendor.d/vendor.yaml", v.File)
					}
					if v.Component == "ecs/cluster" {
						foundECS = true
						assert.Equal(t, "vendor.d/vendor.yaml", v.File)
					}
				}
				assert.True(t, foundComponent, "Component manifest not found")
				assert.True(t, foundEKS, "EKS vendor manifest not found")
				assert.True(t, foundECS, "ECS vendor manifest not found")
			},
		},
		{
			name: "csv format with multiple vendors",
			config: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Component", Value: "{{ .atmos_component }}"},
					{Name: "Type", Value: "{{ .atmos_vendor_type }}"},
					{Name: "Manifest", Value: "{{ .atmos_vendor_file }}"},
					{Name: "Folder", Value: "{{ .atmos_vendor_target }}"},
				},
			},
			format:    "csv",
			delimiter: ",",
			wantErr:   false,
			validate: func(t *testing.T, output string) {
				// Normalize line endings and paths
				output = strings.ReplaceAll(output, "\r\n", "\n")
				lines := strings.Split(strings.TrimSpace(output), "\n")

				// Verify we have the header and at least one row
				assert.GreaterOrEqual(t, len(lines), 1, "Expected at least header row")
				assert.Equal(t, "Component,Type,Manifest,Folder", lines[0])

				var foundComponent bool
				var foundEKS bool
				var foundECS bool
				for _, line := range lines[1:] {
					fields := strings.Split(line, ",")
					// Convert all paths to forward slashes for consistent comparison
					if len(fields) == 4 {
						fields[2] = filepath.ToSlash(fields[2]) // Normalize Manifest path
						fields[3] = filepath.ToSlash(fields[3]) // Normalize Folder path
						if fields[0] == "vpc/v1" {
							foundComponent = true
							assert.Equal(t, "Vendor Manifest", fields[1])
						}
						if fields[0] == "eks/cluster" {
							foundEKS = true
							assert.Equal(t, "Vendor Manifest", fields[1])
						}
						if fields[0] == "ecs/cluster" {
							foundECS = true
							assert.Equal(t, "Vendor Manifest", fields[1])
						}
					}
				}
				assert.True(t, foundComponent, "Component manifest not found")
				assert.True(t, foundEKS, "EKS vendor manifest not found")
				assert.True(t, foundECS, "ECS vendor manifest not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListVendors(tt.config, tt.format, tt.delimiter)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Run custom validation if provided
			if tt.validate != nil {
				tt.validate(t, output)
				return
			}

			// Verify expected content is present
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			// Verify unexpected content is not present
			for _, unexpected := range tt.notContains {
				assert.NotContains(t, output, unexpected)
			}
		})
	}
}
