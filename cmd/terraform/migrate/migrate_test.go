package migrate

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
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

func TestRunTerraformMigratePlanDryRunFixture(t *testing.T) {
	parent := setupMigrateParseTest(t)
	cmd := newMigrateActionTestCommand(parent, tfmigrate.ActionPlan)
	viper.GetViper().Set("dry-run", true)
	viper.GetViper().Set("skip-init", true)

	err := runTerraformMigrate(cmd, []string{
		"service",
		"--stack", "deploy/test",
		"--config-path", "../../../examples/hooks-tfmigrate",
		"--identity=false",
	}, tfmigrate.ActionPlan)

	require.NoError(t, err)
}

func TestRunTerraformMigrateListFixtureRenders(t *testing.T) {
	setupMigrateParseTest(t)
	cmd := &cobra.Command{Use: "list [component]"}
	migrateListParser.RegisterFlags(cmd)
	viper.GetViper().Set(migrateListFlagFormat, "json")
	viper.GetViper().Set(migrateListFlagColumns, []string{"component", "stack", "hook"})

	output := captureDataOutput(t, func() {
		err := runTerraformMigrateList(cmd, []string{
			"service",
			"--stack", "deploy/test",
			"--config-path", "../../../examples/hooks-tfmigrate",
			"--identity=false",
		})
		require.NoError(t, err)
	})

	assert.Contains(t, output, `"Component": "service"`)
	assert.Contains(t, output, `"Stack": "test"`)
	assert.Contains(t, output, `"Hook": "state-migration"`)
}

func TestRenderTfmigrateListFormatsRows(t *testing.T) {
	rows := []map[string]any{
		tfmigrateListRow("deploy/test", "service", map[string]any{
			cfg.WorkspaceSectionName:   "default",
			cfg.BackendTypeSectionName: cfg.BackendTypeLocal,
			cfg.HooksSectionName: map[string]any{
				"state-migration": map[string]any{
					"kind": tfmigrate.Command,
				},
			},
		}),
	}

	output := captureDataOutput(t, func() {
		err := renderTfmigrateList(rows, migrateListRenderOptions{
			Format:  "csv",
			Columns: []string{"component", "stack"},
			Sort:    "component:desc",
		})
		require.NoError(t, err)
	})

	assert.Equal(t, "Component,Stack\nservice,deploy/test\n", output)
}

func TestTfmigrateHelpersCoverBranches(t *testing.T) {
	t.Run("component name precedence", func(t *testing.T) {
		assert.Equal(t, "final", tfmigrateComponentName(&schema.ConfigAndStacksInfo{
			FinalComponent:   "final",
			ComponentFromArg: "arg",
			Component:        "component",
		}))
		assert.Equal(t, "arg", tfmigrateComponentName(&schema.ConfigAndStacksInfo{
			ComponentFromArg: "arg",
			Component:        "component",
		}))
		assert.Equal(t, "component", tfmigrateComponentName(&schema.ConfigAndStacksInfo{
			Component: "component",
		}))
	})

	t.Run("command default", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{}
		atmosConfig := &schema.AtmosConfiguration{}
		atmosConfig.Components.Terraform.Command = "tofu"
		setTfmigrateTerraformCommand(info, atmosConfig)
		assert.Equal(t, "tofu", info.Command)

		info = &schema.ConfigAndStacksInfo{}
		setTfmigrateTerraformCommand(info, &schema.AtmosConfiguration{})
		assert.Equal(t, cfg.TerraformComponentType, info.Command)
	})

	t.Run("skip init and empty workspace are no-ops", func(t *testing.T) {
		assert.NoError(t, initTfmigrateComponent(&schema.ConfigAndStacksInfo{SkipInit: true}))
		assert.Error(t, initTfmigrateComponent(&schema.ConfigAndStacksInfo{DryRun: true}))
		assert.NoError(t, selectTfmigrateWorkspace(&tfmigrateExecutionContext{
			Info: schema.ConfigAndStacksInfo{},
		}))
	})

	t.Run("compat flags empty", func(t *testing.T) {
		assert.Empty(t, CompatFlags())
	})
}

