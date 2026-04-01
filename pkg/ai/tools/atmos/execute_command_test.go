package atmos

import (
	"context"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteAtmosCommandTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteAtmosCommandTool(config)

	assert.Equal(t, "execute_atmos_command", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.True(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 1)
	assert.Equal(t, "command", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestExecuteAtmosCommandTool_Execute_MissingParameter(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteAtmosCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "command")
}

func TestExecuteAtmosCommandTool_Execute_EmptyCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteAtmosCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

// TestExecuteAtmosCommandTool_Execute_WhitespaceCommand verifies that a command string
// consisting entirely of whitespace is treated as an empty command (len(args)==0 branch).
func TestExecuteAtmosCommandTool_Execute_WhitespaceCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewExecuteAtmosCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "   ",
	})

	require.NoError(t, err)
	assert.False(t, result.Success)
	require.Error(t, result.Error)
}

// TestExecuteAtmosCommandTool_Execute_FailedCommand verifies the error path when
// the subprocess exits with a non-zero exit code (err != nil after CombinedOutput).
func TestExecuteAtmosCommandTool_Execute_FailedCommand(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	// _ATMOS_TEST_EXIT_ONE=1 causes TestMain to exit(1) immediately when this test
	// binary is re-invoked as a subprocess, giving us a non-zero exit without any
	// Unix-only helper binaries.
	t.Setenv("_ATMOS_TEST_EXIT_ONE", "1")

	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewExecuteAtmosCommandTool(config)
	tool.binaryPath = exePath
	ctx := context.Background()

	result, ferr := tool.Execute(ctx, map[string]interface{}{
		"command": "some-arg",
	})

	require.NoError(t, ferr)
	assert.False(t, result.Success)
	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "exit")
}

func TestExecuteAtmosCommandTool_Execute_ValidCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewExecuteAtmosCommandTool(config)
	// Override binary to a known command for testing (not the test binary).
	tool.binaryPath = "echo"
	ctx := context.Background()

	// Test with echo which always works.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "hello world",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "hello world")
}

// TestIsDestructiveAtmosCommand verifies the helper correctly classifies commands.
func TestIsDestructiveAtmosCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		destructive bool
	}{
		// State-modifying terraform subcommands.
		{"terraform apply", []string{"terraform", "apply", "vpc", "-s", "prod"}, true},
		{"terraform destroy", []string{"terraform", "destroy", "vpc", "-s", "prod"}, true},
		{"terraform import", []string{"terraform", "import", "vpc", "-s", "prod"}, true},
		{"terraform force-unlock", []string{"terraform", "force-unlock", "12345"}, true},
		{"terraform state rm", []string{"terraform", "state", "rm", "module.vpc"}, true},
		{"terraform state mv", []string{"terraform", "state", "mv", "a", "b"}, true},
		{"terraform state push", []string{"terraform", "state", "push", "terraform.tfstate"}, true},
		{"terraform workspace new", []string{"terraform", "workspace", "new", "prod"}, true},
		{"terraform workspace delete", []string{"terraform", "workspace", "delete", "prod"}, true},
		// Case-insensitive matching.
		{"terraform APPLY uppercase", []string{"terraform", "APPLY", "vpc"}, true},

		// Safe read-only terraform subcommands.
		{"terraform plan", []string{"terraform", "plan", "vpc", "-s", "prod"}, false},
		{"terraform show", []string{"terraform", "show", "vpc", "-s", "prod"}, false},
		{"terraform output", []string{"terraform", "output", "vpc", "-s", "prod"}, false},
		{"terraform validate", []string{"terraform", "validate", "vpc"}, false},
		{"terraform state list", []string{"terraform", "state", "list"}, false},
		{"terraform state show", []string{"terraform", "state", "show", "module.vpc"}, false},
		{"terraform workspace list", []string{"terraform", "workspace", "list"}, false},
		{"terraform workspace show", []string{"terraform", "workspace", "show"}, false},

		// Non-terraform top-level commands.
		{"describe stacks", []string{"describe", "stacks"}, false},
		{"list stacks", []string{"list", "stacks"}, false},
		{"workflow deploy", []string{"workflow", "deploy"}, false},

		// Edge cases.
		{"empty args", []string{}, false},
		{"single arg", []string{"terraform"}, false},
		{"terraform state without subcommand", []string{"terraform", "state"}, false},
		{"terraform workspace without subcommand", []string{"terraform", "workspace"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isDestructiveAtmosCommand(tc.args)
			assert.Equal(t, tc.destructive, got, "isDestructiveAtmosCommand(%v)", tc.args)
		})
	}
}

