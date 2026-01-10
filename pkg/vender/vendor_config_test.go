// The package name `vendor` is reserved in Go for dependency management.
// To avoid conflicts, the name `vender` was chosen as an alternative.
package vender

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	e "github.com/cloudposse/atmos/pkg/vendoring"
)

func TestVendorConfigScenarios(t *testing.T) {
	testDir := t.TempDir()

	// Initialize CLI config with required paths
	atmosConfig := schema.AtmosConfiguration{
		BasePath: testDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}
	atmosConfig.Logs.Level = "Trace"

	// Setup test component directory
	componentPath := filepath.Join(testDir, "components", "terraform", "mock")
	err := os.MkdirAll(componentPath, 0o755)
	assert.Nil(t, err)

	// Test Case 1: vendor.yaml exists and component is defined in it
	t.Run("vendor.yaml exists with defined component", func(t *testing.T) {
		// Create vendor.yaml
		vendorYaml := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test-vendor-config
spec:
  sources:
    - component: mock
      source: github.com/cloudposse/terraform-null-label.git//exports?ref={{.Version}}
      version: 0.25.0
      included_paths:
        - "**/*.tf"
`
		vendorYamlPath := filepath.Join(testDir, "vendor.yaml")
		err := os.WriteFile(vendorYamlPath, []byte(vendorYaml), 0o644)
		assert.Nil(t, err)

		// Test vendoring with component flag
		vendorConfig, exists, configFile, err := e.ReadAndProcessVendorConfigFile(&atmosConfig, vendorYamlPath, true)
		assert.Nil(t, err)
		assert.True(t, exists)
		assert.NotEmpty(t, configFile)

		// Verify the component exists in vendor config
		var found bool
		for _, source := range vendorConfig.Spec.Sources {
			if source.Component == "mock" {
				found = true
				break
			}
		}
		assert.True(t, found, "Component 'mock' should be defined in vendor.yaml")

		// Clean up
		err = os.Remove(vendorYamlPath)
		assert.Nil(t, err)
	})

	// Test Case 2: No vendor.yaml but component.yaml exists
	t.Run("component.yaml exists without vendor.yaml", func(t *testing.T) {
		// Create component.yaml
		componentYaml := `apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: mock-vendor-config
spec:
  source:
    uri: github.com/cloudposse/terraform-null-label.git//exports?ref={{.Version}}
    version: 0.25.0
`
		componentYamlPath := filepath.Join(componentPath, "component.yaml")
		err := os.WriteFile(componentYamlPath, []byte(componentYaml), 0o644)
		assert.Nil(t, err)

		// Test component vendoring
		componentConfig, compPath, err := e.ReadAndProcessComponentVendorConfigFile(&atmosConfig, "mock", "terraform")
		assert.Nil(t, err)
		assert.NotNil(t, componentConfig)
		assert.Equal(t, componentPath, compPath)

		// Clean up
		err = os.Remove(componentYamlPath)
		assert.Nil(t, err)
	})

	// Test Case 3: Neither vendor.yaml nor component.yaml exists
	t.Run("no vendor.yaml or component.yaml", func(t *testing.T) {
		// Test vendoring with component flag
		vendorYamlPath := filepath.Join(testDir, "vendor.yaml")
		_, exists, _, err := e.ReadAndProcessVendorConfigFile(&atmosConfig, vendorYamlPath, true)
		assert.Nil(t, err)
		assert.False(t, exists)

		// Test component vendoring
		_, _, err = e.ReadAndProcessComponentVendorConfigFile(&atmosConfig, "mock", "terraform")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	// Test Case 4: No component specified with vendor.yaml
	t.Run("no component specified with vendor.yaml", func(t *testing.T) {
		// Create vendor.yaml
		vendorYaml := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test-vendor-config
spec:
  sources:
    - component: mock
      source: github.com/cloudposse/terraform-null-label.git//exports?ref={{.Version}}
      version: 0.25.0
`
		vendorYamlPath := filepath.Join(testDir, "vendor.yaml")
		err := os.WriteFile(vendorYamlPath, []byte(vendorYaml), 0o644)
		assert.Nil(t, err)

		// Test vendoring without component flag
		vendorConfig, exists, configFile, err := e.ReadAndProcessVendorConfigFile(&atmosConfig, vendorYamlPath, true)
		assert.Nil(t, err)
		assert.True(t, exists)
		assert.NotEmpty(t, configFile)
		assert.NotNil(t, vendorConfig.Spec.Sources)

		// Clean up
		err = os.Remove(vendorYamlPath)
		assert.Nil(t, err)
	})
}
