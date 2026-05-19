package schema

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

// ErrInvalidRetryConfig is returned when a retry config cannot be decoded from a stack manifest.
var ErrInvalidRetryConfig = errors.New("invalid retry configuration")

// DecodeRetryConfig decodes a map[string]any (typically read from a stack manifest's
// `components.<type>.<name>.retry:` section) into a *RetryConfig.
//
// Returns (nil, nil) when the input is nil or empty so callers can write:
//
//	cfg, err := schema.DecodeRetryConfig(componentSection["retry"])
//
// Duration fields like "2s" are parsed via mapstructure's StringToTimeDurationHookFunc.
// Errors are joined with ErrInvalidRetryConfig for predictable error checking.
func DecodeRetryConfig(raw any) (*RetryConfig, error) {
	if raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected map, got %T", ErrInvalidRetryConfig, raw)
	}
	if len(m) == 0 {
		return nil, nil
	}

	var cfg RetryConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return nil, errors.Join(ErrInvalidRetryConfig, err)
	}
	if err := decoder.Decode(m); err != nil {
		return nil, errors.Join(ErrInvalidRetryConfig, err)
	}
	return &cfg, nil
}