func TestTfmigrateSelectionHelpers(t *testing.T) {
	t.Run("skip abstract disabled and query mismatch", func(t *testing.T) {
		assert.True(t, shouldSkipTfmigrateComponent(map[string]any{
			cfg.MetadataSectionName: map[string]any{"type": "abstract"},
		}, ""))
		assert.True(t, shouldSkipTfmigrateComponent(map[string]any{
			cfg.MetadataSectionName: map[string]any{"enabled": false},
		}, ""))
		assert.True(t, shouldSkipTfmigrateComponent(map[string]any{"vars": map[string]any{"enabled": true}}, ".vars.enabled == false"))
		assert.False(t, shouldSkipTfmigrateComponent(map[string]any{"vars": map[string]any{"enabled": true}}, ".vars.enabled == true"))
	})

	t.Run("walk skips malformed sections and stops on callback error", func(t *testing.T) {
		stacks := map[string]any{
			"bad":                "skip",
			"missing-components": map[string]any{},
			"missing-terraform": map[string]any{
				"components": map[string]any{},
			},
			"good": map[string]any{
				"components": map[string]any{
					cfg.TerraformSectionName: map[string]any{
						"bad-component": "skip",
						"service":       map[string]any{"vars": map[string]any{}},
					},
				},
			},
		}

		var visited []string
		err := walkTfmigrateComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
			visited = append(visited, stackName+"/"+componentName)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"good/service"}, visited)

		expectedErr := assert.AnError
		err = walkTfmigrateComponents(stacks, func(string, string, map[string]any) error {
			return expectedErr
		})
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestExecuteTfmigrateQueryReturnsComponentError(t *testing.T) {
	fixtureDir := createMinimalAtmosFixture(t)
	info := schema.ConfigAndStacksInfo{
		Stack:                  "dev",
		AtmosConfigDirsFromArg: []string{fixtureDir},
		Query:                  ".vars.enabled == true",
		DryRun:                 true,
		SkipInit:               true,
		Identity:               cfg.IdentityFlagDisabledValue,
		ProcessTemplates:       true,
		ProcessFunctions:       true,
	}

	err := executeTfmigrateQuery(&info, tfmigrate.Options{Action: tfmigrate.ActionPlan})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Could not find the component")
}

func TestExecuteTfmigrateAffectedRejectsDependents(t *testing.T) {
	err := executeTfmigrateAffected(&e.DescribeAffectedCmdArgs{
		IncludeDependents: true,
	}, &schema.ConfigAndStacksInfo{}, tfmigrate.Options{Action: tfmigrate.ActionPlan})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestExecuteAffectedMigrateCommandRejectsDependents(t *testing.T) {
	fixtureDir := createMinimalAtmosFixture(t)
	cmd := newDescribeAffectedTestCommand()
	require.NoError(t, cmd.PersistentFlags().Set("include-dependents", "true"))

	err := executeAffectedMigrateCommand(cmd, []string{
		"--config-path", fixtureDir,
		"--identity=false",
	}, &schema.ConfigAndStacksInfo{}, tfmigrate.Options{Action: tfmigrate.ActionPlan})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestDescribeAffectedForTfmigrateRepoPathError(t *testing.T) {
	_, err := describeAffectedForTfmigrate(&e.DescribeAffectedCmdArgs{
		CLIConfig: &schema.AtmosConfiguration{},
		RepoPath:  filepath.Join(t.TempDir(), "missing"),
	})
	require.Error(t, err)
}

func TestFirstTfmigrateHookDefaultsAndInvalidValues(t *testing.T) {
	assert.Equal(t, map[string]string{tfmigrateListKeyMode: tfmigrate.ModeDynamic}, firstTfmigrateHook(map[string]any{}))
	assert.Equal(t, map[string]string{tfmigrateListKeyMode: tfmigrate.ModeDynamic}, firstTfmigrateHook(map[string]any{
		cfg.HooksSectionName: "not-a-map",
	}))

	hook := firstTfmigrateHook(map[string]any{
		cfg.HooksSectionName: map[string]any{
			"not-tfmigrate": map[string]any{"kind": "store"},
			"state-migration": map[string]any{
				"kind":      tfmigrate.Command,
				"mode":      123,
				"migration": 456,
				"config":    true,
			},
		},
	})
	assert.Equal(t, "state-migration", hook[tfmigrateListKeyName])
	assert.Equal(t, tfmigrate.ModeDynamic, hook[tfmigrateListKeyMode])
	assert.Empty(t, hook[tfmigrateListKeyMigration])
	assert.Empty(t, hook[tfmigrateListKeyConfig])
}

func TestTfmigrateListColumnDefaultsAndInvalidRender(t *testing.T) {
	defaults := tfmigrateListColumns(nil)
	require.NotEmpty(t, defaults)
	assert.Equal(t, "Component", defaults[0].Name)

	columns := tfmigrateListColumns([]string{" ", "component"})
	require.Len(t, columns, 1)
	assert.Equal(t, "Component", columns[0].Name)

	err := renderTfmigrateList(nil, migrateListRenderOptions{Format: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func captureDataOutput(t *testing.T, fn func()) string {
	t.Helper()

	ctx, err := ioLayer.NewContext()
	require.NoError(t, err)
	data.InitWriter(ctx)
	t.Cleanup(data.Reset)

	previousStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writePipe
	t.Cleanup(func() {
		os.Stdout = previousStdout
	})

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, readPipe)
		done <- err
	}()

	fn()

	require.NoError(t, writePipe.Close())
	require.NoError(t, <-done)
	require.NoError(t, readPipe.Close())

	return buf.String()
}

func createMinimalAtmosFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform", "service"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "components", "terraform", "service", "main.tf"), []byte(`
terraform {
  required_version = ">= 1.0.0"
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
    command: terraform
    auto_generate_backend_file: false
    workspaces_enabled: false
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*.yaml"
  name_pattern: "{stage}"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "dev.yaml"), []byte(`
vars:
  stage: dev
terraform:
  backend_type: local
  backend:
    local:
      path: terraform.tfstate
components:
  terraform:
    service:
      vars:
        enabled: true
`), 0o644))
	return dir
}

func newDescribeAffectedTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "affected"}
	flags := cmd.PersistentFlags()
	flags.String("base", "", "")
	flags.String("ref", "", "")
	flags.String("sha", "", "")
	flags.String("repo-path", "", "")
	flags.String("ssh-key", "", "")
	flags.String("ssh-key-password", "", "")
	flags.Bool("include-spacelift-admin-stacks", false, "")
	flags.Bool("include-dependents", false, "")
	flags.Bool("include-settings", false, "")
	flags.Bool("upload", false, "")
	flags.Bool("clone-target-ref", false, "")
	flags.Bool("process-templates", true, "")
	flags.Bool("process-functions", true, "")
	flags.StringSlice("skip", nil, "")
	flags.String("pager", "", "")
	flags.StringP("stack", "s", "", "")
	flags.String("format", "yaml", "")
	flags.String("file", "", "")
	flags.String("output-file", "", "")
	flags.String("query", "", "")
	flags.Bool("verbose", false, "")
	flags.Bool("exclude-locked", false, "")
	flags.String("base-path", "", "")
	flags.StringSlice("config", nil, "")
	flags.StringSlice("config-path", nil, "")
	flags.StringSlice("profile", nil, "")
	flags.StringP(cfg.IdentityFlagName, cfg.IdentityFlagShortName, "", "")
	flags.Lookup(cfg.IdentityFlagName).NoOptDefVal = cfg.IdentityFlagSelectValue
	return cmd
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
