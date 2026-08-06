package schema

import (
	"errors"
	"fmt"

	"github.com/go-viper/mapstructure/v2"
)

// ErrInvalidComponentProConfig is returned when a component-level `pro:` section cannot be
// decoded into ComponentProSettings -- either because it isn't a map, or because it contains a
// key that doesn't match any known field.
var ErrInvalidComponentProConfig = errors.New("invalid pro configuration")

// DecodeComponentPro decodes a map[string]any (typically read from a stack manifest's
// `components.<type>.<name>.pro:` section) into a *ComponentProSettings.
//
// Unlike DecodeRetryConfig, this decoder rejects unknown keys (ErrorUnused: true). The JSON
// Schema behind `atmos validate stacks` already treats `pro:` as `additionalProperties: false`,
// so a typo like `pro: {enable: true}` (meant "enabled") is schema-invalid -- but validation is a
// separate, opt-in command, not something `describe`/`plan`/`apply` run automatically. Without
// this check, that typo silently produces an empty-looking pro block that still resolves to
// enabled-by-default, the exact opposite of a user's likely intent when trying to disable Pro.
// Making the runtime agree with the schema here closes that gap.
//
// Returns (nil, nil) when the input is nil or empty so callers can write:
//
//	cfg, err := schema.DecodeComponentPro(componentSection["pro"])
//
// Errors are joined with ErrInvalidComponentProConfig for predictable error checking. Callers
// that only want to warn (not fail) on a bad `pro:` block should treat any error here as
// non-fatal -- see internal/exec's extraction of the pro section for why this must never abort
// processing of sibling components in the same manifest file.
func DecodeComponentPro(raw any) (*ComponentProSettings, error) {
	if raw == nil {
		return nil, nil //nolint:nilnil // (nil, nil) is the documented "nothing to decode" contract, mirroring DecodeRetryConfig.
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected map, got %T", ErrInvalidComponentProConfig, raw)
	}
	if len(m) == 0 {
		return nil, nil //nolint:nilnil // (nil, nil) is the documented "nothing to decode" contract, mirroring DecodeRetryConfig.
	}

	var cfg ComponentProSettings
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		ErrorUnused:      true,
	})
	if err != nil {
		return nil, errors.Join(ErrInvalidComponentProConfig, err)
	}
	if err := decoder.Decode(m); err != nil {
		return nil, errors.Join(ErrInvalidComponentProConfig, err)
	}
	return &cfg, nil
}
