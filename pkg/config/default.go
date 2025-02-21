package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
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
			},
			Helmfile: schema.Helmfile{
				BasePath:              "components/helmfile",
				KubeconfigPath:        "",
				HelmAwsProfilePattern: "{namespace}-{tenant}-gbl-{stage}-helm",
				ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
				UseEKS:                true,
			},
		},
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
			Terminal: schema.Terminal{
				MaxWidth: templates.GetTerminalWidth(),
				Pager:    true,
				Colors:   true,
				Unicode:  true,
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled:                true,
					Formatter:              "terminal",
					Theme:                  "dracula",
					HighlightedOutputPager: true,
					LineNumbers:            true,
					Wrap:                   false,
				},
			},
		},
		Workflows: schema.Workflows{
			BasePath: "stacks/workflows",
		},
		Logs: schema.Logs{
			File:  "/dev/stderr",
			Level: "Info",
		},
		Schemas: schema.Schemas{
			JsonSchema: schema.JsonSchema{
				BasePath: "stacks/schemas/jsonschema",
			},
			Opa: schema.Opa{
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
