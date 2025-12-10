package extract

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// VendorTypeComponent is the type for components with component manifests.
	VendorTypeComponent = "Component Manifest"
	// VendorTypeVendor is the type for vendor manifests.
	VendorTypeVendor = "Vendor Manifest"
)

// VendorInfo holds information about a vendor component.
type VendorInfo struct {
	Component string
	Type      string
	Manifest  string
	Folder    string
}

// Vendor transforms vendorInfos into structured data.
// Returns []map[string]any suitable for the renderer pipeline.
func Vendor(vendorInfos []VendorInfo) ([]map[string]any, error) {
	defer perf.Track(nil, "extract.Vendor")()

	var vendors []map[string]any

	for _, vi := range vendorInfos {
		vendor := map[string]any{
			"atmos_component":     vi.Component,
			"atmos_vendor_type":   vi.Type,
			"atmos_vendor_file":   vi.Manifest,
			"atmos_vendor_target": vi.Folder,
			// Also add simple column names for easier templates.
			"component": vi.Component,
			"type":      vi.Type,
			"manifest":  vi.Manifest,
			"folder":    vi.Folder,
		}

		vendors = append(vendors, vendor)
	}

	return vendors, nil
}

// GetVendorInfosFunc is a function type for getting vendor information.
// This allows the extract package to avoid importing the list package.
type GetVendorInfosFunc func(*schema.AtmosConfiguration) ([]VendorInfo, error)
