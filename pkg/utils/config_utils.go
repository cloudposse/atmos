package utils

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExtractAtmosConfig extracts the Atmos configuration from any data type.
// It handles both direct AtmosConfiguration instances and pointers to AtmosConfiguration.
// If the data is neither, it returns an empty configuration.
func ExtractAtmosConfig(data any) schema.AtmosConfiguration {
	defer perf.Track(nil, "utils.ExtractAtmosConfig")()

	switch v := data.(type) {
	case schema.AtmosConfiguration:
		return v
	case *schema.AtmosConfiguration:
		return *v
	default:
		return schema.AtmosConfiguration{}
	}
}
