package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestParsedConfig_ToTerraformOptions(t *testing.T) {
	tests := []struct {
		name             string
		parsedConfig     ParsedConfig
		wantStack        string
		wantDryRun       bool
		wantUploadStatus bool
		wantSkipInit     bool
		wantFromPlan     string
		wantLogsLevel    string
		wantChdir        string
		wantConfig       []string
	}{
		{
			name: "all flags set",
			parsedConfig: ParsedConfig{
				Flags: map[string]interface{}{
					"stack":         "dev",
					"dry-run":       true,
					"upload-status": true,
					"skip-init":     true,
					"from-plan":     "plan.tfplan",
					"logs-level":    "Debug",
					"chdir":         "/tmp/atmos",
					"config":        []string{"atmos.yaml", "override.yaml"},
				},
				PositionalArgs:  []string{"plan", "vpc"},
				PassThroughArgs: []string{"-var", "foo=bar"},
			},
			wantStack:        "dev",
			wantDryRun:       true,
			wantUploadStatus: true,
			wantSkipInit:     true,
			wantFromPlan:     "plan.tfplan",
			wantLogsLevel:    "Debug",
			wantChdir:        "/tmp/atmos",
			wantConfig:       []string{"atmos.yaml", "override.yaml"},
		},
		{
			name: "minimal flags",
			parsedConfig: ParsedConfig{
				Flags: map[string]interface{}{
					"stack":      "prod",
					"logs-level": "Warning",
				},
				PositionalArgs: []string{"apply", "rds"},
			},
			wantStack:        "prod",
			wantDryRun:       false,
			wantUploadStatus: false,
			wantSkipInit:     false,
			wantFromPlan:     "",
			wantLogsLevel:    "Warning",
			wantChdir:        "",
			wantConfig:       nil,
		},
		{
			name: "empty config",
			parsedConfig: ParsedConfig{
				Flags:          map[string]interface{}{},
				PositionalArgs: []string{"plan"},
			},
			wantStack:        "",
			wantDryRun:       false,
			wantUploadStatus: false,
			wantSkipInit:     false,
			wantFromPlan:     "",
			wantLogsLevel:    "",
			wantChdir:        "",
			wantConfig:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interpreter := tt.parsedConfig.ToTerraformOptions()

			// Verify common flags.
			assert.Equal(t, tt.wantStack, interpreter.Stack, "Stack mismatch")
			assert.Equal(t, tt.wantDryRun, interpreter.DryRun, "DryRun mismatch")

			// Verify Terraform-specific flags.
			assert.Equal(t, tt.wantUploadStatus, interpreter.UploadStatus, "UploadStatus mismatch")
			assert.Equal(t, tt.wantSkipInit, interpreter.SkipInit, "SkipInit mismatch")
			assert.Equal(t, tt.wantFromPlan, interpreter.FromPlan, "FromPlan mismatch")

			// Verify global flags.
			assert.Equal(t, tt.wantLogsLevel, interpreter.LogsLevel, "LogsLevel mismatch")
			assert.Equal(t, tt.wantChdir, interpreter.Chdir, "Chdir mismatch")
			assert.Equal(t, tt.wantConfig, interpreter.Config, "Config mismatch")

			// Verify arguments.
			assert.Equal(t, tt.parsedConfig.PositionalArgs, interpreter.GetPositionalArgs(), "PositionalArgs mismatch")
			assert.Equal(t, tt.parsedConfig.PassThroughArgs, interpreter.GetPassThroughArgs(), "PassThroughArgs mismatch")
		})
	}
}

func TestParsedConfig_ToTerraformOptions_Identity(t *testing.T) {
	tests := []struct {
		name                      string
		atmosFlags                map[string]interface{}
		wantIsInteractiveSelector bool
		wantValue                 string
		wantIsEmpty               bool
	}{
		{
			name:                      "identity not provided",
			atmosFlags:                map[string]interface{}{},
			wantIsInteractiveSelector: false,
			wantValue:                 "",
			wantIsEmpty:               true,
		},
		{
			name: "interactive selection",
			atmosFlags: map[string]interface{}{
				"identity": cfg.IdentityFlagSelectValue,
			},
			wantIsInteractiveSelector: true,
			wantValue:                 cfg.IdentityFlagSelectValue,
			wantIsEmpty:               false,
		},
		{
			name: "explicit identity",
			atmosFlags: map[string]interface{}{
				"identity": "prod-admin",
			},
			wantIsInteractiveSelector: false,
			wantValue:                 "prod-admin",
			wantIsEmpty:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedConfig := ParsedConfig{
				Flags: tt.atmosFlags,
			}

			interpreter := parsedConfig.ToTerraformOptions()

			assert.Equal(t, tt.wantIsInteractiveSelector, interpreter.Identity.IsInteractiveSelector(),
				"IsInteractiveSelector mismatch")
			assert.Equal(t, tt.wantValue, interpreter.Identity.Value(), "Identity value mismatch")
			assert.Equal(t, tt.wantIsEmpty, interpreter.Identity.IsEmpty(), "IsEmpty mismatch")
		})
	}
}

func TestParsedConfig_ToTerraformOptions_TypeSafety(t *testing.T) {
	// Test that type-safe access is better than map access.
	parsedConfig := ParsedConfig{
		Flags: map[string]interface{}{
			"stack":         "dev",
			"upload-status": true,
			"logs-level":    "Debug",
		},
		PositionalArgs:  []string{"plan", "vpc"},
		PassThroughArgs: []string{"-var", "foo=bar"},
	}

	// ❌ Old way: Weak typing with runtime type assertions.
	// This can panic if types don't match.
	t.Run("weak_typing_old_way", func(t *testing.T) {
		stack := parsedConfig.Flags["stack"].(string)
		uploadStatus := parsedConfig.Flags["upload-status"].(bool)
		logsLevel := parsedConfig.Flags["logs-level"].(string)

		assert.Equal(t, "dev", stack)
		assert.True(t, uploadStatus)
		assert.Equal(t, "Debug", logsLevel)
	})

	// ✅ New way: Strong typing with compile-time safety.
	// No runtime type assertions needed.
	t.Run("strong_typing_new_way", func(t *testing.T) {
		interpreter := parsedConfig.ToTerraformOptions()

		// Type-safe access - compiler verifies types.
		stack := interpreter.Stack
		uploadStatus := interpreter.UploadStatus
		logsLevel := interpreter.LogsLevel

		assert.Equal(t, "dev", stack)
		assert.True(t, uploadStatus)
		assert.Equal(t, "Debug", logsLevel)

		// Also has properly typed arguments.
		assert.Equal(t, []string{"plan", "vpc"}, interpreter.GetPositionalArgs())
		assert.Equal(t, []string{"-var", "foo=bar"}, interpreter.GetPassThroughArgs())
	})
}
