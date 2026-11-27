package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultVersionCheckTimeout = 1000

var NotFound = errors.New("\n'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars." +
	"\nYou can download a sample config and adapt it to your requirements from " +
	"https://raw.githubusercontent.com/cloudposse/atmos/main/examples/quick-start-advanced/atmos.yaml")

// setProjectDefaults registers project conventions before loading any user configuration.
func setProjectDefaults(v *viper.Viper) {
	setYAMLStackDefaults(v)
	v.SetDefault("stacks.excluded_paths", []string{"**/_defaults.yaml"})
	v.SetDefault("workflows.base_path", "stacks/workflows")
	v.SetDefault("components.terraform.base_path", "components/terraform")
	v.SetDefault("components.terraform.deploy_run_init", true)
	v.SetDefault("components.terraform.auto_generate_backend_file", true)
	v.SetDefault("components.terraform.workspace.prefix_separator", "-")
	v.SetDefault("components.helmfile.base_path", "components/helmfile")
	v.SetDefault("components.packer.base_path", "components/packer")
	v.SetDefault("components.packer.command", "packer")
	v.SetDefault("components.kubernetes.base_path", "components/kubernetes")
	v.SetDefault("components.kubernetes.provider", "kubectl")
	v.SetDefault("components.container.base_path", "components/container")
	v.SetDefault("schemas.jsonschema.base_path", "stacks/schemas/jsonschema")
	v.SetDefault("schemas.opa.base_path", "stacks/schemas/opa")
	v.SetDefault("settings.list_merge_strategy", "replace")
	v.SetDefault("settings.terminal.unicode", true)
	v.SetDefault("settings.terminal.syntax_highlighting.enabled", true)
	v.SetDefault("settings.terminal.syntax_highlighting.formatter", "terminal")
	v.SetDefault("settings.terminal.syntax_highlighting.theme", "dracula")
	v.SetDefault("settings.terminal.syntax_highlighting.line_numbers", true)
	v.SetDefault("version.check.enabled", true)
	v.SetDefault("version.check.timeout", defaultVersionCheckTimeout)
	v.SetDefault("version.check.frequency", "daily")
	v.SetDefault("docs.generate.readme.base-dir", ".")
	v.SetDefault("docs.generate.readme.input", []any{"./README.yaml"})
	v.SetDefault("docs.generate.readme.template", "https://raw.githubusercontent.com/cloudposse/.github/5a599e3b929f871f333cb9681a721d26b237d8de/README.md.gotmpl")
	v.SetDefault("docs.generate.readme.terraform.source", "src/")
	v.SetDefault("docs.generate.readme.terraform.format", "markdown")
	v.SetDefault("docs.generate.readme.terraform.show_providers", false)
	v.SetDefault("docs.generate.readme.terraform.show_inputs", true)
	v.SetDefault("docs.generate.readme.terraform.show_outputs", true)
	v.SetDefault("docs.generate.readme.terraform.sort_by", "name")
	v.SetDefault("docs.generate.readme.terraform.indent_level", 2)
}

func setYAMLStackDefaults(v *viper.Viper) {
	v.SetDefault("stacks.base_path", "stacks")
	v.SetDefault("stacks.included_paths", []string{"orgs/**/*"})
}

// Discovery providers can supply the whole inventory. A default directory or glob
// must not imply that the project also requested YAML stack manifests.
func resolveDiscoveryStackDefaults(v *viper.Viper, config *schema.AtmosConfiguration) {
	if !config.HasComponentDiscovery() {
		return
	}
	v.SetDefault("stacks.base_path", nil)
	v.SetDefault("stacks.included_paths", nil)
	if v.IsSet("stacks.base_path") || v.IsSet("stacks.included_paths") {
		setYAMLStackDefaults(v)
		return
	}
	config.Stacks.BasePath = ""
	config.Stacks.IncludedPaths = nil
}
