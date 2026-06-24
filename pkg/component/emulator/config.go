// Package emulator implements the `emulator` component kind: a stack-scoped,
// long-running cloud-API emulator container (Floci, MiniStack, k3s, OpenBao, …).
// It reuses the container component lifecycle (ComponentType "emulator") and the
// emulator driver/profile registry in pkg/emulator.
package emulator

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultBasePath is the conventional location for emulator component assets.
const defaultBasePath = "components/emulator"

// Config is the global configuration for the emulator component kind, stored
// under `components.emulator` in atmos.yaml and read via the Plugins map.
type Config struct {
	// BasePath is the base directory for emulator component assets.
	BasePath string `mapstructure:"base_path"`
}

// DefaultConfig returns the default global emulator component configuration.
func DefaultConfig() Config {
	defer perf.Track(nil, "componentemulator.DefaultConfig")()

	return Config{BasePath: defaultBasePath}
}

// parseConfig decodes a raw global-config value (from the Plugins map) into a
// typed Config.
func parseConfig(raw any) (Config, error) {
	var config Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &config,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
	})
	if err != nil {
		return Config{}, fmt.Errorf("%w: create config decoder: %w", errUtils.ErrComponentConfigInvalid, err)
	}
	if err := decoder.Decode(raw); err != nil {
		return Config{}, fmt.Errorf("%w: decode emulator config: %w", errUtils.ErrComponentConfigInvalid, err)
	}
	return config, nil
}
