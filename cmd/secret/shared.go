package secret

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
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

// parseScope reads the persistent flags for the current command.
func parseScope(cmd *cobra.Command) (secretScope, error) {
	v := viper.GetViper()
	if err := secretParser.BindFlagsToViper(cmd, v); err != nil {
		return secretScope{}, err
	}
	scope := secretScope{
		Stack:         v.GetString("stack"),
		Component:     v.GetString("component"),
		ComponentType: v.GetString("type"),
		Identity:      v.GetString(cfg.IdentityFlagName),
	}
	if scope.Stack == "" {
		return scope, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack is required for secret operations").
			WithHint("Specify a stack with --stack or -s").
			Err()
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

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: scope.Component,
		Stack:            scope.Stack,
	}, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	authManager, err := buildAuthManager(&atmosConfig, scope)
	if err != nil {
		return nil, err
	}

	// Bridge auth credentials into identity-aware secret stores (lazy resolution).
	if authManager != nil {
		resolver := authbridge.NewResolver(authManager, authManager.GetStackInfo())
		atmosConfig.Stores.SetAuthContextResolver(resolver)
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
