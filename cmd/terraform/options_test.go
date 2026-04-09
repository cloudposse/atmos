package terraform

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseTerraformRunOptions(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*viper.Viper)
		expected *TerraformRunOptions
	}{
		{
			name: "all flags set",
			setup: func(v *viper.Viper) {
				v.Set("process-templates", true)
				v.Set("process-functions", false)
				v.Set("skip", []string{"func1", "func2"})
				v.Set("dry-run", true)
				v.Set("skip-init", true)
				v.Set("init-pass-vars", true)
				v.Set("auto-generate-backend-file", "true")
				v.Set("init-run-reconfigure", "false")
				v.Set("planfile", "/tmp/my-plan.tfplan")
				v.Set("skip-planfile", true)
				v.Set("deploy-run-init", true)
				v.Set("query", ".components.terraform.vpc")
				v.Set("components", []string{"vpc", "eks"})
				v.Set("all", true)
				v.Set("affected", true)
			},
			expected: &TerraformRunOptions{
				ProcessTemplates:        true,
				ProcessFunctions:        false,
				Skip:                    []string{"func1", "func2"},
				DryRun:                  true,
				SkipInit:                true,
				InitPassVars:            true,
				AutoGenerateBackendFile: "true",
				InitRunReconfigure:      "false",
				PlanFile:                "/tmp/my-plan.tfplan",
				PlanSkipPlanfile:        true,
				DeployRunInit:           true,
				Query:                   ".components.terraform.vpc",
				Components:              []string{"vpc", "eks"},
				All:                     true,
				Affected:                true,
			},
		},
		{
			name:  "empty values (defaults)",
			setup: func(v *viper.Viper) {},
			expected: &TerraformRunOptions{
				ProcessTemplates:        false,
				ProcessFunctions:        false,
				Skip:                    nil,
				DryRun:                  false,
				SkipInit:                false,
				InitPassVars:            false,
				AutoGenerateBackendFile: "",
				InitRunReconfigure:      "",
				PlanFile:                "",
				PlanSkipPlanfile:        false,
				DeployRunInit:           false,
				Query:                   "",
				Components:              nil,
				All:                     false,
				Affected:                false,
			},
		},
		{
			name: "only processing flags set",
			setup: func(v *viper.Viper) {
				v.Set("process-templates", true)
				v.Set("process-functions", true)
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
			},
		},
		{
			name: "only multi-component flags set",
			setup: func(v *viper.Viper) {
				v.Set("all", true)
				v.Set("affected", false)
				v.Set("components", []string{"comp1", "comp2", "comp3"})
			},
			expected: &TerraformRunOptions{
				Components: []string{"comp1", "comp2", "comp3"},
				All:        true,
				Affected:   false,
			},
		},
		{
			name: "dry-run only",
			setup: func(v *viper.Viper) {
				v.Set("dry-run", true)
			},
			expected: &TerraformRunOptions{
				DryRun: true,
			},
		},
		{
			name: "skip-init only",
			setup: func(v *viper.Viper) {
				v.Set("skip-init", true)
			},
			expected: &TerraformRunOptions{
				SkipInit: true,
			},
		},
		{
			name: "init-pass-vars flag",
			setup: func(v *viper.Viper) {
				v.Set("init-pass-vars", true)
			},
			expected: &TerraformRunOptions{
				InitPassVars: true,
			},
		},
		{
			name: "skip with single item",
			setup: func(v *viper.Viper) {
				v.Set("skip", []string{"template_function"})
			},
			expected: &TerraformRunOptions{
				Skip: []string{"template_function"},
			},
		},
		{
			name: "query only",
			setup: func(v *viper.Viper) {
				v.Set("query", ".components.terraform | keys")
			},
			expected: &TerraformRunOptions{
				Query: ".components.terraform | keys",
			},
		},
		{
			name: "planfile flag",
			setup: func(v *viper.Viper) {
				v.Set("planfile", "/tmp/my-plan.tfplan")
			},
			expected: &TerraformRunOptions{
				PlanFile: "/tmp/my-plan.tfplan",
			},
		},
		{
			name: "skip-planfile flag",
			setup: func(v *viper.Viper) {
				v.Set("skip-planfile", true)
			},
			expected: &TerraformRunOptions{
				PlanSkipPlanfile: true,
			},
		},
		{
			name: "deploy-run-init flag",
			setup: func(v *viper.Viper) {
				v.Set("deploy-run-init", true)
			},
			expected: &TerraformRunOptions{
				DeployRunInit: true,
			},
		},
		{
			name: "auto-generate-backend-file flag",
			setup: func(v *viper.Viper) {
				v.Set("auto-generate-backend-file", "true")
			},
			expected: &TerraformRunOptions{
				AutoGenerateBackendFile: "true",
			},
		},
		{
			name: "init-run-reconfigure flag",
			setup: func(v *viper.Viper) {
				v.Set("init-run-reconfigure", "true")
			},
			expected: &TerraformRunOptions{
				InitRunReconfigure: "true",
			},
		},
		{
			name: "apply command flags (planfile with backend flags)",
			setup: func(v *viper.Viper) {
				v.Set("planfile", "/tmp/apply.tfplan")
				v.Set("auto-generate-backend-file", "false")
				v.Set("init-run-reconfigure", "true")
			},
			expected: &TerraformRunOptions{
				PlanFile:                "/tmp/apply.tfplan",
				AutoGenerateBackendFile: "false",
				InitRunReconfigure:      "true",
			},
		},
		{
			name: "deploy command flags (deploy-run-init with planfile)",
			setup: func(v *viper.Viper) {
				v.Set("deploy-run-init", true)
				v.Set("planfile", "/tmp/deploy.tfplan")
				v.Set("auto-generate-backend-file", "true")
			},
			expected: &TerraformRunOptions{
				DeployRunInit:           true,
				PlanFile:                "/tmp/deploy.tfplan",
				AutoGenerateBackendFile: "true",
			},
		},
		{
			name: "plan command flags (skip-planfile with backend flags)",
			setup: func(v *viper.Viper) {
				v.Set("skip-planfile", true)
				v.Set("auto-generate-backend-file", "true")
				v.Set("init-run-reconfigure", "false")
			},
			expected: &TerraformRunOptions{
				PlanSkipPlanfile:        true,
				AutoGenerateBackendFile: "true",
				InitRunReconfigure:      "false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			tt.setup(v)

			result := ParseTerraformRunOptions(v)

			assert.Equal(t, tt.expected.ProcessTemplates, result.ProcessTemplates, "ProcessTemplates should match")
			assert.Equal(t, tt.expected.ProcessFunctions, result.ProcessFunctions, "ProcessFunctions should match")
			assert.Equal(t, tt.expected.Skip, result.Skip, "Skip should match")
			assert.Equal(t, tt.expected.DryRun, result.DryRun, "DryRun should match")
			assert.Equal(t, tt.expected.SkipInit, result.SkipInit, "SkipInit should match")
			assert.Equal(t, tt.expected.InitPassVars, result.InitPassVars, "InitPassVars should match")
			assert.Equal(t, tt.expected.AutoGenerateBackendFile, result.AutoGenerateBackendFile, "AutoGenerateBackendFile should match")
			assert.Equal(t, tt.expected.InitRunReconfigure, result.InitRunReconfigure, "InitRunReconfigure should match")
			assert.Equal(t, tt.expected.PlanFile, result.PlanFile, "PlanFile should match")
			assert.Equal(t, tt.expected.PlanSkipPlanfile, result.PlanSkipPlanfile, "PlanSkipPlanfile should match")
			assert.Equal(t, tt.expected.DeployRunInit, result.DeployRunInit, "DeployRunInit should match")
			assert.Equal(t, tt.expected.Query, result.Query, "Query should match")
			assert.Equal(t, tt.expected.Components, result.Components, "Components should match")
			assert.Equal(t, tt.expected.All, result.All, "All should match")
			assert.Equal(t, tt.expected.Affected, result.Affected, "Affected should match")
		})
	}
}

