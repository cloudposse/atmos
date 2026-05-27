package migrate

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

func TestMigrateCommandRegistered(t *testing.T) {
	cmd := GetCommand(Options{ParentCommand: &cobra.Command{Use: "terraform"}})
	require.NotNil(t, cmd)
	assert.Equal(t, "migrate", cmd.Use)

	var foundPlan, foundApply, foundList bool
	for _, subCmd := range cmd.Commands() {
		switch subCmd.Use {
		case "plan [component]":
			foundPlan = true
		case "apply [component]":
			foundApply = true
		case "list [component]":
			foundList = true
		}
	}
	assert.True(t, foundPlan)
	assert.True(t, foundApply)
	assert.True(t, foundList)
}

func TestMigrateParserFlags(t *testing.T) {
	require.NotNil(t, migrateParser)
	registry := migrateParser.Registry()
	assert.True(t, registry.Has("migration"))
	assert.True(t, registry.Has("tfmigrate-config"))
	assert.True(t, registry.Has("backend-config"))
	assert.True(t, registry.Has("all"))
	assert.True(t, registry.Has("affected"))
}

func TestMigrateParserViperBinding(t *testing.T) {
	v := viper.New()
	require.NoError(t, migrateParser.BindToViper(v))

	v.Set("migration", "migrations/001.hcl")
	v.Set("tfmigrate-config", ".tfmigrate.hcl")
	v.Set("backend-config", []string{"bucket=state"})

	assert.Equal(t, "migrations/001.hcl", v.GetString("migration"))
	assert.Equal(t, ".tfmigrate.hcl", v.GetString("tfmigrate-config"))
	assert.Equal(t, []string{"bucket=state"}, v.GetStringSlice("backend-config"))
}

func TestParseTerraformMigratePreservesIdentity(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		args     []string
		env      string
		expected string
	}{
		{
			name:     "plan with explicit identity",
			action:   tfmigrate.ActionPlan,
			args:     []string{"vpc", "--stack", "dev", "--identity", "aws-dev"},
			expected: "aws-dev",
		},
		{
			name:     "apply with explicit identity",
			action:   tfmigrate.ActionApply,
			args:     []string{"vpc", "--stack", "dev", "--identity=aws-prod"},
			expected: "aws-prod",
		},
		{
			name:     "plan with identity from environment",
			action:   tfmigrate.ActionPlan,
			args:     []string{"vpc", "--stack", "dev"},
			env:      "env-identity",
			expected: "env-identity",
		},
		{
			name:     "apply with identity from environment",
			action:   tfmigrate.ActionApply,
			args:     []string{"vpc", "--stack", "dev"},
			env:      "env-identity",
			expected: "env-identity",
		},
		{
			name:     "identity false disables auth",
			action:   tfmigrate.ActionPlan,
			args:     []string{"vpc", "--stack", "dev", "--identity=false"},
			expected: cfg.IdentityFlagDisabledValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := setupMigrateParseTest(t)
			if tt.env != "" {
				t.Setenv("ATMOS_IDENTITY", tt.env)
			}
			cmd := newMigrateActionTestCommand(parent, tt.action)

			info, opts, err := parseTerraformMigrate(cmd, tt.args, tt.action)
			require.NoError(t, err)

			assert.Equal(t, tt.action, opts.Action)
			assert.Equal(t, "vpc", info.ComponentFromArg)
			assert.Equal(t, "dev", info.Stack)
			assert.Equal(t, tt.expected, info.Identity)
		})
	}
}

func TestParseTerraformMigrateListArgsPreservesIdentity(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      string
		expected string
	}{
		{
			name:     "explicit identity",
			args:     []string{"vpc", "--stack", "dev", "--identity", "aws-dev"},
			expected: "aws-dev",
		},
		{
			name:     "identity from environment",
			args:     []string{"vpc", "--stack", "dev"},
			env:      "env-identity",
			expected: "env-identity",
		},
		{
			name:     "identity false disables auth",
			args:     []string{"vpc", "--stack", "dev", "--identity=false"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "identity without value uses interactive selector",
			args:     []string{"vpc", "--stack", "dev", "--identity"},
			expected: cfg.IdentityFlagSelectValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMigrateParseTest(t)
			if tt.env != "" {
				t.Setenv("ATMOS_IDENTITY", tt.env)
			}

			info, err := parseTerraformMigrateListArgs(tt.args)
			require.NoError(t, err)

			assert.Equal(t, "dev", info.Stack)
			assert.Equal(t, []string{"vpc"}, info.Components)
			assert.Equal(t, tt.expected, info.Identity)
		})
	}
}

func TestMigrateListParserFlags(t *testing.T) {
	require.NotNil(t, migrateListParser)
	registry := migrateListParser.Registry()
	assert.True(t, registry.Has("format"))
	assert.True(t, registry.Has("columns"))
	assert.True(t, registry.Has("sort"))
	assert.True(t, registry.Has("delimiter"))
	assert.True(t, registry.Has("all"))
}

