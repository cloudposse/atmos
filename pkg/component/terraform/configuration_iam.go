package terraform

import (
	"context"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	awsidentity "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func probeAndApplyIAM(ctx context.Context, runtime schema.ComponentConfigurationRuntime, info *schema.ConfigAndStacksInfo) (*schema.ComponentIAM, error) {
	if requestedIdentity(info) != "" {
		return nil, nil
	}
	probe, ok := runtime.(schema.ComponentConfigurationIAMProbe)
	if !ok {
		return nil, nil
	}
	iam, err := probe.ProbeIAM(ctx, info)
	if err != nil || iam == nil {
		return iam, err
	}
	if err := applyIAMRole(ctx, info, iam); err != nil {
		return nil, err
	}
	return iam, nil
}

func finishIAM(ctx context.Context, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration, staged *schema.ComponentIAM) error {
	if staged != nil {
		if !reflect.DeepEqual(staged, prepared.IAM) {
			return fmt.Errorf("%w: Terragrunt IAM settings changed between role and unit evaluation", errUtils.ErrPluginInvalidConfig)
		}
		return nil
	}
	// An operation the provider excludes never runs, so it must not need the unit's role.
	if excludedFromOperation(info, prepared) {
		return nil
	}
	return applyIAMRole(ctx, info, prepared.IAM)
}

// assumeInlineRole is replaced by tests that must not call STS.
var assumeInlineRole = awsidentity.AssumeInlineRole

// applyIAMRole assumes the role a component asks for, on top of the credentials of the invocation, and
// exports the session to every command the component runs, and to in-process source downloads, as Terragrunt
// fetches a unit's source with the assumed role. An Atmos identity that the user selected
// explicitly decides who the component runs as, so it replaces the role instead of being layered under it.
// The session lives only in the environment of this invocation: it is neither written to a file nor to a
// saved plan, and it is registered with the output masker.
func applyIAMRole(ctx context.Context, info *schema.ConfigAndStacksInfo, iam *schema.ComponentIAM) error {
	if iam == nil {
		return nil
	}
	if requested := requestedIdentity(info); requested != "" {
		ui.Infof("Component `%s` in stack `%s` runs as Atmos identity `%s`; its Terragrunt `iam_role` `%s` is not assumed.", info.ComponentFromArg, info.Stack, info.Identity, iam.Role)
		return nil
	}
	token, err := webIdentityToken(iam)
	if err != nil {
		return iamRoleError(info, iam, err)
	}
	role := awsidentity.InlineRole{Role: iam.Role, SessionName: iam.SessionName, Duration: time.Duration(iam.DurationSeconds) * time.Second, WebIdentityToken: token}
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	session, err := assumeInlineRole(ctx, role, append(slices.Clone(base), info.ComponentEnvList...))
	if err != nil {
		return iamRoleError(info, iam, err)
	}
	for _, key := range slices.Sorted(maps.Keys(session)) {
		info.ComponentEnvList = append(info.ComponentEnvList, key+"="+session[key])
	}
	info.IAMSession = session
	return nil
}

// webIdentityToken returns the token to assume the role with: read from its file when one is set,
// otherwise the literal value. A literal token is not stored with a saved plan, so applying such a
// plan fails here instead of assuming the role with the base credentials.
func webIdentityToken(iam *schema.ComponentIAM) (string, error) {
	token := iam.WebIdentityToken
	if iam.WebIdentityTokenFile != "" {
		content, err := os.ReadFile(iam.WebIdentityTokenFile)
		if err != nil {
			return "", fmt.Errorf("%w: web identity token file: %w", errUtils.ErrAuthenticationFailed, err)
		}
		token = strings.TrimSpace(string(content))
		if token == "" {
			return "", fmt.Errorf("%w: web identity token file %s is empty", errUtils.ErrAuthenticationFailed, iam.WebIdentityTokenFile)
		}
	}
	if iam.WebIdentityTokenLiteral && token == "" {
		return "", fmt.Errorf("%w: the web identity token is not stored with a saved plan", errUtils.ErrAuthenticationFailed)
	}
	return token, nil
}

// requestedIdentity returns what the user asked for with --identity or ATMOS_IDENTITY, or empty when no
// identity was requested. A default identity that authentication selected on its own is not a request, so
// the role is assumed on top of it. Disabling authentication requests nothing either.
func requestedIdentity(info *schema.ConfigAndStacksInfo) string {
	requested := info.Identity
	if info.RequestedIdentity != nil {
		requested = *info.RequestedIdentity
	}
	if requested == cfg.IdentityFlagDisabledValue {
		return ""
	}
	return requested
}

func iamRoleError(info *schema.ConfigAndStacksInfo, iam *schema.ComponentIAM, cause error) error {
	return errUtils.Build(errUtils.ErrAuthenticationFailed).
		WithCause(cause).
		WithExplanationf("Component `%s` in stack `%s` sets the Terragrunt `iam_role` `%s`, and Atmos could not assume it", info.ComponentFromArg, info.Stack, iam.Role).
		WithHint("Check that the credentials in use may assume the role, and that its trust policy allows them").
		WithHint("To run as an Atmos identity instead of assuming `iam_role`, select it with `--identity <name>`").
		WithHint("A web identity token must be a file path when a plan is saved and applied later").
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		Err()
}
