package list

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

// TestFilterAndListVendor tests the vendor listing functionality

func TestFilterAndListVendor(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "atmos-test-vendor")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	vendorDir := filepath.Join(tempDir, "vendor.d")
	err = os.Mkdir(vendorDir, 0755)
	if err != nil {
		t.Fatalf("Error creating vendor dir: %v", err)
	}

	componentsDir := filepath.Join(tempDir, "components")
	err = os.Mkdir(componentsDir, 0755)
	if err != nil {
		t.Fatalf("Error creating components dir: %v", err)
	}

	terraformDir := filepath.Join(componentsDir, "terraform")
	err = os.Mkdir(terraformDir, 0755)
	if err != nil {
		t.Fatalf("Error creating terraform dir: %v", err)
	}

	vpcDir := filepath.Join(terraformDir, "vpc/v1")
	err = os.MkdirAll(vpcDir, 0755)
	if err != nil {
		t.Fatalf("Error creating vpc dir: %v", err)
	}

	componentYaml := `apiVersion: atmos/v1
kind: Component
metadata:
  name: vpc
  description: VPC component
spec:
  source:
    type: git
    uri: github.com/cloudposse/terraform-aws-vpc
    version: 1.0.0
`
	err = os.WriteFile(filepath.Join(vpcDir, "component.yaml"), []byte(componentYaml), 0644)
	if err != nil {
		t.Fatalf("Error writing component.yaml: %v", err)
	}

	vendorYaml := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: eks
  description: EKS component
spec:
  sources:
    - component: eks/cluster
      source: github.com/cloudposse/terraform-aws-eks-cluster
      version: 1.0.0
      file: vendor.d/eks
      targets:
        - components/terraform/eks/cluster
    - component: ecs/cluster
      source: github.com/cloudposse/terraform-aws-ecs-cluster
      version: 1.0.0
      file: vendor.d/ecs
      targets:
        - components/terraform/ecs/cluster
`
	err = os.WriteFile(filepath.Join(vendorDir, "vendor.yaml"), []byte(vendorYaml), 0644)
	if err != nil {
		t.Fatalf("Error writing vendor.yaml: %v", err)
	}

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Vendor: schema.Vendor{
			BasePath: "vendor.d",
			List: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{
						Name:  "Component",
						Value: "{{ .atmos_component }}",
					},
					{
						Name:  "Type",
						Value: "{{ .atmos_vendor_type }}",
					},
					{
						Name:  "Manifest",
						Value: "{{ .atmos_vendor_file }}",
					},
					{
						Name:  "Folder",
						Value: "{{ .atmos_vendor_target }}",
					},
				},
			},
		},
	}

	// Test table format (default)
	t.Run("TableFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatTable),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component")
		assert.Contains(t, output, "Type")
		assert.Contains(t, output, "Manifest")
		assert.Contains(t, output, "Folder")
		assert.Contains(t, output, "vpc/v1")
		assert.Contains(t, output, "Component Manifest")
		assert.Contains(t, output, "eks/cluster")
		assert.Contains(t, output, "Vendor Manifest")
		assert.Contains(t, output, "ecs/cluster")
	})

	// Test JSON format
	t.Run("JSONFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatJSON),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "\"Component\": \"vpc/v1\"")
		assert.Contains(t, output, "\"Type\": \"Component Manifest\"")
		assert.Contains(t, output, "\"Component\": \"eks/cluster\"")
		assert.Contains(t, output, "\"Type\": \"Vendor Manifest\"")
		assert.Contains(t, output, "\"Component\": \"ecs/cluster\"")
	})

	// Test YAML format
	t.Run("YAMLFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatYAML),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component: vpc/v1")
		assert.Contains(t, output, "Type: Component Manifest")
		assert.Contains(t, output, "Component: eks/cluster")
		assert.Contains(t, output, "Type: Vendor Manifest")
		assert.Contains(t, output, "Component: ecs/cluster")
	})

	// Test CSV format
	t.Run("CSVFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatCSV),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component,Type,Manifest,Folder")
		assert.Contains(t, output, "vpc/v1,Component Manifest")
		assert.Contains(t, output, "eks/cluster,Vendor Manifest")
		assert.Contains(t, output, "ecs/cluster,Vendor Manifest")
	})

	// Test TSV format
	t.Run("TSVFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatTSV),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component\tType\tManifest\tFolder")
		assert.Contains(t, output, "vpc/v1\tComponent Manifest")
		assert.Contains(t, output, "eks/cluster\tVendor Manifest")
		assert.Contains(t, output, "ecs/cluster\tVendor Manifest")
	})

	// Test stack pattern filtering
	t.Run("StackPatternFiltering", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr:    string(format.FormatTable),
			StackPattern: "vpc*",
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "vpc/v1")
		assert.NotContains(t, output, "eks/cluster")
		assert.NotContains(t, output, "ecs/cluster")
	})

	// Test multiple stack patterns
	t.Run("MultipleStackPatterns", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr:    string(format.FormatTable),
			StackPattern: "vpc*,ecs*",
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "vpc/v1")
		assert.NotContains(t, output, "eks/cluster")
		assert.Contains(t, output, "ecs/cluster")
	})

	// Test error when vendor.base_path not set
	t.Run("ErrorVendorBasepathNotSet", func(t *testing.T) {
		invalidConfig := atmosConfig
		invalidConfig.Vendor.BasePath = ""

		options := &FilterOptions{
			FormatStr: string(format.FormatTable),
		}

		_, err := FilterAndListVendor(&invalidConfig, options)
		assert.Error(t, err)
		assert.Equal(t, ErrVendorBasepathNotSet, err)
	})
}
