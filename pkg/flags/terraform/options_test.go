package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name             string
		viperValues      map[string]interface{}
		positionalArgs   []string
		passThroughArgs  []string
		wantStack        string
		wantDryRun       bool
		wantUploadStatus bool
		wantSkipInit     bool
		wantFromPlan     string
		wantLogsLevel    string
	}{
		{
			name: "all defaults",
			viperValues: map[string]interface{}{
				"logs-level": "Warning",
			},
			positionalArgs:   []string{"plan", "vpc"},
			passThroughArgs:  nil,
			wantStack:        "",
			wantDryRun:       false,
			wantUploadStatus: false,
			wantSkipInit:     false,
			wantFromPlan:     "",
			wantLogsLevel:    "Warning",
		},
		{
			name: "terraform plan with stack and upload-status",
			viperValues: map[string]interface{}{
				"stack":         "dev",
				"upload-status": true,
				"logs-level":    "Debug",
			},
			positionalArgs:   []string{"plan", "vpc"},
			passThroughArgs:  []string{"-var", "foo=bar"},
			wantStack:        "dev",
			wantDryRun:       false,
			wantUploadStatus: true,
			wantSkipInit:     false,
			wantFromPlan:     "",
			wantLogsLevel:    "Debug",
		},
		{
			name: "terraform apply with from-plan",
			viperValues: map[string]interface{}{
				"stack":      "prod",
				"from-plan":  "vpc-plan-123.tfplan",
				"skip-init":  true,
				"logs-level": "Warn",
			},
			positionalArgs:   []string{"apply", "vpc"},
			passThroughArgs:  nil,
			wantStack:        "prod",
			wantDryRun:       false,
			wantUploadStatus: false,
			wantSkipInit:     true,
			wantFromPlan:     "vpc-plan-123.tfplan",
			wantLogsLevel:    "Warn",
		},
		{
			name: "dry-run mode",
			viperValues: map[string]interface{}{
				"stack":      "staging",
				"dry-run":    true,
				"logs-level": "Warning",
			},
			positionalArgs:   []string{"plan", "rds"},
			passThroughArgs:  []string{"-var-file", "staging.tfvars"},
			wantStack:        "staging",
			wantDryRun:       true,
			wantUploadStatus: false,
			wantSkipInit:     false,
			wantFromPlan:     "",
			wantLogsLevel:    "Warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test Viper instance.
			v := viper.New()
			for key, value := range tt.viperValues {
				v.Set(key, value)
			}

			// Create test command.
			cmd := &cobra.Command{}

			// Parse flags.
			interpreter := ParseFlags(cmd, v, tt.positionalArgs, tt.passThroughArgs)

			// Verify common flags.
			assert.Equal(t, tt.wantStack, interpreter.Stack, "Stack mismatch")
			assert.Equal(t, tt.wantDryRun, interpreter.DryRun, "DryRun mismatch")

			// Verify Terraform-specific flags.
			assert.Equal(t, tt.wantUploadStatus, interpreter.UploadStatus, "UploadStatus mismatch")
			assert.Equal(t, tt.wantSkipInit, interpreter.SkipInit, "SkipInit mismatch")
			assert.Equal(t, tt.wantFromPlan, interpreter.FromPlan, "FromPlan mismatch")

			// Verify global flags (from embedded global.Flags).
			assert.Equal(t, tt.wantLogsLevel, interpreter.LogsLevel, "LogsLevel mismatch")

			// Verify arguments.
			assert.Equal(t, tt.positionalArgs, interpreter.GetPositionalArgs(), "Positional args mismatch")
			assert.Equal(t, tt.passThroughArgs, interpreter.GetSeparatedArgs(), "Pass-through args mismatch")
		})
	}
}

