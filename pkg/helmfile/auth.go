package helmfile

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthInput contains all possible sources for AWS auth resolution.
type AuthInput struct {
	// Identity is the value from --identity flag (highest priority).
	Identity string
	// ProfilePattern is the helm_aws_profile_pattern from config (deprecated).
	ProfilePattern string
}

// AuthResult contains the resolved AWS authentication information.
type AuthResult struct {
	// UseIdentityAuth is true if identity-based authentication should be used.
	UseIdentityAuth bool
	// Profile is the AWS profile name (only set if UseIdentityAuth is false).
	Profile string
	// Source indicates where the auth came from.
	Source string
	// IsDeprecated is true if the source uses deprecated configuration.
	IsDeprecated bool
}

// ResolveAWSAuth determines the AWS authentication method with precedence:
// 1. The --identity flag (highest - uses identity system).
// 2. The helm_aws_profile_pattern (deprecated, logs warning).
// The context parameter must be non-nil when using helm_aws_profile_pattern.
func ResolveAWSAuth(input AuthInput, context *Context) (*AuthResult, error) {
	defer perf.Track(nil, "helmfile.ResolveAWSAuth")()

	// 1. --identity flag (highest priority).
	if input.Identity != "" {
		return &AuthResult{
			UseIdentityAuth: true,
			Profile:         "",
			Source:          "identity",
			IsDeprecated:    false,
		}, nil
	}

	// 2. helm_aws_profile_pattern (deprecated).
	if input.ProfilePattern != "" {
		if context == nil {
			return nil, fmt.Errorf("ResolveAWSAuth: context is required for helm_aws_profile_pattern expansion: %w", errUtils.ErrNilParam)
		}
		profile := cfg.ReplaceContextTokens(*context, input.ProfilePattern)
		return &AuthResult{
			UseIdentityAuth: false,
			Profile:         profile,
			Source:          "pattern",
			IsDeprecated:    true,
		}, nil
	}

	// No auth source configured.
	return nil, fmt.Errorf("%w: use --identity flag or configure helm_aws_profile_pattern",
		errUtils.ErrMissingHelmfileAuth)
}
