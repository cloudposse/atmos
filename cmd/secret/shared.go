package secret

import (
	"fmt"

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
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

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

	// Bridge auth credentials into identity-aware secret stores (lazy resolution).
	if authManager != nil {
		injectSecretStoreAuthResolver(&atmosConfig, authManager, scope)
	}

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

func injectSecretStoreAuthResolver(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager, scope secretScope) {
	resolver := authbridge.NewResolver(authManager, authManager.GetStackInfo())
	atmosConfig.Stores.SetAuthContextResolverWithDefaultIdentity(resolver, secretStoreDefaultIdentity(authManager, scope.Identity))
}

func secretStoreDefaultIdentity(authManager auth.AuthManager, requestedIdentity string) string {
	switch requestedIdentity {
	case cfg.IdentityFlagDisabledValue:
		return ""
	case "", cfg.IdentityFlagSelectValue:
		chain := authManager.GetChain()
		if len(chain) == 0 {
			return ""
		}
		return chain[len(chain)-1]
	default:
		return requestedIdentity
	}
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