// TestExecuteAtmosCommandTool_DestructiveBlocked verifies that state-modifying commands
// are rejected when the permission mode is not ModePrompt.
func TestExecuteAtmosCommandTool_DestructiveBlocked(t *testing.T) {
	blockingModes := []permission.Mode{
		permission.ModeAllow,
		permission.ModeDeny,
		permission.ModeYOLO,
	}

	destructiveCmds := []string{
		"terraform apply vpc -s prod",
		"terraform destroy vpc -s prod",
		"terraform import vpc -s prod",
		"terraform force-unlock 12345",
		"terraform state rm module.vpc",
		"terraform state mv a b",
		"terraform state push terraform.tfstate",
		"terraform workspace new prod",
		"terraform workspace delete prod",
	}

	// Use the test binary as a placeholder; the validator must block before it is ever invoked.
	exePath, err := os.Executable()
	require.NoError(t, err)

	for _, mode := range blockingModes {
		for _, cmd := range destructiveCmds {
			t.Run(string(mode)+"/"+cmd, func(t *testing.T) {
				config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
				tool := NewExecuteAtmosCommandToolWithPermission(config, mode)
				tool.binaryPath = exePath // never reached; validator blocks first

				result, err := tool.Execute(context.Background(), map[string]interface{}{
					"command": cmd,
				})

				require.NoError(t, err)
				assert.False(t, result.Success, "expected command to be blocked: %s", cmd)
				require.Error(t, result.Error)
				assert.Contains(t, result.Error.Error(), "modifies state")
			})
		}
	}
}

// TestExecuteAtmosCommandTool_DestructiveAllowedInPromptMode verifies that
// state-modifying commands are NOT blocked when permission mode is ModePrompt,
// allowing the upstream permission system to prompt for confirmation.
func TestExecuteAtmosCommandTool_DestructiveAllowedInPromptMode(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewExecuteAtmosCommandToolWithPermission(config, permission.ModePrompt)
	// Use echo consistent with the existing ValidCommand test in this file.
	tool.binaryPath = "echo"

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "terraform apply vpc -s prod",
	})

	require.NoError(t, err)
	// echo always succeeds; we just verify the command was NOT blocked by the validator.
	assert.True(t, result.Success, "destructive command should not be pre-blocked in ModePrompt")
}

// TestExecuteAtmosCommandTool_SafeCommandsAlwaysAllowed verifies that safe
// read-only commands pass through the validator regardless of the permission mode.
func TestExecuteAtmosCommandTool_SafeCommandsAlwaysAllowed(t *testing.T) {
	modes := []permission.Mode{
		permission.ModeAllow,
		permission.ModePrompt,
		permission.ModeDeny,
		permission.ModeYOLO,
	}

	safeCmds := []string{
		"terraform plan vpc -s prod",
		"terraform show vpc -s prod",
		"terraform output vpc -s prod",
		"terraform validate vpc",
		"terraform state list",
		"terraform workspace list",
		"describe stacks",
		"list stacks",
	}

	// Use echo consistent with the existing ValidCommand test in this file.
	for _, mode := range modes {
		for _, cmd := range safeCmds {
			t.Run(string(mode)+"/"+cmd, func(t *testing.T) {
				config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
				tool := NewExecuteAtmosCommandToolWithPermission(config, mode)
				tool.binaryPath = "echo"

				result, err := tool.Execute(context.Background(), map[string]interface{}{
					"command": cmd,
				})

				require.NoError(t, err)
				require.NotNil(t, result)
				// The validator must not block safe commands with ErrAICommandDestructive.
				if result.Error != nil {
					assert.NotContains(t, result.Error.Error(), "modifies state",
						"safe command blocked unexpectedly: %s (mode=%s)", cmd, mode)
				}
			})
		}
	}
}
