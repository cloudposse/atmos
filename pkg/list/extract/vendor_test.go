package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVendor(t *testing.T) {
	vendorInfos := []VendorInfo{
		{
			Component: "vpc/v1",
			Type:      VendorTypeComponent,
			Manifest:  "components/terraform/vpc/v1/component.yaml",
			Folder:    "components/terraform/vpc/v1",
		},
		{
			Component: "eks/cluster",
			Type:      VendorTypeVendor,
			Manifest:  "vendor.d/eks.yaml",
			Folder:    "components/terraform/eks/cluster",
		},
	}

	vendors, err := Vendor(vendorInfos)
	require.NoError(t, err)
	assert.Len(t, vendors, 2)

	// Verify structure of extracted data.
	for _, vendor := range vendors {
		// Check template keys (used in atmos.yaml column templates).
		assert.Contains(t, vendor, "atmos_component")
		assert.Contains(t, vendor, "atmos_vendor_type")
		assert.Contains(t, vendor, "atmos_vendor_file")
		assert.Contains(t, vendor, "atmos_vendor_target")

		// Check simple column names (easier for users).
		assert.Contains(t, vendor, "component")
		assert.Contains(t, vendor, "type")
		assert.Contains(t, vendor, "manifest")
		assert.Contains(t, vendor, "folder")
	}

	// Verify first vendor.
	vpc := vendors[0]
	assert.Equal(t, "vpc/v1", vpc["component"])
	assert.Equal(t, "vpc/v1", vpc["atmos_component"])
	assert.Equal(t, VendorTypeComponent, vpc["type"])
	assert.Equal(t, VendorTypeComponent, vpc["atmos_vendor_type"])
	assert.Equal(t, "components/terraform/vpc/v1/component.yaml", vpc["manifest"])
	assert.Equal(t, "components/terraform/vpc/v1", vpc["folder"])

	// Verify second vendor.
	eks := vendors[1]
	assert.Equal(t, "eks/cluster", eks["component"])
	assert.Equal(t, VendorTypeVendor, eks["type"])
	assert.Equal(t, "vendor.d/eks.yaml", eks["manifest"])
	assert.Equal(t, "components/terraform/eks/cluster", eks["folder"])
}

func TestVendor_EmptyList(t *testing.T) {
	vendors, err := Vendor([]VendorInfo{})
	require.NoError(t, err)
	assert.Empty(t, vendors)
}

func TestVendor_SingleVendor(t *testing.T) {
	vendorInfos := []VendorInfo{
		{
			Component: "rds",
			Type:      VendorTypeVendor,
			Manifest:  "vendor.d/rds.yaml",
			Folder:    "components/terraform/rds",
		},
	}

	vendors, err := Vendor(vendorInfos)
	require.NoError(t, err)
	assert.Len(t, vendors, 1)

	vendor := vendors[0]
	assert.Equal(t, "rds", vendor["component"])
	assert.Equal(t, VendorTypeVendor, vendor["type"])
}

func TestVendor_ComponentManifests(t *testing.T) {
	vendorInfos := []VendorInfo{
		{
			Component: "vpc",
			Type:      VendorTypeComponent,
			Manifest:  "components/terraform/vpc/component.yaml",
			Folder:    "components/terraform/vpc",
		},
		{
			Component: "eks",
			Type:      VendorTypeComponent,
			Manifest:  "components/terraform/eks/component.yaml",
			Folder:    "components/terraform/eks",
		},
	}

	vendors, err := Vendor(vendorInfos)
	require.NoError(t, err)
	assert.Len(t, vendors, 2)

	// Verify all are component manifests.
	for _, vendor := range vendors {
		assert.Equal(t, VendorTypeComponent, vendor["type"])
	}
}

func TestVendor_VendorManifests(t *testing.T) {
	vendorInfos := []VendorInfo{
		{
			Component: "vpc",
			Type:      VendorTypeVendor,
			Manifest:  "vendor.d/vpc.yaml",
			Folder:    "components/terraform/vpc",
		},
		{
			Component: "eks",
			Type:      VendorTypeVendor,
			Manifest:  "vendor.d/eks.yaml",
			Folder:    "components/terraform/eks",
		},
	}

	vendors, err := Vendor(vendorInfos)
	require.NoError(t, err)
	assert.Len(t, vendors, 2)

	// Verify all are vendor manifests.
	for _, vendor := range vendors {
		assert.Equal(t, VendorTypeVendor, vendor["type"])
	}
}

func TestVendor_MixedTypes(t *testing.T) {
	vendorInfos := []VendorInfo{
		{
			Component: "vpc",
			Type:      VendorTypeComponent,
			Manifest:  "components/terraform/vpc/component.yaml",
			Folder:    "components/terraform/vpc",
		},
		{
			Component: "eks",
			Type:      VendorTypeVendor,
			Manifest:  "vendor.d/eks.yaml",
			Folder:    "components/terraform/eks",
		},
		{
			Component: "rds",
			Type:      VendorTypeComponent,
			Manifest:  "components/terraform/rds/component.yaml",
			Folder:    "components/terraform/rds",
		},
	}

	vendors, err := Vendor(vendorInfos)
	require.NoError(t, err)
	assert.Len(t, vendors, 3)

	// Count types.
	componentCount := 0
	vendorCount := 0
	for _, vendor := range vendors {
		switch vendor["type"] {
		case VendorTypeComponent:
			componentCount++
		case VendorTypeVendor:
			vendorCount++
		}
	}

	assert.Equal(t, 2, componentCount)
	assert.Equal(t, 1, vendorCount)
}