func TestOptions_IdentityFlag(t *testing.T) {
	tests := []struct {
		name                      string
		viperValue                string
		flagChanged               bool
		wantIsInteractiveSelector bool
		wantValue                 string
		wantIsEmpty               bool
	}{
		{
			name:                      "identity not provided",
			viperValue:                "",
			flagChanged:               false,
			wantIsInteractiveSelector: false,
			wantValue:                 "",
			wantIsEmpty:               true,
		},
		{
			name:                      "interactive selection (--identity with no value)",
			viperValue:                cfg.IdentityFlagSelectValue,
			flagChanged:               true,
			wantIsInteractiveSelector: true,
			wantValue:                 cfg.IdentityFlagSelectValue,
			wantIsEmpty:               false,
		},
		{
			name:                      "explicit identity",
			viperValue:                "prod-admin",
			flagChanged:               true,
			wantIsInteractiveSelector: false,
			wantValue:                 "prod-admin",
			wantIsEmpty:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test Viper instance.
			v := viper.New()
			v.Set("identity", tt.viperValue)
			v.Set("logs-level", "Warning")

			// Create test command with identity flag.
			cmd := &cobra.Command{}
			cmd.Flags().StringP("identity", "i", "", "Identity")

			// Mark flag as changed if needed.
			if tt.flagChanged {
				_ = cmd.Flags().Set("identity", tt.viperValue)
			}

			// Parse flags.
			interpreter := ParseFlags(cmd, v, nil, nil)

			// Verify identity selector behavior.
			assert.Equal(t, tt.wantIsInteractiveSelector, interpreter.Identity.IsInteractiveSelector(),
				"IsInteractiveSelector mismatch")
			assert.Equal(t, tt.wantValue, interpreter.Identity.Value(), "Identity value mismatch")
			assert.Equal(t, tt.wantIsEmpty, interpreter.Identity.IsEmpty(), "IsEmpty mismatch")
		})
	}
}

func TestOptions_Embedding(t *testing.T) {
	// Test that Options properly embeds global.Flags and implements flags.CommandOptions.
	v := viper.New()
	v.Set("stack", "dev")
	v.Set("logs-level", "Debug")
	v.Set("chdir", "/tmp/atmos")
	v.Set("upload-status", true)

	cmd := &cobra.Command{}

	interpreter := ParseFlags(cmd, v, []string{"plan", "vpc"}, []string{"-var", "foo=bar"})

	// Verify flags.CommandOptions interface implementation.
	var _ flags.CommandOptions = &interpreter

	// Verify global.Flags embedding.
	globalFlags := interpreter.Getglobal.Flags()
	assert.NotNil(t, globalFlags, "Getglobal.Flags should not return nil")
	assert.Equal(t, "Debug", globalFlags.LogsLevel, "global.Flags.LogsLevel mismatch")
	assert.Equal(t, "/tmp/atmos", globalFlags.Chdir, "global.Flags.Chdir mismatch")

	// Verify Terraform-specific fields are accessible.
	assert.Equal(t, "dev", interpreter.Stack, "Stack mismatch")
	assert.True(t, interpreter.UploadStatus, "UploadStatus should be true")

	// Verify arguments.
	assert.Equal(t, []string{"plan", "vpc"}, interpreter.GetPositionalArgs())
	assert.Equal(t, []string{"-var", "foo=bar"}, interpreter.GetSeparatedArgs())
}

func TestOptions_ZeroValues(t *testing.T) {
	// Test that zero-value Options is safe to use.
	interpreter := Options{}

	// Should not panic.
	assert.NotPanics(t, func() {
		_ = interpreter.Getglobal.Flags()
		_ = interpreter.GetPositionalArgs()
		_ = interpreter.GetSeparatedArgs()
		_ = interpreter.Stack
		_ = interpreter.UploadStatus
		_ = interpreter.SkipInit
		_ = interpreter.FromPlan
	})

	// Zero values should be sensible.
	assert.Equal(t, "", interpreter.Stack)
	assert.False(t, interpreter.UploadStatus)
	assert.False(t, interpreter.SkipInit)
	assert.Equal(t, "", interpreter.FromPlan)
	assert.Nil(t, interpreter.GetPositionalArgs())
	assert.Nil(t, interpreter.GetSeparatedArgs())
}

func TestFlagsRegistry(t *testing.T) {
	registry := FlagsRegistry()

	// Verify registry contains all expected flags.
	tests := []struct {
		name         string
		wantExists   bool
		wantRequired bool
	}{
		// Global flags.
		{"chdir", true, false},
		{"logs-level", true, false},
		{"identity", true, false},

		// Common flags.
		{"stack", true, false},
		{"dry-run", true, false},

		// Terraform-specific flags.
		{"upload-status", true, false},
		{"skip-init", true, false},
		{"from-plan", true, false},

		// Non-existent flag.
		{"nonexistent", false, false},
	}

	for _, tt := range tests {
		t.Run("has_"+tt.name, func(t *testing.T) {
			exists := registry.Has(tt.name)
			assert.Equal(t, tt.wantExists, exists, "Flag existence mismatch for %s", tt.name)

			if tt.wantExists {
				flag := registry.Get(tt.name)
				assert.NotNil(t, flag, "Flag should not be nil: %s", tt.name)
				assert.Equal(t, tt.wantRequired, flag.IsRequired(), "Required mismatch for %s", tt.name)
			}
		})
	}
}

