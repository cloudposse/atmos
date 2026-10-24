// Package rc renders Atmos's `components.terraform.rc` section into Terraform's
// native CLI configuration (HCL) and exposes it to the terraform/tofu subprocess
// via TF_CLI_CONFIG_FILE. The section is a near-opaque passthrough: keys map
// directly to Terraform CLI-config directives (provider_installation, host,
// credentials, plugin_cache_dir, ...), so new directives need no Atmos schema
// change. Both terraform and tofu honor TF_CLI_CONFIG_FILE and the same grammar,
// so no binary-specific branching is required.
package rc

// CLI config environment variables.
//
// Terraform reads TF_CLI_CONFIG_FILE (and the legacy TERRAFORM_CONFIG). OpenTofu
// reads TOFU_CLI_CONFIG_FILE first, then falls back to TF_CLI_CONFIG_FILE. Atmos
// sets BOTH primary vars to the generated file so the config is honored regardless
// of which binary (terraform or tofu) runs, and treats any of them being set by the
// user as a signal to defer.
const (
	// EnvKeyTFCLIConfigFile is the Terraform CLI config env var (also honored by OpenTofu).
	EnvKeyTFCLIConfigFile = "TF_CLI_CONFIG_FILE"
	// EnvKeyTofuCLIConfigFile is the OpenTofu CLI config env var (takes precedence in tofu).
	EnvKeyTofuCLIConfigFile = "TOFU_CLI_CONFIG_FILE"
	// EnvKeyLegacyTerraformConfig is Terraform's deprecated CLI config env var, still honored.
	EnvKeyLegacyTerraformConfig = "TERRAFORM_CONFIG"
)

// cliConfigEnvKeys are the env vars Atmos sets to point both Terraform and OpenTofu
// at the generated CLI config file.
var cliConfigEnvKeys = []string{EnvKeyTFCLIConfigFile, EnvKeyTofuCLIConfigFile}

// userCLIConfigEnvKeys are the env vars whose presence means the user manages their
// own CLI config, so Atmos must defer.
var userCLIConfigEnvKeys = []string{EnvKeyTFCLIConfigFile, EnvKeyTofuCLIConfigFile, EnvKeyLegacyTerraformConfig}

// noop is a closer that does nothing, returned when RC management is disabled or
// when the user already manages the CLI config.
func noop() error { return nil }
