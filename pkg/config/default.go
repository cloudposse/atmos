package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	NotFound = errors.New("\n'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars." +
		"\nYou can download a sample config and adapt it to your requirements from " +
		"https://raw.githubusercontent.com/cloudposse/atmos/main/examples/quick-start-advanced/atmos.yaml")

	defaultCliConfig = schema.AtmosConfiguration{
		Default:  true,
		BasePath: ".",
		Stacks: schema.Stacks{
			BasePath:    "stacks",
			NamePattern: "{tenant}-{environment}-{stage}",
			IncludedPaths: []string{
				"orgs/**/*",
			},
			ExcludedPaths: []string{
				"**/_defaults.yaml",
			},
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath:                "components/terraform",
				ApplyAutoApprove:        false,
				DeployRunInit:           true,
				InitRunReconfigure:      true,
				AutoGenerateBackendFile: true,
				AppendUserAgent:         fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version),
				PluginCache:             true, // Enabled by default for zero-config performance.
				PluginCacheDir:          "",   // Empty = use XDG default (~/.cache/atmos/terraform/plugins).
				Init: schema.TerraformInit{
					PassVars: false,
				},
				Plan: schema.TerraformPlan{
					SkipPlanfile: false,
				},
			},
			Helmfile: schema.Helmfile{
				BasePath:              "components/helmfile",
				KubeconfigPath:        "",
				HelmAwsProfilePattern: "{namespace}-{tenant}-gbl-{stage}-helm",
				ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
				UseEKS:                true,
			},
			Packer: schema.Packer{
				BasePath: "components/packer",
				Command:  "packer",
			},
		},
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
			Terminal: schema.Terminal{
				MaxWidth: templates.GetTerminalWidth(),
				Pager:    "less",
				Unicode:  true,
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled:     true,
					Formatter:   "terminal",
					Theme:       "dracula",
					LineNumbers: true,
					Wrap:        false,
				},
			},
		},
		Workflows: schema.Workflows{
			BasePath: "stacks/workflows",
		},
		Logs: schema.Logs{
			File:  "/dev/stderr",
			Level: "Warning",
		},
		Schemas: map[string]interface{}{
			"jsonschema": schema.ResourcePath{
				BasePath: "stacks/schemas/jsonschema",
			},
			"opa": schema.ResourcePath{
				BasePath: "stacks/schemas/opa",
			},
		},
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled:     true,
					Datasources: make(map[string]schema.TemplatesSettingsGomplateDatasource),
				},
			},
		},
		Initialized: true,
		Version: schema.Version{
			Check: schema.VersionCheck{
				Enabled:   true,
				Timeout:   1000,
				Frequency: "daily",
			},
		},
		Docs: schema.Docs{
			// Even if these fields are deprecated, initialize them with valid values.
			MaxWidth:   0,
			Pagination: false,
			Generate: map[string]schema.DocsGenerate{
				"readme": {
					BaseDir:  ".",
					Input:    []any{"./README.yaml"},
					Template: "https://raw.githubusercontent.com/cloudposse/.github/5a599e3b929f871f333cb9681a721d26b237d8de/README.md.gotmpl",
					Output:   "./README.md",
					Terraform: schema.TerraformDocsReadmeSettings{
						Source:        "src/",
						Enabled:       false,
						Format:        "markdown",
						ShowProviders: false,
						ShowInputs:    true,
						ShowOutputs:   true,
						SortBy:        "name",
						HideEmpty:     false,
						IndentLevel:   2,
					},
				},
			},
		},
	}
)

// mergeDefaultConfig merges the contents of defaultCliConfig into the
// current Viper instance if no other configuration file was located.
func mergeDefaultConfig(v *viper.Viper) error {
	j, err := json.Marshal(defaultCliConfig)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(j)
	return v.MergeConfig(reader)
}
