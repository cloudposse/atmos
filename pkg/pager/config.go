package pager

import "github.com/cloudposse/atmos/pkg/schema"

// NewFromAtmosConfig creates a PageCreator configured based on AtmosConfiguration settings.
// This is a convenience function that extracts the pager settings from the AtmosConfiguration
// and creates an appropriately configured PageCreator instance.
func NewFromAtmosConfig(atmosConfig *schema.AtmosConfiguration) PageCreator {
	if atmosConfig == nil {
		return New()
	}

	// Terminal is not a pointer, so we can directly access it
	return NewWithAtmosConfigAndFlag(
		atmosConfig.Settings.Terminal.IsPagerEnabled(),
		atmosConfig.Settings.Terminal.PagerFlagExplicit,
	)
}