func TestFlagsRegistry_IdentityFlag(t *testing.T) {
	registry := FlagsRegistry()

	// Verify identity flag has NoOptDefVal for interactive selection.
	identityFlag := registry.Get("identity")
	assert.NotNil(t, identityFlag, "identity flag should exist")

	noOptDefVal := identityFlag.GetNoOptDefVal()
	assert.Equal(t, cfg.IdentityFlagSelectValue, noOptDefVal,
		"identity flag should have NoOptDefVal = %s", cfg.IdentityFlagSelectValue)

	// Verify environment variables.
	envVars := identityFlag.GetEnvVars()
	assert.Contains(t, envVars, "ATMOS_IDENTITY", "identity flag should support ATMOS_IDENTITY env var")
	assert.Contains(t, envVars, "IDENTITY", "identity flag should support IDENTITY env var")
}

func TestOptions_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name            string
		scenario        string
		viperValues     map[string]interface{}
		positionalArgs  []string
		passThroughArgs []string
		verify          func(t *testing.T, interpreter Options)
	}{
		{
			name:     "scenario 1: terraform plan with upload to Atmos Pro",
			scenario: "User runs: atmos terraform plan vpc -s dev --upload-status --logs-level=Debug",
			viperValues: map[string]interface{}{
				"stack":         "dev",
				"upload-status": true,
				"logs-level":    "Debug",
			},
			positionalArgs:  []string{"plan", "vpc"},
			passThroughArgs: nil,
			verify: func(t *testing.T, interpreter Options) {
				assert.Equal(t, "dev", interpreter.Stack)
				assert.True(t, interpreter.UploadStatus)
				assert.Equal(t, "Debug", interpreter.LogsLevel)
				assert.Equal(t, []string{"plan", "vpc"}, interpreter.GetPositionalArgs())
			},
		},
		{
			name:     "scenario 2: terraform apply from saved plan",
			scenario: "User runs: atmos terraform apply vpc -s prod --from-plan=vpc.tfplan --skip-init",
			viperValues: map[string]interface{}{
				"stack":      "prod",
				"from-plan":  "vpc.tfplan",
				"skip-init":  true,
				"logs-level": "Warning",
			},
			positionalArgs:  []string{"apply", "vpc"},
			passThroughArgs: nil,
			verify: func(t *testing.T, interpreter Options) {
				assert.Equal(t, "prod", interpreter.Stack)
				assert.Equal(t, "vpc.tfplan", interpreter.FromPlan)
				assert.True(t, interpreter.SkipInit)
				assert.Equal(t, []string{"apply", "vpc"}, interpreter.GetPositionalArgs())
			},
		},
		{
			name:     "scenario 3: dry-run terraform destroy",
			scenario: "User runs: atmos terraform destroy rds -s staging --dry-run -- -auto-approve",
			viperValues: map[string]interface{}{
				"stack":      "staging",
				"dry-run":    true,
				"logs-level": "Warn",
			},
			positionalArgs:  []string{"destroy", "rds"},
			passThroughArgs: []string{"-auto-approve"},
			verify: func(t *testing.T, interpreter Options) {
				assert.Equal(t, "staging", interpreter.Stack)
				assert.True(t, interpreter.DryRun)
				assert.Equal(t, []string{"destroy", "rds"}, interpreter.GetPositionalArgs())
				assert.Equal(t, []string{"-auto-approve"}, interpreter.GetSeparatedArgs())
			},
		},
		{
			name:     "scenario 4: terraform plan with custom working directory",
			scenario: "User runs: atmos terraform plan vpc -s dev --chdir=/tmp/atmos -- -var foo=bar",
			viperValues: map[string]interface{}{
				"stack":      "dev",
				"chdir":      "/tmp/atmos",
				"logs-level": "Warning",
			},
			positionalArgs:  []string{"plan", "vpc"},
			passThroughArgs: []string{"-var", "foo=bar"},
			verify: func(t *testing.T, interpreter Options) {
				assert.Equal(t, "dev", interpreter.Stack)
				assert.Equal(t, "/tmp/atmos", interpreter.Chdir)
				assert.Equal(t, []string{"plan", "vpc"}, interpreter.GetPositionalArgs())
				assert.Equal(t, []string{"-var", "foo=bar"}, interpreter.GetSeparatedArgs())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.scenario)

			// Create test Viper instance.
			v := viper.New()
			for key, value := range tt.viperValues {
				v.Set(key, value)
			}

			// Create test command.
			cmd := &cobra.Command{}

			// Parse flags.
			interpreter := ParseFlags(cmd, v, tt.positionalArgs, tt.passThroughArgs)

			// Run scenario-specific verification.
			tt.verify(t, interpreter)
		})
	}
}
