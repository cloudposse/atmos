package list

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExtractVendor transforms vendorInfos into structured data.
// Returns []map[string]any suitable for the renderer pipeline.
func ExtractVendor(vendorInfos []VendorInfo) ([]map[string]any, error) {
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

// GetVendorConfigurations is a wrapper around getVendorInfos that returns data suitable for the renderer.
func GetVendorConfigurations(atmosConfig *schema.AtmosConfiguration) ([]map[string]any, error) {
	vendorInfos, err := getVendorInfos(atmosConfig)
	if err != nil {
		return nil, err
	}

	return ExtractVendor(vendorInfos)
}
