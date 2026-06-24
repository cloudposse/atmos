package secret

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// credentialFreeSkip lists the YAML functions that must NOT be evaluated during credential-free
// secret operations (enumeration and `secret list` without `--verify`). Listing only needs the
// static `secrets.vars` declarations (see secrets.ExtractDeclarations); it never needs a resolved
// value. These functions all perform an authenticated backend read, so with auth disabled they
// fall back to the default AWS credential chain and fail (e.g. the S3 backend assumes a role with
// no base credentials and the SDK ultimately dials the EC2 IMDS endpoint, which is unreachable on
// a workstation). Skipping them keeps listing genuinely credential-free: a skipped function leaves
// its raw string in place, which the declaration extractor ignores. `!secret` is included because
// retrieving secret values is a separate, explicit step.
func credentialFreeSkip() []string {
	// skipFunc compares against the tag with the leading "!" trimmed, so the skip tokens are bare.
	tags := []string{
		u.AtmosYamlFuncSecret,
		u.AtmosYamlFuncStore,
		u.AtmosYamlFuncStoreGet,
		u.AtmosYamlFuncTerraformOutput,
		u.AtmosYamlFuncTerraformState,
	}
	skip := make([]string, len(tags))
	for i, tag := range tags {
		skip[i] = strings.TrimPrefix(tag, "!")
	}
	return skip
}

// secretScope holds the parsed common flags for a secret subcommand.
type secretScope struct {
	Stack         string
	Component     string
	ComponentType string
	Identity      string
}

// parseFacets reads the persistent flags as optional facets (filters). Unlike parseScope it never
// errors on a missing --stack/--component, so enumeration commands (e.g. `secret list`) can list
// across all stacks/components when a facet is omitted and narrow when it is given.
func parseFacets(cmd *cobra.Command) (secretScope, error) {
	v := viper.GetViper()
	if err := secretParser.BindFlagsToViper(cmd, v); err != nil {
		return secretScope{}, err
	}
	return secretScope{
		Stack:         v.GetString(cfg.StackStr),
		Component:     v.GetString("component"),
		ComponentType: v.GetString("type"),
		Identity:      v.GetString(cfg.IdentityFlagName),
	}, nil
}

// parseScope reads the persistent flags for the current command, interactively prompting for a
// missing --stack/--component on a TTY via Atmos's built-in prompt (auto-disabled in CI and
// non-interactive shells). In a non-interactive context a missing flag falls back to the standard
// "required flag not provided" error, preserving today's pipeline behavior.
func parseScope(cmd *cobra.Command, args []string) (secretScope, error) {
	v := viper.GetViper()
	if err := secretParser.BindFlagsToViper(cmd, v); err != nil {
		return secretScope{}, err
	}
	scope := secretScope{
		Stack:         v.GetString(cfg.StackStr),
		Component:     v.GetString("component"),
		ComponentType: v.GetString("type"),
		Identity:      v.GetString(cfg.IdentityFlagName),
	}

	if scope.Stack == "" {
		chosen, err := flags.PromptForMissingRequired(cfg.StackStr, "Choose a stack", stackCompletion, cmd, args)
		if err != nil {
			return scope, err
		}
		scope.Stack = chosen
	}
	if scope.Stack == "" {
		return scope, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack is required for secret operations").
			WithHint("Specify a stack with --stack or -s").
			Err()
	}
	// Make the chosen stack visible to the component completion (it filters by --stack).
	v.Set(cfg.StackStr, scope.Stack)

	if scope.Component == "" {
		chosen, err := flags.PromptForMissingRequired("component", "Choose a component", componentCompletion, cmd, args)
		if err != nil {
			return scope, err
		}
		scope.Component = chosen
	}
	if scope.Component == "" {
		return scope, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--component is required for secret operations").
			WithHint("Specify a component with --component or -c").
			Err()
	}
	return scope, nil
}

// loadService initializes config + auth and returns a secrets.Service scoped to (stack,
// component). It resolves the component section with `!secret` skipped so declarations and
// includes resolve without retrieving secret values (which is a separate, explicit step).
func loadService(scope secretScope) (*secrets.Service, error) {
	defer perf.Track(nil, "secret.loadService")()

	svc, _, err := loadServiceAndConfig(scope)
	return svc, err
}

