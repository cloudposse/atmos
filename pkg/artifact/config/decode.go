package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	keyRepositories = "repositories"
	keyPermissions  = "permissions"
)

// Decode strictly decodes a raw `artifacts` section (as parsed from YAML, with original key
// spelling) into the schema type. Unknown fields, mistyped values, and non-string permissions are
// errors, so `permissions: 0600` (which YAML reads as the integer 384) never silently becomes the
// wrong mode. A nil or empty section decodes to the zero value.
func Decode(raw map[string]any) (schema.ArtifactConfig, error) {
	defer perf.Track(nil, "artifactconfig.Decode")()

	var cfg schema.ArtifactConfig
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := checkPermissionTypes(raw); err != nil {
		return cfg, err
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      &cfg,
		TagName:     "mapstructure",
		ErrorUnused: true,
	})
	if err != nil {
		return cfg, fmt.Errorf("%w: decoder: %w", errUtils.ErrArtifactConfigInvalid, err)
	}
	if err := decoder.Decode(raw); err != nil {
		return cfg, fmt.Errorf("%w: artifacts: %w", errUtils.ErrArtifactConfigInvalid, err)
	}
	normalize(&cfg)
	return cfg, nil
}

// normalize turns empty collections into nil so that a value decoded from `trusted_keys: []`
// is identical to the same value after a YAML, JSON, or mapstructure round trip, all of which omit
// empty collections.
func normalize(cfg *schema.ArtifactConfig) {
	if len(cfg.Repositories) == 0 {
		cfg.Repositories = nil
	}
	normalizeVerification(cfg.Verification)
	for alias := range cfg.Repositories {
		repo := cfg.Repositories[alias]
		if len(repo.Options) == 0 {
			repo.Options = nil
		}
		if repo.Defaults != nil && repo.Defaults.Metadata != nil && len(repo.Defaults.Metadata.Labels) == 0 {
			repo.Defaults.Metadata.Labels = nil
		}
		normalizeVerification(repo.Verification)
		cfg.Repositories[alias] = repo
	}
}

func normalizeVerification(verification *schema.ArtifactVerificationConfig) {
	if verification == nil {
		return
	}
	if len(verification.TrustedKeys) == 0 {
		verification.TrustedKeys = nil
	}
	if len(verification.TrustedIdentities) == 0 {
		verification.TrustedIdentities = nil
	}
}

// checkPermissionTypes rejects any repository whose permissions value is not a string.
func checkPermissionTypes(raw map[string]any) error {
	repos, ok := lookupKey(raw, keyRepositories).(map[string]any)
	if !ok {
		return nil
	}
	aliases := make([]string, 0, len(repos))
	for alias := range repos {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	var errs []error
	for _, alias := range aliases {
		repo, ok := repos[alias].(map[string]any)
		if !ok {
			continue
		}
		value := lookupKey(repo, keyPermissions)
		if value == nil {
			continue
		}
		if _, isString := value.(string); !isString {
			errs = append(errs, fmt.Errorf("%w: repository %q: permissions must be a quoted octal string such as \"0600\", got %T %v (an unquoted 0600 is read by YAML as a different number)", errUtils.ErrArtifactConfigInvalid, alias, value, value))
		}
	}
	return errors.Join(errs...)
}

// lookupKey finds a key case-insensitively, matching the loader's case-insensitive key handling.
func lookupKey(m map[string]any, key string) any {
	if value, ok := m[key]; ok {
		return value
	}
	for candidate, value := range m {
		if strings.EqualFold(candidate, key) {
			return value
		}
	}
	return nil
}