func TestTerraformRunOptions_Fields(t *testing.T) {
	// Test that TerraformRunOptions struct has all expected fields.
	opts := TerraformRunOptions{
		ProcessTemplates:        true,
		ProcessFunctions:        true,
		Skip:                    []string{"skip1"},
		DryRun:                  true,
		SkipInit:                true,
		InitPassVars:            true,
		AutoGenerateBackendFile: "true",
		InitRunReconfigure:      "false",
		PlanFile:                "/tmp/plan.tfplan",
		PlanSkipPlanfile:        true,
		DeployRunInit:           true,
		Query:                   "query",
		Components:              []string{"comp1"},
		All:                     true,
		Affected:                true,
	}

	assert.True(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
	assert.Equal(t, []string{"skip1"}, opts.Skip)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.SkipInit)
	assert.True(t, opts.InitPassVars)
	assert.Equal(t, "true", opts.AutoGenerateBackendFile)
	assert.Equal(t, "false", opts.InitRunReconfigure)
	assert.Equal(t, "/tmp/plan.tfplan", opts.PlanFile)
	assert.True(t, opts.PlanSkipPlanfile)
	assert.True(t, opts.DeployRunInit)
	assert.Equal(t, "query", opts.Query)
	assert.Equal(t, []string{"comp1"}, opts.Components)
	assert.True(t, opts.All)
	assert.True(t, opts.Affected)
}

