package output

import (
	"fmt"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// EnvironmentSetup handles environment variable configuration for terraform execution.
type EnvironmentSetup interface {
	// SetupEnvironment prepares environment variables for terraform execution.
	SetupEnvironment(config *ComponentConfig, authContext *schema.AuthContext) (map[string]string, error)
}

// defaultEnvironmentSetup is the default implementation of EnvironmentSetup.
type defaultEnvironmentSetup struct{}

// prohibitedEnvVars are environment variables that should not be passed to terraform-exec.
var prohibitedEnvVars = []string{
	"TF_CLI_ARGS",
	"TF_INPUT",
	"TF_IN_AUTOMATION",
	"TF_LOG",
	"TF_LOG_CORE",
	"TF_LOG_PATH",
	"TF_LOG_PROVIDER",
	"TF_REATTACH_PROVIDERS",
	"TF_APPEND_USER_AGENT",
	"TF_WORKSPACE",
	"TF_DISABLE_PLUGIN_TLS",
	"TF_SKIP_PROVIDER_VERIFY",
}

// prohibitedEnvVarPrefixes are prefixes for environment variables that should not be passed.
var prohibitedEnvVarPrefixes = []string{
	"TF_VAR_",
	"TF_CLI_ARGS_",
}

// SetupEnvironment prepares environment variables for terraform execution.
// It merges environment variables from:
// 1. Parent process (filtered to exclude terraform-exec prohibited vars)
// 2. Auth context (AWS credentials)
// 3. Component's env section.
func (s *defaultEnvironmentSetup) SetupEnvironment(config *ComponentConfig, authContext *schema.AuthContext) (map[string]string, error) {
	defer perf.Track(nil, "output.defaultEnvironmentSetup.SetupEnvironment")()

	// Get all environment variables from parent process (excluding prohibited vars).
	environMap := envpkg.EnvironToMapFiltered(prohibitedEnvVars, prohibitedEnvVarPrefixes)

	// Add auth-based environment variables if authContext is provided.
	if authContext != nil && authContext.AWS != nil {
		log.Debug("Adding auth-based environment variables",
			"profile", authContext.AWS.Profile,
			"credentials_file", authContext.AWS.CredentialsFile,
			"config_file", authContext.AWS.ConfigFile,
		)

		environMap = awsCloud.PrepareEnvironment(
			environMap,
			authContext.AWS.Profile,
			authContext.AWS.CredentialsFile,
			authContext.AWS.ConfigFile,
			authContext.AWS.Region,
		)
	}

	// Add/override environment variables from the component's env section.
	if len(config.Env) > 0 {
		log.Debug("Adding environment variables from component",
			"source", "env section",
			"count", len(config.Env),
		)
		for k, v := range config.Env {
			environMap[k] = fmt.Sprintf("%v", v)
		}
	}

	log.Debug("Resolved final environment variables",
		"count", len(environMap),
	)
	return environMap, nil
}
