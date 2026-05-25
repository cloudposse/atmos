package atmos

import (
	_ "embed"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

//go:embed builtin.yaml
var builtinYAML []byte

// builtinConfig is the on-disk shape of builtin.yaml.
type builtinConfig struct {
	Tools map[string]any `yaml:"tools"`
}

// NewBuiltinRegistry returns the Atmos-curated registry of tool overrides
// shipped with the binary. The data is loaded from the embedded
// builtin.yaml so it stays editable as YAML — diffs are reviewable and
// new entries don't require Go code changes.
//
// This registry is registered by the toolchain registry loader at a
// priority above the default Aqua registry, so tools the upstream
// registry doesn't handle well (e.g., KICS) install cleanly without users
// having to add overrides to their atmos.yaml. Users can still configure
// a higher-priority registry to override a built-in if needed.
func NewBuiltinRegistry() (*AtmosRegistry, error) {
	defer perf.Track(nil, "atmos.NewBuiltinRegistry")()

	var cfg builtinConfig
	if err := yaml.Unmarshal(builtinYAML, &cfg); err != nil {
		return nil, errUtils.Build(errUtils.ErrParseFile).
			WithCause(err).
			WithExplanation("Failed to parse builtin atmos registry").
			Err()
	}
	return NewAtmosRegistry(cfg.Tools)
}
