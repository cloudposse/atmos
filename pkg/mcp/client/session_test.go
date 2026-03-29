package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewSession_InitialState(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test-server",
		Command: "echo",
	}
	session := NewSession(cfg)

	assert.Equal(t, "test-server", session.Name())
	assert.Equal(t, StatusStopped, session.Status())
	assert.Nil(t, session.LastError())
	assert.Nil(t, session.Tools())
}

func TestSession_Start_InvalidCommand(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "bad-server",
		Command: "nonexistent-binary-that-does-not-exist-xyz",
	}
	session := NewSession(cfg)

	err := session.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerStartFailed)
	assert.Equal(t, StatusError, session.Status())
	assert.NotNil(t, session.LastError())
}

func TestSession_Stop_WhenStopped(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)

	// Stopping an already-stopped session should succeed.
	err := session.Stop()
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, session.Status())
}

func TestSession_CallTool_WhenNotRunning(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)

	_, err := session.CallTool(context.Background(), "some-tool", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerNotRunning)
}

func TestSession_Ping_WhenNotRunning(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)

	err := session.Ping(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerNotRunning)
}

func TestBuildEnv(t *testing.T) {
	env := map[string]string{
		"AWS_REGION":  "us-east-1",
		"AWS_PROFILE": "production",
	}

	result := buildEnv(env)

	// Should contain the original environment plus the new vars.
	assert.True(t, len(result) > 2, "should include OS env plus configured vars")

	// Check that configured vars are present.
	found := 0
	for _, e := range result {
		if e == "AWS_REGION=us-east-1" || e == "AWS_PROFILE=production" {
			found++
		}
	}
	assert.Equal(t, 2, found, "both configured env vars should be present")
}

func TestSession_Config(t *testing.T) {
	cfg := &ParsedConfig{
		Name:        "test",
		Command:     "echo",
		Description: "Test server",
	}
	session := NewSession(cfg)
	assert.Equal(t, cfg, session.Config())
	assert.Equal(t, "Test server", session.Config().Description)
}

func TestSession_Start_AlreadyRunning(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)
	// Manually set to running to test the early return.
	session.mu.Lock()
	session.status = StatusRunning
	session.mu.Unlock()

	err := session.Start(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, session.Status())
}

func TestBuildEnv_EmptyMap(t *testing.T) {
	result := buildEnv(map[string]string{})
	// Should just be the OS environment.
	assert.NotEmpty(t, result)
}

func TestPrepareEnv_NoOpts(t *testing.T) {
	config := &ParsedConfig{
		Name:    "test",
		Command: "echo",
		Env:     map[string]string{"KEY": "VALUE"},
	}
	env, err := prepareEnv(context.Background(), config, nil)
	require.NoError(t, err)

	// Should contain the configured env var.
	found := false
	for _, e := range env {
		if e == "KEY=VALUE" {
			found = true
			break
		}
	}
	assert.True(t, found, "configured env var should be present")
}

func TestPrepareEnv_WithFailingOpt(t *testing.T) {
	config := &ParsedConfig{
		Name:    "test",
		Command: "echo",
		Env:     map[string]string{},
	}
	// An opt that returns an error — should fail startup.
	failingOpt := func(_ context.Context, _ *ParsedConfig, env []string) ([]string, error) {
		return env, assert.AnError
	}
	_, err := prepareEnv(context.Background(), config, []StartOption{failingOpt})
	assert.Error(t, err, "prepareEnv should return error when opt fails")
	assert.Contains(t, err.Error(), "auth setup failed")
}

func TestPrepareEnv_WithSuccessOpt(t *testing.T) {
	config := &ParsedConfig{
		Name:    "test",
		Command: "echo",
		Env:     map[string]string{},
	}
	appendOpt := func(_ context.Context, _ *ParsedConfig, env []string) ([]string, error) {
		return append(env, "INJECTED=true"), nil
	}
	env, err := prepareEnv(context.Background(), config, []StartOption{appendOpt})
	require.NoError(t, err)

	found := false
	for _, e := range env {
		if e == "INJECTED=true" {
			found = true
			break
		}
	}
	assert.True(t, found, "opt-injected var should be present")
}

func TestSession_Start_WithOpts(t *testing.T) {
	// Start with a non-existent binary but verify opts are called.
	optCalled := false
	opt := func(_ context.Context, _ *ParsedConfig, env []string) ([]string, error) {
		optCalled = true
		return env, nil
	}

	cfg := &ParsedConfig{
		Name:    "test",
		Command: "nonexistent-binary-xyz-456",
	}
	session := NewSession(cfg)

	// Will fail due to bad command, but opts should have been called.
	err := session.Start(context.Background(), opt)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerStartFailed)
	assert.True(t, optCalled, "start option should have been called even if server fails")
}

func TestManager_Start_WithOpts(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPServerConfig{
		"test": {Command: "nonexistent-binary-xyz-789"},
	})
	require.NoError(t, err)

	optCalled := false
	opt := func(_ context.Context, _ *ParsedConfig, env []string) ([]string, error) {
		optCalled = true
		return env, nil
	}

	// Will fail but opt should be called.
	err = mgr.Start(context.Background(), "test", opt)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerStartFailed)
	assert.True(t, optCalled)
}

func TestManager_Test_WithFailedStart(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPServerConfig{
		"bad": {Command: "nonexistent-binary-xyz-000"},
	})
	require.NoError(t, err)

	result := mgr.Test(context.Background(), "bad")
	assert.False(t, result.ServerStarted)
	assert.Error(t, result.Error)
	assert.ErrorIs(t, result.Error, errUtils.ErrMCPServerStartFailed)
}

