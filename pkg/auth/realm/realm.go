// Package realm provides credential realm computation and validation for authentication isolation.
// A realm defines an isolated credential namespace, preventing collisions when the same identity
// names are used across different repositories or customer environments.
package realm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// EnvVarName is the environment variable name for overriding the auth realm.
	EnvVarName = "ATMOS_AUTH_REALM"

	// SourceEnv indicates the realm was set via environment variable.
	SourceEnv = "env"

	// SourceConfig indicates the realm was set via atmos.yaml configuration.
	SourceConfig = "config"

	// SourceAuto indicates the realm was automatically computed from the config path.
	SourceAuto = "auto"

	// MaxLength is the maximum allowed length for a realm value.
	MaxLength = 64

	// hashLength is the number of characters to use from the SHA256 hash.
	hashLength = 8
)

// RealmInfo contains the computed realm value and its source.
type RealmInfo struct {
	// Value is the realm identifier used for credential isolation.
	Value string

	// Source indicates how the realm was determined: "env", "config", or "auto".
	Source string
}

// GetRealm computes the authentication realm with the following precedence:
//  1. ATMOS_AUTH_REALM environment variable (highest priority)
//  2. configRealm from atmos.yaml auth.realm configuration
//  3. SHA256 hash of cliConfigPath (first 8 characters) as automatic default
//
// Returns an error if an explicit realm value (env var or config) contains invalid characters.
// Auto-generated realms from path hashes are always valid since they only contain hex characters.
func GetRealm(configRealm, cliConfigPath string) (RealmInfo, error) {
	defer perf.Track(nil, "realm.GetRealm")()

	// Priority 1: Environment variable.
	if envRealm := os.Getenv(EnvVarName); envRealm != "" {
		if err := Validate(envRealm); err != nil {
			return RealmInfo{}, fmt.Errorf("%w: %s environment variable: %w", errUtils.ErrInvalidRealm, EnvVarName, err)
		}
		return RealmInfo{
			Value:  envRealm,
			Source: SourceEnv,
		}, nil
	}

	// Priority 2: Configuration file.
	if configRealm != "" {
		if err := Validate(configRealm); err != nil {
			return RealmInfo{}, fmt.Errorf("%w: auth.realm configuration: %w", errUtils.ErrInvalidRealm, err)
		}
		return RealmInfo{
			Value:  configRealm,
			Source: SourceConfig,
		}, nil
	}

	// Priority 3: Auto-generate from path hash.
	autoRealm := computeHash(cliConfigPath)
	return RealmInfo{
		Value:  autoRealm,
		Source: SourceAuto,
	}, nil
}

// computeHash generates an 8-character hex string from the SHA256 hash of the input.
// If the input is empty, returns a hash of an empty string for consistency.
func computeHash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])[:hashLength]
}

// validRealmPattern matches valid realm characters: lowercase alphanumeric, hyphen, underscore.
var validRealmPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// invalidCharsPattern finds characters that are not allowed in realm values.
var invalidCharsPattern = regexp.MustCompile(`[^a-z0-9_-]`)

// Validate checks that a realm value contains only allowed characters and follows all rules.
//
// Validation rules:
//   - Must contain only lowercase letters (a-z), digits (0-9), hyphens (-), and underscores (_)
//   - Must not be empty
//   - Must not exceed MaxLength (64) characters
//   - Must not start or end with hyphen or underscore
//   - Must not contain consecutive hyphens or underscores
//   - Must not contain path traversal sequences (/, \, ..)
//
// Returns an error describing the validation failure, or nil if valid.
func Validate(input string) error {
	defer perf.Track(nil, "realm.Validate")()

	// Check for empty input.
	if input == "" {
		return fmt.Errorf("realm cannot be empty")
	}

	// Check maximum length.
	if len(input) > MaxLength {
		return fmt.Errorf("realm exceeds maximum length of %d characters: got %d", MaxLength, len(input))
	}

	// Check for path traversal sequences first (security).
	if strings.Contains(input, "/") || strings.Contains(input, "\\") || strings.Contains(input, "..") {
		return fmt.Errorf("realm cannot contain path traversal characters (/, \\, ..)")
	}

	// Check for invalid characters.
	if !validRealmPattern.MatchString(input) {
		invalidChars := invalidCharsPattern.FindAllString(input, -1)
		// Deduplicate invalid characters.
		seen := make(map[string]bool)
		var unique []string
		for _, ch := range invalidChars {
			if !seen[ch] {
				seen[ch] = true
				unique = append(unique, ch)
			}
		}
		return fmt.Errorf("realm contains invalid characters: %v (only lowercase letters, numbers, hyphens, and underscores are allowed)", unique)
	}

	// Check start character.
	if strings.HasPrefix(input, "-") || strings.HasPrefix(input, "_") {
		return fmt.Errorf("realm cannot start with hyphen or underscore")
	}

	// Check end character.
	if strings.HasSuffix(input, "-") || strings.HasSuffix(input, "_") {
		return fmt.Errorf("realm cannot end with hyphen or underscore")
	}

	// Check for consecutive hyphens or underscores.
	if strings.Contains(input, "--") || strings.Contains(input, "__") ||
		strings.Contains(input, "-_") || strings.Contains(input, "_-") {
		return fmt.Errorf("realm cannot contain consecutive hyphens or underscores")
	}

	return nil
}

// SourceDescription returns a human-readable description of where the realm came from.
func (r RealmInfo) SourceDescription(cliConfigPath string) string {
	switch r.Source {
	case SourceEnv:
		return fmt.Sprintf("%s environment variable", EnvVarName)
	case SourceConfig:
		return "atmos.yaml (auth.realm)"
	case SourceAuto:
		if cliConfigPath != "" {
			return fmt.Sprintf("auto-generated from %s", cliConfigPath)
		}
		return "auto-generated"
	default:
		return r.Source
	}
}
