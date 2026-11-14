package aws

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// inlineRoleIdentityName names the transient identity behind a role that a component asks for.
const inlineRoleIdentityName = "component-iam-role"

// InlineRole is an IAM role that a component, rather than the Atmos configuration, asks to assume. It
// reuses the aws/assume-role identity, so STS calls, endpoints, and error reporting behave the same way.
type InlineRole struct {
	// Role is the role ARN.
	Role string
	// SessionName is the STS role session name. It defaults to a generated name.
	SessionName string
	// Duration is the session length. Zero selects the STS default.
	Duration time.Duration
	// WebIdentityToken selects AssumeRoleWithWebIdentity, which needs no starting credentials.
	WebIdentityToken string
}

// AssumeInlineRole assumes the role on top of the AWS credentials found in environ, a list of KEY=VALUE
// entries in which the last entry of a key wins. The starting credentials come from environ alone, in the
// order of the SDK default chain: static keys, a web identity token file, the profile and files that environ
// names, container credentials, then instance metadata. The process environment is neither read nor changed. It returns the
// environment variables that make the new session the credentials of a child process, and registers the
// secrets with the output masker.
func AssumeInlineRole(ctx context.Context, role InlineRole, environ []string) (map[string]string, error) {
	defer perf.Track(nil, "aws.AssumeInlineRole")()

	env := environMap(environ)
	identity, err := NewAssumeRoleIdentity(inlineRoleIdentityName, inlineRoleConfig(role, env))
	if err != nil {
		return nil, err
	}
	var base types.ICredentials
	if role.WebIdentityToken != "" {
		base = &types.OIDCCredentials{Token: role.WebIdentityToken}
	} else if base, err = baseCredentials(ctx, env); err != nil {
		return nil, err
	}
	assumed, err := identity.Authenticate(ctx, base)
	if err != nil {
		return nil, err
	}
	session, ok := assumed.(*types.AWSCredentials)
	if !ok || session.AccessKeyID == "" {
		return nil, fmt.Errorf("%w: STS returned no credentials for role %s", errUtils.ErrAuthenticationFailed, role.Role)
	}
	atmosio.RegisterSecretValue(session.AccessKeyID)
	atmosio.RegisterSecretValue(session.SecretAccessKey)
	atmosio.RegisterSecretValue(session.SessionToken)
	result := map[string]string{
		"AWS_ACCESS_KEY_ID":     session.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": session.SecretAccessKey,
		"AWS_SESSION_TOKEN":     session.SessionToken,
		"AWS_SECURITY_TOKEN":    session.SessionToken,
		// Emptied so that a profile never competes with the session for the child process.
		"AWS_PROFILE": "",
	}
	// The region is left to the invocation's own settings: the session does not choose one.
	return result, nil
}

// inlineRoleConfig describes the transient aws/assume-role identity. An STS endpoint in the environment,
// as emulators export, directs the call.
func inlineRoleConfig(role InlineRole, env map[string]string) *schema.Identity {
	principal := map[string]any{"assume_role": role.Role}
	if role.SessionName != "" {
		principal[principalSessionNameKey] = role.SessionName
	}
	if role.Duration > 0 {
		principal[principalDurationKey] = fmt.Sprintf("%ds", int64(role.Duration.Seconds()))
	}
	if region := cmp.Or(env["AWS_REGION"], env["AWS_DEFAULT_REGION"]); region != "" {
		principal["region"] = region
	}
	identity := &schema.Identity{Kind: "aws/assume-role", Principal: principal}
	if endpoint := cmp.Or(env["AWS_ENDPOINT_URL_STS"], env["AWS_ENDPOINT_URL"]); endpoint != "" {
		identity.Spec = map[string]any{"endpoint_url": endpoint}
	}
	return identity
}

func environMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	return env
}