func TestTfmigrateListRow(t *testing.T) {
	row := tfmigrateListRow("plat-ue2-dev", "s3-bucket", map[string]any{
		cfg.WorkspaceSectionName:   "prod",
		cfg.BackendTypeSectionName: cfg.BackendTypeS3,
		cfg.BackendSectionName: map[string]any{
			"bucket": "tfstate-bucket",
			"region": "us-east-1",
			"assume_role": map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/tfstate",
			},
		},
		cfg.HooksSectionName: map[string]any{
			"state-migration": map[string]any{
				"kind":      "tfmigrate",
				"mode":      "apply",
				"migration": "migrations/001.hcl",
				"config":    ".tfmigrate.hcl",
			},
		},
	})

	assert.Equal(t, "s3-bucket", row["component"])
	assert.Equal(t, "plat-ue2-dev", row["stack"])
	assert.Equal(t, "prod", row["workspace"])
	assert.Equal(t, "state-migration", row["hook"])
	assert.Equal(t, "apply", row["mode"])
	assert.Equal(t, "migrations/001.hcl", row["migration"])
	assert.Equal(t, ".tfmigrate.hcl", row["config"])
	assert.Equal(t, cfg.BackendTypeS3, row["history_storage"])
	assert.Equal(t, "tfstate-bucket", row["history_bucket"])
	assert.Equal(t, "tfmigrate/plat-ue2-dev/s3-bucket/prod/history.json", row["history_key"])
	assert.Equal(t, "arn:aws:iam::123456789012:role/tfstate", row["history_role_arn"])
	assert.Equal(t, true, row["tfmigrate_enabled"])
}

func TestTfmigrateListColumns(t *testing.T) {
	columns := tfmigrateListColumns([]string{"component", "History Key=history_key"})
	require.Len(t, columns, 2)
	assert.Equal(t, "Component", columns[0].Name)
	assert.Equal(t, "{{ .component }}", columns[0].Value)
	assert.Equal(t, "History Key", columns[1].Name)
	assert.Equal(t, "{{ .history_key }}", columns[1].Value)
}

func TestBuildTfmigrateEnvAddsHistoryNamespace(t *testing.T) {
	env, err := buildTfmigrateEnv(&schema.AtmosConfiguration{BasePath: "."}, &schema.ConfigAndStacksInfo{
		Command:              "tofu",
		Stack:                "plat-ue2-dev",
		FinalComponent:       "s3-bucket",
		TerraformWorkspace:   "prod",
		ComponentBackendType: cfg.BackendTypeS3,
		ComponentBackendSection: map[string]any{
			"bucket": "tfstate-bucket",
			"region": "us-east-1",
		},
		ComponentEnvSection: map[string]any{
			"AWS_REGION": "us-east-1",
		},
	}, nil)
	require.NoError(t, err)

	assert.Contains(t, env, "AWS_REGION=us-east-1")
	assert.Contains(t, env, "ATMOS_STACK=plat-ue2-dev")
	assert.Contains(t, env, "ATMOS_COMPONENT=s3-bucket")
	assert.Contains(t, env, "ATMOS_TERRAFORM_WORKSPACE=prod")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_KEY=tfmigrate/plat-ue2-dev/s3-bucket/prod/history.json")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_STORAGE=s3")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_BUCKET=tfstate-bucket")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_REGION=us-east-1")
	assert.Contains(t, env, "TFMIGRATE_EXEC_PATH=tofu")
}

func setupMigrateParseTest(t *testing.T) *cobra.Command {
	t.Helper()

	previousParentCommand := parentCommand
	previousTerraformParser := terraformParser
	viper.Reset()
	t.Setenv("ATMOS_IDENTITY", "")

	parent := &cobra.Command{
		Use: "terraform",
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
	}
	parser := flags.NewStandardParser(
		flags.WithCommonFlags(),
		shared.WithBackendExecutionFlags(),
		flags.WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing"),
		flags.WithBoolFlag("process-functions", "", true, "Enable/disable YAML functions processing"),
		flags.WithStringSliceFlag("skip", "", nil, "Skip YAML functions"),
		flags.WithBoolFlag("skip-init", "", false, "Skip terraform init before running command"),
		flags.WithBoolFlag("init-pass-vars", "", false, "Pass generated varfile to init"),
		flags.WithStringFlag("planfile", "", "", "Path to a Terraform plan file"),
		flags.WithBoolFlag("skip-planfile", "", false, "Skip planfile generation"),
		flags.WithBoolFlag("deploy-run-init", "", false, "Run init during deploy"),
		flags.WithBoolFlag("verify-plan", "", false, "Verify plan before apply"),
		flags.WithStringFlag("query", "q", "", "YQ component filter"),
		flags.WithStringSliceFlag("components", "", nil, "Component filters"),
		flags.WithBoolFlag("upload-status", "", false, "Upload status"),
	)
	parser.RegisterPersistentFlags(parent)
	registerProcessCommandLineLocalFlags(parent)
	require.NoError(t, parser.BindToViper(viper.GetViper()))

	parentCommand = parent
	terraformParser = parser

	t.Cleanup(func() {
		parentCommand = previousParentCommand
		terraformParser = previousTerraformParser
		viper.Reset()
	})

	return parent
}

func registerProcessCommandLineLocalFlags(cmd *cobra.Command) {
	cmd.Flags().String("base-path", "", "")
	cmd.Flags().StringSlice("config", nil, "")
	cmd.Flags().StringSlice("config-path", nil, "")
	cmd.Flags().StringSlice("profile", nil, "")
	cmd.Flags().StringP("stack", "s", "", "")
	cmd.Flags().StringP(cfg.IdentityFlagName, cfg.IdentityFlagShortName, "", "")
	cmd.Flags().Lookup(cfg.IdentityFlagName).NoOptDefVal = cfg.IdentityFlagSelectValue
}

func newMigrateActionTestCommand(parent *cobra.Command, action string) *cobra.Command {
	cmd := &cobra.Command{Use: action + " [component]"}
	migrateParser.RegisterFlags(cmd)
	parent.AddCommand(cmd)
	return cmd
}
