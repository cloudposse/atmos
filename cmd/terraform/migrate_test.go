package terraform

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMigrateCommandRegistered(t *testing.T) {
	require.NotNil(t, migrateCmd)
	assert.Equal(t, "migrate", migrateCmd.Use)

	var foundPlan, foundApply, foundList bool
	for _, cmd := range migrateCmd.Commands() {
		switch cmd.Use {
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