// TestResolveCommandInEnv tests the resolveCommandInEnv function.
func TestResolveCommandInEnv(t *testing.T) {
	t.Run("absolute path returned as-is", func(t *testing.T) {
		absPath := filepath.Join(t.TempDir(), "mybin")
		result := resolveCommandInEnv(absPath, nil)
		assert.Equal(t, absPath, result)
	})

	t.Run("relative path with slash returned as-is", func(t *testing.T) {
		result := resolveCommandInEnv("./bin/server", []string{"PATH=/usr/bin"})
		assert.Equal(t, "./bin/server", result)
	})

	t.Run("relative path with backslash returned as-is", func(t *testing.T) {
		result := resolveCommandInEnv("bin\\server", []string{"PATH=/usr/bin"})
		assert.Equal(t, "bin\\server", result)
	})

	t.Run("empty PATH returns original command", func(t *testing.T) {
		env := []string{"HOME=/tmp"}
		result := resolveCommandInEnv("mycommand", env)
		assert.Equal(t, "mycommand", result)
	})

	t.Run("command found in PATH", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "mytool")
		err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755)
		require.NoError(t, err)

		env := []string{"PATH=" + dir}
		result := resolveCommandInEnv("mytool", env)
		assert.Equal(t, binPath, result)
	})

	t.Run("command not found in PATH", func(t *testing.T) {
		dir := t.TempDir()
		env := []string{"PATH=" + dir}
		result := resolveCommandInEnv("nonexistent-cmd-xyz", env)
		assert.Equal(t, "nonexistent-cmd-xyz", result)
	})

	t.Run("PATHEXT handling with exe extension", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "mytool.exe")
		err := os.WriteFile(binPath, []byte("binary"), 0o755)
		require.NoError(t, err)

		env := []string{
			"PATH=" + dir,
			"PATHEXT=.exe;.cmd;.bat",
		}
		result := resolveCommandInEnv("mytool", env)
		assert.Equal(t, binPath, result)
	})

	t.Run("multiple PATH entries command in second directory", func(t *testing.T) {
		dir1 := t.TempDir()
		dir2 := t.TempDir()
		binPath := filepath.Join(dir2, "secondtool")
		err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755)
		require.NoError(t, err)

		env := []string{"PATH=" + dir1 + string(os.PathListSeparator) + dir2}
		result := resolveCommandInEnv("secondtool", env)
		assert.Equal(t, binPath, result)
	})

	t.Run("directory is skipped not returned as match", func(t *testing.T) {
		dir := t.TempDir()
		// Create a directory with the command name - should not match.
		subdir := filepath.Join(dir, "mydir")
		err := os.MkdirAll(subdir, 0o755)
		require.NoError(t, err)

		env := []string{"PATH=" + dir}
		result := resolveCommandInEnv("mydir", env)
		assert.Equal(t, "mydir", result)
	})

	t.Run("last PATH entry wins for env variable", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "tool")
		err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755)
		require.NoError(t, err)

		// Second PATH= entry should override the first.
		env := []string{
			"PATH=/nonexistent",
			"PATH=" + dir,
		}
		result := resolveCommandInEnv("tool", env)
		assert.Equal(t, binPath, result)
	})
}

func TestExtractEnvVars(t *testing.T) {
	t.Run("both PATH and PATHEXT present", func(t *testing.T) {
		env := []string{"HOME=/tmp", "PATH=/usr/bin", "PATHEXT=.exe;.cmd"}
		path, pathext := extractEnvVars(env)
		assert.Equal(t, "/usr/bin", path)
		assert.Equal(t, ".exe;.cmd", pathext)
	})

	t.Run("only PATH present", func(t *testing.T) {
		env := []string{"PATH=/usr/bin"}
		path, pathext := extractEnvVars(env)
		assert.Equal(t, "/usr/bin", path)
		assert.Empty(t, pathext)
	})

	t.Run("empty env", func(t *testing.T) {
		path, pathext := extractEnvVars(nil)
		assert.Empty(t, path)
		assert.Empty(t, pathext)
	})

	t.Run("last PATH wins", func(t *testing.T) {
		env := []string{"PATH=/first", "PATH=/second"}
		path, _ := extractEnvVars(env)
		assert.Equal(t, "/second", path)
	})
}

func TestBuildExtensions(t *testing.T) {
	t.Run("empty pathext", func(t *testing.T) {
		result := buildExtensions("")
		assert.Equal(t, []string{""}, result)
	})

	t.Run("with extensions", func(t *testing.T) {
		result := buildExtensions(".exe;.cmd;.bat")
		assert.Equal(t, []string{"", ".exe", ".cmd", ".bat"}, result)
	})

	t.Run("with empty entries", func(t *testing.T) {
		result := buildExtensions(".exe;;.cmd;")
		assert.Equal(t, []string{"", ".exe", ".cmd"}, result)
	})
}

func TestFindExecutable(t *testing.T) {
	t.Run("found bare name", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "mytool")
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))

		result := findExecutable(dir, "mytool", []string{""})
		assert.Equal(t, binPath, result)
	})

	t.Run("found with extension", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "mytool.exe")
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))

		result := findExecutable(dir, "mytool", []string{"", ".exe"})
		assert.Equal(t, binPath, result)
	})

	t.Run("not found", func(t *testing.T) {
		dir := t.TempDir()
		result := findExecutable(dir, "nonexistent", []string{""})
		assert.Empty(t, result)
	})

	t.Run("directory skipped", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))

		result := findExecutable(dir, "subdir", []string{""})
		assert.Empty(t, result)
	})
}
