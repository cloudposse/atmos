package user

import (
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	authUtils "github.com/cloudposse/atmos/pkg/auth/utils"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// awsUserIdentityInfo holds extracted credential information from YAML config.
type awsUserIdentityInfo struct {
	AccessKeyID     string
	SecretAccessKey string
	MfaArn          string
	SessionDuration string
	AllInYAML       bool
}

// selectAWSUserIdentities filters identities to find aws/user types.
func selectAWSUserIdentities(identities map[string]schema.Identity) ([]string, string, error) {
	defer perf.Track(nil, "auth.user.selectAWSUserIdentities")()

	if identities == nil {
		return nil, "", fmt.Errorf("%w: no auth identities configured in atmos.yaml", errUtils.ErrInvalidAuthConfig)
	}

	var selectable []string
	defaultChoice := ""

	for ident := range identities {
		identity := identities[ident]
		if identity.Kind == "aws/user" {
			selectable = append(selectable, ident)
			if identity.Default && defaultChoice == "" {
				defaultChoice = ident
			}
		}
	}

	if len(selectable) == 0 {
		return nil, "", fmt.Errorf("%w: no identities configured for provider type 'aws/user'. Define one under auth.identities in atmos.yaml", errUtils.ErrInvalidAuthConfig)
	}

	return selectable, defaultChoice, nil
}

// extractAWSUserInfo extracts credential information from YAML identity config.
func extractAWSUserInfo(identity schema.Identity) awsUserIdentityInfo {
	defer perf.Track(nil, "auth.user.extractAWSUserInfo")()

	var info awsUserIdentityInfo

	// Extract credentials from YAML.
	if identity.Credentials != nil {
		if accessKey, ok := identity.Credentials["access_key_id"].(string); ok {
			info.AccessKeyID = accessKey
		}
		if secretKey, ok := identity.Credentials["secret_access_key"].(string); ok {
			info.SecretAccessKey = secretKey
		}
		if mfa, ok := identity.Credentials["mfa_arn"].(string); ok {
			info.MfaArn = mfa
		}
	}

	// Check if session duration is configured at identity level.
	if identity.Session != nil && identity.Session.Duration != "" {
		info.SessionDuration = identity.Session.Duration
	}

	// Check if all required credentials are managed by YAML.
	info.AllInYAML = info.AccessKeyID != "" && info.SecretAccessKey != ""

	return info
}

// formFieldConfig defines configuration for building credential form fields.
type formFieldConfig struct {
	YAMLValue      string
	InputTitle     string
	NoteTitle      string
	NoteDesc       string
	IsPassword     bool
	IsOptional     bool
	DefaultValue   string
	ValidateFunc   func(string) error
	DescriptionMsg string
}

// buildCredentialFormField creates appropriate form field based on whether value is in YAML.
func buildCredentialFormField(cfg formFieldConfig, valuePtr *string) huh.Field {
	defer perf.Track(nil, "auth.user.buildCredentialFormField")()

	if cfg.YAMLValue != "" {
		// Value is in YAML config - show as informational note.
		*valuePtr = cfg.YAMLValue
		return huh.NewNote().
			Title(cfg.NoteTitle).
			Description(cfg.NoteDesc)
	}

	// Value not in YAML - create input field.
	if cfg.DefaultValue != "" {
		*valuePtr = cfg.DefaultValue
	}

	input := huh.NewInput().
		Title(cfg.InputTitle).
		Value(valuePtr)

	if cfg.DescriptionMsg != "" {
		input = input.Description(cfg.DescriptionMsg)
	}

	if cfg.IsPassword {
		input = input.EchoMode(huh.EchoModePassword)
	}

	if cfg.ValidateFunc != nil {
		input = input.Validate(cfg.ValidateFunc)
	} else if !cfg.IsOptional {
		// Default validation: required field.
		input = input.Validate(func(s string) error {
			if s == "" {
				return errUtils.ErrMissingInput
			}
			return nil
		})
	}

	return input
}

// validateSessionDuration validates session duration format.
func validateSessionDuration(s string) error {
	defer perf.Track(nil, "auth.user.validateSessionDuration")()

	if s == "" {
		return nil // Optional field.
	}
	// Validate duration format using flexible parser.
	_, err := authUtils.ParseDurationFlexible(s)
	if err != nil {
		return fmt.Errorf("invalid duration format (use: 3600, 1h, 12h, 1d, etc.)")
	}
	return nil
}
