package output

import (
	"encoding/json"
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
		log.Debug(
			"Adding auth-based environment variables",
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
		log.Debug(
			"Adding environment variables from component",
			"source", "env section",
			"count", len(config.Env),
		)
		for k, v := range config.Env {
			environMap[k] = fmt.Sprintf("%v", v)
		}
	}

	// When components.terraform.init.pass_vars is enabled, export the component's
	// vars as TF_VAR_* so the internal `terraform init` (run while resolving
	// !terraform.output) can satisfy init-time variable dependencies — e.g. a
	// module whose `version`/`source` is bound to var.foo. The main terraform
	// path passes -var-file on init for this; the output executor runs init via
	// the terraform-exec library, which cannot pass a var-file to init, so vars
	// are forwarded through the environment instead. See issue #1412.
	if config.PassVars && len(config.Vars) > 0 {
		addTerraformVarsToEnv(environMap, config.Vars)
	}

	log.Debug(
		"Resolved final environment variables",
		"count", len(environMap),
	)
	return environMap, nil
}

// addTerraformVarsToEnv writes each component var into environMap as a TF_VAR_*
// entry. String values are passed through verbatim; all other types (bool,
// number, list, map) are JSON-encoded, which Terraform/OpenTofu accept for the
// corresponding variable types. Existing TF_VAR_* entries are not overwritten so
// that an explicit env-section override always wins.
func addTerraformVarsToEnv(environMap map[string]string, vars map[string]any) {
	for name, value := range vars {
		key := "TF_VAR_" + name
		if _, exists := environMap[key]; exists {
			continue
		}
		environMap[key] = encodeTerraformVarValue(value)
	}
}

// encodeTerraformVarValue renders a single var value for a TF_VAR_* environment
// variable. Strings are used as-is; everything else is JSON-encoded, falling back
// to fmt formatting if JSON marshaling fails.
func encodeTerraformVarValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
