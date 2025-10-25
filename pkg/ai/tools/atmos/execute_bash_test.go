package atmos

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestExecuteBashCommandTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteBashCommandTool(config)

	assert.Equal(t, "execute_bash_command", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.True(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 2)
	assert.Equal(t, "command", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "working_dir", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestExecuteBashCommandTool_Execute_MissingParameter(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "command")
}

func TestExecuteBashCommandTool_Execute_EmptyCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestExecuteBashCommandTool_Execute_ValidCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with 'echo' which should always work.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "echo 'Hello World'",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Hello World")
	assert.Equal(t, 0, result.Data["exit_code"])
}

func TestExecuteBashCommandTool_Execute_BlacklistedCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	blacklistedCmds := []string{
		"rm -rf /",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda",
		"kill -9 1",
		"killall -9 bash",
		"reboot",
		"shutdown -h now",
		"halt",
		"poweroff",
		"init 0",
	}

	for _, cmd := range blacklistedCmds {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"command": cmd,
		})

		assert.NoError(t, err, "Command: %s", cmd)
		assert.False(t, result.Success, "Command: %s", cmd)
		assert.Error(t, result.Error, "Command: %s", cmd)
		assert.Contains(t, result.Error.Error(), "blacklisted", "Command: %s", cmd)
	}
}

func TestExecuteBashCommandTool_Execute_WorkingDirectory(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with absolute working directory.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command":     "pwd",
		"working_dir": "/tmp",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "/tmp")
	assert.Equal(t, "/tmp", result.Data["working_dir"])
}

func TestExecuteBashCommandTool_Execute_CommandFails(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with a command that will fail.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "ls /nonexistent/directory",
	})

	assert.NoError(t, err) // Execute doesn't return error, it's in Result.
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.NotEqual(t, 0, result.Data["exit_code"])
}

func TestExecuteBashCommandTool_Execute_ShellFeatures(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with pipes and redirects.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "echo 'test' | grep 'test'",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "test")
}

func TestExecuteBashCommandTool_Execute_GitCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with 'git --version' which should work anywhere.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "git --version",
	})

	assert.NoError(t, err)
	// Command should succeed since git --version doesn't need a repo.
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "git version")
	assert.Equal(t, 0, result.Data["exit_code"])
}
