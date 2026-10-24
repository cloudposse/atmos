package schema

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

// ErrInvalidTerraformFlagsConfig is returned when a flags config cannot be decoded from a stack manifest.
var ErrInvalidTerraformFlagsConfig = errors.New("invalid terraform flags configuration")

// terraformFlagsKnownKeys lists every key DecodeTerraformFlags recognizes. Kept in sync
// manually with TerraformFlags' mapstructure tags; see ValidateTerraformFlagsKeys.
var terraformFlagsKnownKeys = map[string]bool{
	"lock_timeout":     true,
	"lock":             true,
	"parallelism":      true,
	"refresh":          true,
	"compact_warnings": true,
}

// ValidateTerraformFlagsKeys reports an error if raw (typically a component's merged
// `flags:` section) contains a key DecodeTerraformFlags doesn't recognize — e.g. a typo
// like `lock_timout`. DecodeTerraformFlags itself ignores unrecognized keys (mapstructure
// has no ErrorUnused here) rather than erroring, because it's also called from
// internal/exec's tolerant stack-name-candidate search, which treats ANY error as "this
// isn't the right candidate, try the next one" and would silently swallow a real typo
// error into a confusing "component not found" message instead. Callers that want typo
// detection must call this separately, once the component is definitively resolved (e.g.
// at terraform argv-build time, alongside where an invalid lock_timeout duration is
// already caught) — see internal/exec/terraform_execute_helpers_args.go.
func ValidateTerraformFlagsKeys(raw any) error {
	m, ok := raw.(map[string]any)
	if !ok {
		// Non-map/nil is DecodeTerraformFlags' concern, not this function's.
		return nil
	}
	var unknown []string
	for k := range m {
		if !terraformFlagsKnownKeys[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%w: unrecognized key(s) %s (valid keys: lock_timeout, lock, parallelism, refresh, compact_warnings)",
		ErrInvalidTerraformFlagsConfig, strings.Join(unknown, ", "))
}

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
// An unrecognized key (e.g. a typo like `lock_timout`) is silently ignored here — see
// ValidateTerraformFlagsKeys for typo detection, which callers should invoke separately.
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
