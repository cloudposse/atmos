package exec

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	"github.com/cloudposse/atmos/pkg/ui"
)

// authManagerFactory creates an AuthManager from the given parameters.
// Package-level variable to allow test injection.
var authManagerFactory = func(identity string, authConfig schema.AuthConfig, flagSelectValue string, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error) {
	mergedAuthConfig := auth.CopyGlobalAuthConfig(&authConfig)
	return auth.CreateAndAuthenticateManagerWithAtmosConfig(identity, mergedAuthConfig, flagSelectValue, atmosConfig)
}

// ExecuteTerraformQuery executes `atmos terraform <command> --query <yq-expression --stack <stack>`.
func ExecuteTerraformQuery(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQuery")()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	// Always use debug level for internal logging.
	logFunc := log.Debug

	// Create auth manager for YAML function processing during stack description.
	// Without this, YAML functions like !terraform.state fail when using --all
	// because they cannot access authenticated credentials (e.g., AWS SSO).
	// Fixes: https://github.com/cloudposse/atmos/issues/2081
	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		return err
	}

	// Inject auth resolver into identity-aware stores so they can lazily resolve
	// credentials on first access. This bridges the store system with the auth system.
	if authManager != nil {
		resolver := authbridge.NewResolver(authManager, info)
		atmosConfig.Stores.SetAuthContextResolver(resolver)
	}

	stacks, err := ExecuteDescribeStacks(
		&atmosConfig,
		info.Stack,
		info.Components,
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
	)
	if err != nil {
		return err
	}

	// Track how many components were processed.
	processedCount := 0

	err = walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		processed, err := processTerraformComponent(&atmosConfig, info, stackName, componentName, componentSection, logFunc)
		if processed {
			processedCount++
		}
		return err
	})
	if err != nil {
		return err
	}

	// Show success message if no components matched the criteria.
	if processedCount == 0 {
		ui.Success("No components matched")
	}

	return nil
}

// createQueryAuthManager creates an AuthManager for multi-component execution paths.
// This is needed so that YAML functions (e.g., !terraform.state) can use authenticated
// credentials when ExecuteDescribeStacks processes stack configurations.
// Returns nil AuthManager (no error) if authentication is not configured.
func createQueryAuthManager(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "exec.createQueryAuthManager")()

	authManager, err := authManagerFactory(
		info.Identity, atmosConfig.Auth, cfg.IdentityFlagSelectValue, atmosConfig,
	)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		return nil, fmt.Errorf("create query auth manager: %w", err)
	}

	// Store AuthManager in info so downstream operations can reuse it.
	if authManager != nil {
		info.AuthManager = authManager
		log.Debug("Created AuthManager for multi-component execution")
	}

	return authManager, nil
}
