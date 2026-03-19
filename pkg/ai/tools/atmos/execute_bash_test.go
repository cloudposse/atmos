package atmos

import (
	"context"
	"runtime"
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

	result, err := tool.Execute(ctx, map[string]any{})

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

	result, err := tool.Execute(ctx, map[string]any{
		"command": "",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestExecuteBashCommandTool_Execute_ValidCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping bash test on Windows")
	}

	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with 'echo' which should always work.
	result, err := tool.Execute(ctx, map[string]any{
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
		result, err := tool.Execute(ctx, map[string]any{
			"command": cmd,
		})

		assert.NoError(t, err, "Command: %s", cmd)
		assert.False(t, result.Success, "Command: %s", cmd)
		assert.Error(t, result.Error, "Command: %s", cmd)
	}
}

func TestExecuteBashCommandTool_Execute_WorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test that requires bash on Windows")
	}

	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with absolute working directory.
	result, err := tool.Execute(ctx, map[string]any{
		"command":     "pwd",
		"working_dir": "/tmp",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "/tmp")
	assert.Equal(t, "/tmp", result.Data["working_dir"])
}

func TestExecuteBashCommandTool_Execute_CommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test that requires bash on Windows")
	}

	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with a command that will fail.
	result, err := tool.Execute(ctx, map[string]any{
		"command": "ls /nonexistent/directory",
	})

	assert.NoError(t, err) // Execute doesn't return error, it's in Result.
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.NotEqual(t, 0, result.Data["exit_code"])
}

func TestExecuteBashCommandTool_Execute_PipeAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test that requires bash on Windows")
	}

	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Pipes with safe commands on both sides are allowed.
	result, err := tool.Execute(ctx, map[string]any{
		"command": "echo 'test' | grep 'test'",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "test")
}

func TestExecuteBashCommandTool_Execute_GitCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test that requires bash on Windows")
	}

	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	// Test with 'git --version' which should work anywhere.
	result, err := tool.Execute(ctx, map[string]any{
		"command": "git --version",
	})

	assert.NoError(t, err)
	// Command should succeed since git --version doesn't need a repo.
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "git version")
	assert.Equal(t, 0, result.Data["exit_code"])
}

func TestExecuteBashCommandTool_Execute_InjectionBypass(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	tests := []struct {
		name    string
		command string
	}{
		{"semicolon", "echo safe; rm -rf /"},
		{"and operator", "echo safe && rm -rf /"},
		{"or operator", "echo safe || rm -rf /"},
		{"pipe with blacklisted right", "echo safe | rm -rf /"},
		{"pipe with blacklisted left", "kill -9 1 | cat"},
		{"command substitution dollar", "echo $(rm -rf /)"},
		{"command substitution backticks", "echo `rm -rf /`"},
		{"subshell", "(rm -rf /)"},
		{"background", "rm -rf / &"},
		{"newline", "echo safe\nrm -rf /"},
		{"quoted substitution", `echo "$(rm -rf //)"`},
		{"path prefixed", "/usr/bin/rm -rf /"},
		{"process substitution", "diff <(cat /etc/shadow) <(echo test)"},
		{"output redirect", "echo pwned > /tmp/evil"},
		{"append redirect", "echo pwned >> /tmp/evil"},
		{"input redirect", "cat < /etc/passwd"},
		{"env var override", "PATH=/evil:$PATH ls"},
		{"negated command", "! echo test"},
		{"if clause", "if true; then echo ok; fi"},
		{"while clause", "while false; do echo loop; done"},
		{"for clause", "for i in 1 2 3; do echo $i; done"},
		{"case clause", "case x in a) echo a;; esac"},
		{"block", "{ echo block; }"},
		{"function declaration", "foo() { echo bar; }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, map[string]any{
				"command": tt.command,
			})

			assert.NoError(t, err, "Command: %s", tt.command)
			assert.False(t, result.Success, "Command should be blocked: %s", tt.command)
			assert.Error(t, result.Error, "Command: %s", tt.command)
		})
	}
}

func TestExecuteBashCommandTool_Execute_AllowedCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping bash test on Windows")
	}

	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewExecuteBashCommandTool(config)
	ctx := context.Background()

	tests := []struct {
		name    string
		command string
	}{
		{"echo hello", "echo hello"},
		{"echo quoted", "echo 'hello world'"},
		{"ls", "ls -la"},
		{"git version", "git --version"},
		{"pwd", "pwd"},
		{"pipe echo grep", "echo hello | grep hello"},
		{"pipe with head", "ls -la | head -5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, map[string]any{
				"command": tt.command,
			})

			assert.NoError(t, err, "Command: %s", tt.command)
			assert.True(t, result.Success, "Command should succeed: %s (error: %v)", tt.command, result.Error)
		})
	}
}

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantBlock bool
	}{
		// Allowed commands.
		{"simple echo", "echo hello", false},
		{"echo with quotes", "echo 'hello world'", false},
		{"ls with flags", "ls -la", false},
		{"git version", "git --version", false},
		{"pwd", "pwd", false},
		{"variable assignment", "VAR=value", false},

		// Blacklisted commands.
		{"rm", "rm -rf /", true},
		{"rm without flags", "rm file.txt", true},
		{"dd", "dd if=/dev/zero of=/dev/sda", true},
		{"kill", "kill -9 1", true},
		{"reboot", "reboot", true},

		// Allowed pipes (both sides are safe).
		{"safe pipe", "echo test | cat", false},
		{"safe pipe chain", "echo test | grep test | head -1", false},

		// Compound/injection commands.
		{"semicolon", "echo safe; rm -rf /", true},
		{"and operator", "echo safe && rm -rf /", true},
		{"or operator", "echo safe || rm -rf /", true},
		{"pipe with blacklisted cmd", "echo test | rm -rf /", true},
		{"subshell", "(echo test)", true},
		{"command substitution", "echo $(whoami)", true},

		// I/O redirections.
		{"output redirect", "echo test > /tmp/file", true},
		{"append redirect", "echo test >> /tmp/file", true},
		{"input redirect", "cat < /etc/passwd", true},

		// Dynamic command names.
		{"dynamic command name", "$CMD arg", true},

		// Environment variable overrides.
		{"env var prefix", "PATH=/evil ls", true},

		// Negation.
		{"negated command", "! echo test", true},

		// Shell control structures.
		{"if clause", "if true; then echo ok; fi", true},
		{"while clause", "while false; do echo loop; done", true},
		{"for clause", "for i in 1 2 3; do echo $i; done", true},
		{"case clause", "case x in a) echo a;; esac", true},
		{"block", "{ echo block; }", true},
		{"function declaration", "foo() { echo bar; }", true},

		// Parse errors.
		{"unclosed single quote", "echo 'unterminated", true},
		{"unclosed double quote", `echo "unterminated`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateCommand(tt.command)
			if tt.wantBlock {
				assert.NotNil(t, result, "Command should be blocked: %s", tt.command)
				assert.False(t, result.Success, "Command should not succeed: %s", tt.command)
			} else {
				assert.Nil(t, result, "Command should be allowed: %s", tt.command)
			}
		})
	}
}