// TestApplyOptionsToInfo tests that options are correctly applied to ConfigAndStacksInfo.
func TestApplyOptionsToInfo(t *testing.T) {
	tests := []struct {
		name      string
		opts      *TerraformRunOptions
		checkInfo func(*testing.T, *schema.ConfigAndStacksInfo)
	}{
		{
			name: "planfile sets PlanFile and UseTerraformPlan",
			opts: &TerraformRunOptions{
				PlanFile: "/tmp/test.tfplan",
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "/tmp/test.tfplan", info.PlanFile)
				assert.True(t, info.UseTerraformPlan)
			},
		},
		{
			name: "skip-planfile sets PlanSkipPlanfile to true",
			opts: &TerraformRunOptions{
				PlanSkipPlanfile: true,
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "true", info.PlanSkipPlanfile)
			},
		},
		{
			name: "deploy-run-init sets DeployRunInit to true",
			opts: &TerraformRunOptions{
				DeployRunInit: true,
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "true", info.DeployRunInit)
			},
		},
		{
			name: "auto-generate-backend-file is applied",
			opts: &TerraformRunOptions{
				AutoGenerateBackendFile: "false",
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "false", info.AutoGenerateBackendFile)
			},
		},
		{
			name: "init-run-reconfigure is applied",
			opts: &TerraformRunOptions{
				InitRunReconfigure: "true",
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "true", info.InitRunReconfigure)
			},
		},
		{
			name: "init-pass-vars sets InitPassVars to true",
			opts: &TerraformRunOptions{
				InitPassVars: true,
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "true", info.InitPassVars)
			},
		},
		{
			name: "skip-init sets SkipInit to true",
			opts: &TerraformRunOptions{
				SkipInit: true,
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.True(t, info.SkipInit)
			},
		},
		{
			name: "empty planfile does not set UseTerraformPlan",
			opts: &TerraformRunOptions{
				PlanFile: "",
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "", info.PlanFile)
				assert.False(t, info.UseTerraformPlan)
			},
		},
		{
			name: "all command-specific flags together",
			opts: &TerraformRunOptions{
				PlanFile:                "/tmp/deploy.tfplan",
				DeployRunInit:           true,
				AutoGenerateBackendFile: "true",
				InitRunReconfigure:      "false",
				InitPassVars:            true,
			},
			checkInfo: func(t *testing.T, info *schema.ConfigAndStacksInfo) {
				assert.Equal(t, "/tmp/deploy.tfplan", info.PlanFile)
				assert.True(t, info.UseTerraformPlan)
				assert.Equal(t, "true", info.DeployRunInit)
				assert.Equal(t, "true", info.AutoGenerateBackendFile)
				assert.Equal(t, "false", info.InitRunReconfigure)
				assert.Equal(t, "true", info.InitPassVars)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{}
			applyOptionsToInfo(info, tt.opts)
			tt.checkInfo(t, info)
		})
	}
}
