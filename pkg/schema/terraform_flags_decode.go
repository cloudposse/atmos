package schema

import (
	"errors"
	"fmt"

	"github.com/go-viper/mapstructure/v2"
)

// ErrInvalidTerraformFlagsConfig is returned when a flags config cannot be decoded from a stack manifest.
var ErrInvalidTerraformFlagsConfig = errors.New("invalid terraform flags configuration")

// DecodeTerraformFlags decodes a map[string]any (typically read from a stack manifest's
// merged `components.terraform.<name>.flags:` section, already resolved through the
// atmos.yaml global default, stack-level, and component-inheritance layers) into a
// TerraformFlags value.
//
// Returns a zero-value TerraformFlags (all fields unset) when raw is nil or empty, so
// callers can write:
//
//	flags, err := schema.DecodeTerraformFlags(componentSection["flags"])
//
// Errors are joined with ErrInvalidTerraformFlagsConfig for predictable error checking.
func DecodeTerraformFlags(raw any) (TerraformFlags, error) {
	if raw == nil {
		return TerraformFlags{}, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return TerraformFlags{}, fmt.Errorf("%w: expected map, got %T", ErrInvalidTerraformFlagsConfig, raw)
	}
	if len(m) == 0 {
		return TerraformFlags{}, nil
	}

	var flags TerraformFlags
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &flags,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
	})
	if err != nil {
		return TerraformFlags{}, errors.Join(ErrInvalidTerraformFlagsConfig, err)
	}
	if err := decoder.Decode(m); err != nil {
		return TerraformFlags{}, errors.Join(ErrInvalidTerraformFlagsConfig, err)
	}
	return flags, nil
}