// loadServiceAndConfig is loadService plus the resolved AtmosConfiguration. Callers that
// need atmosConfig (e.g. `secret exec`/`secret shell`, which merge atmosConfig.Env into the
// child environment) use this variant; loadService wraps it for the common case.
func loadServiceAndConfig(scope secretScope) (*secrets.Service, *schema.AtmosConfiguration, error) {
	defer perf.Track(nil, "secret.loadServiceAndConfig")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: scope.Component,
		Stack:            scope.Stack,
	}, true)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	authManager, err := buildAuthManager(&atmosConfig, scope)
	if err != nil {
		return nil, nil, err
	}

	// Bridge auth credentials into identity-aware secret stores and cloud-KMS SOPS providers
	// (lazy resolution).
	injectSecretStoreAuthResolver(&atmosConfig, authManager, scope)

	section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            scope.Component,
		Stack:                scope.Stack,
		ComponentType:        scope.ComponentType,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 []string{"secret"}, // resolve includes etc., but never retrieve secrets here.
		AuthManager:          authManager,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load component config: %w", err)
	}

	return secrets.NewService(&atmosConfig, scope.Stack, scope.Component, section), &atmosConfig, nil
}

// loadServiceForList loads a scoped secrets service for `secret list`. It is credential-free by
// default: with verify=false it resolves the component config with auth disabled (no identity
// authentication, no store auth resolver) and builds the service from the declarations alone —
// SOPS status is still answered locally, remote-store status is reported as unknown. With
// verify=true it delegates to loadService, which authenticates and wires the store resolver so
// remote existence checks (Has) can run.
func loadServiceForList(scope secretScope, verify bool) (*secrets.Service, error) {
	defer perf.Track(nil, "secret.loadServiceForList")()

	if verify {
		return loadService(scope)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: scope.Component,
		Stack:            scope.Stack,
	}, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            scope.Component,
		Stack:                scope.Stack,
		ComponentType:        scope.ComponentType,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		// Auth is disabled here, so any credentialed read function (e.g. !terraform.state) would
		// fall back to the default AWS chain and fail. Skip them all — listing reads declarations only.
		Skip:         credentialFreeSkip(),
		AuthManager:  nil,
		AuthDisabled: true, // listing reads declarations only — no identity, no decryption.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load component config: %w", err)
	}

	return secrets.NewService(&atmosConfig, scope.Stack, scope.Component, section), nil
}

// buildAuthManager merges component auth and creates an authenticated manager for the scope.
func buildAuthManager(atmosConfig *schema.AtmosConfiguration, scope secretScope) (auth.AuthManager, error) {
	componentConfig, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            scope.Component,
		Stack:                scope.Stack,
		ComponentType:        scope.ComponentType,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		AuthManager:          nil,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load component config for auth: %w", err)
	}

	mergedAuthConfig, err := auth.MergeComponentAuthFromConfig(&atmosConfig.Auth, componentConfig, atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to merge component auth: %w", err)
	}

	authManager, err := auth.CreateAndAuthenticateManager(scope.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue)
	if err != nil {
		return nil, err
	}
	return authManager, nil
}

// injectSecretStoreAuthResolver wires the auth manager into atmosConfig as the store auth-context
// resolver (so secret stores resolve credentials lazily) and as the SecretsAuth context (so cloud-KMS
// SOPS providers authenticate KMS calls as the effective identity instead of requiring ambient
// credentials). It is a no-op when either argument is nil.
func injectSecretStoreAuthResolver(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager, scope secretScope) {
	if atmosConfig == nil || authManager == nil {
		return
	}

	resolver := authbridge.NewResolver(authManager, authManager.GetStackInfo())
	atmosConfig.Stores.SetAuthContextResolver(resolver)
	atmosConfig.SecretsAuth = &store.SecretsAuthContext{
		Resolver:        resolver,
		DefaultIdentity: secretsDefaultIdentity(scope, authManager),
	}
}

// secretsDefaultIdentity resolves the effective identity for cloud-KMS SOPS providers: an explicit
// `--identity`/`ATMOS_IDENTITY` (when not a select/disabled sentinel), otherwise the identity the
// auth manager actually authenticated (the last link in its chain). A per-provider
// `secrets.providers.<name>.identity` still takes precedence over this in the provider itself.
func secretsDefaultIdentity(scope secretScope, authManager auth.AuthManager) string {
	switch scope.Identity {
	case "", cfg.IdentityFlagSelectValue, cfg.IdentityFlagDisabledValue:
		// Fall back to the authenticated chain below.
	default:
		return scope.Identity
	}
	if chain := authManager.GetChain(); len(chain) > 0 {
		return chain[len(chain)-1]
	}
	return ""
}
