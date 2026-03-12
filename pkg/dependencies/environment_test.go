package dependencies

import (
	"errors"
	"os"
	execPkg "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestToolchainEnvironment_Resolve tests the Resolve method.
func TestToolchainEnvironment_Resolve(t *testing.T) {
	t.Run("absolute path returned unchanged", func(t *testing.T) {
		absPath := filepath.Join(os.TempDir(), "terraform")
		env := &ToolchainEnvironment{resolved: map[string]string{}}
		result := env.Resolve(absPath)
		assert.Equal(t, absPath, result)
	})

	t.Run("resolved tool returns toolchain path", func(t *testing.T) {
		tofuPath := filepath.Join("home", "user", ".atmos", "bin", "opentofu", "opentofu", "1.8.0", "tofu")
		terraformPath := filepath.Join("home", "user", ".atmos", "bin", "hashicorp", "terraform", "1.10.0", "terraform")
		env := &ToolchainEnvironment{
			resolved: map[string]string{
				"tofu":      tofuPath,
				"terraform": terraformPath,
			},
		}
		result := env.Resolve("tofu")
		assert.Equal(t, tofuPath, result)
	})

	t.Run("unknown tool falls back to original name", func(t *testing.T) {
		env := &ToolchainEnvironment{resolved: map[string]string{}}
		result := env.Resolve("nonexistent-tool-xyz")
		assert.Equal(t, "nonexistent-tool-xyz", result)
	})

	t.Run("system PATH tool is found via LookPath", func(t *testing.T) {
		// Use the current test binary as a known executable on PATH.
		base := filepath.Base(os.Args[0])
		// Only run this subtest if the binary is findable on PATH.
		if _, err := execPkg.LookPath(base); err != nil {
			t.Skip("test binary not on PATH")
		}
		env := &ToolchainEnvironment{resolved: map[string]string{}}
		result := env.Resolve(base)
		assert.NotEqual(t, base, result)
		assert.True(t, filepath.IsAbs(result))
	})
}

// TestToolchainEnvironment_EnvVars tests the EnvVars method.
func TestToolchainEnvironment_EnvVars(t *testing.T) {
	t.Run("no path returns nil", func(t *testing.T) {
		env := &ToolchainEnvironment{resolved: map[string]string{}}
		assert.Nil(t, env.EnvVars())
	})

	t.Run("with path returns PATH entry", func(t *testing.T) {
		testPATH := filepath.Join("toolchain", "bin") + string(os.PathListSeparator) + filepath.Join("usr", "bin")
		env := &ToolchainEnvironment{
			path:     testPATH,
			resolved: map[string]string{},
		}
		vars := env.EnvVars()
		require.Len(t, vars, 1)
		assert.Equal(t, "PATH="+testPATH, vars[0])
	})
}

// TestToolchainEnvironment_PATH tests the PATH method.
func TestToolchainEnvironment_PATH(t *testing.T) {
	t.Run("empty when no deps", func(t *testing.T) {
		env := &ToolchainEnvironment{resolved: map[string]string{}}
		assert.Empty(t, env.PATH())
	})

	t.Run("returns augmented path", func(t *testing.T) {
		testPATH := filepath.Join("toolchain", "bin") + string(os.PathListSeparator) + filepath.Join("usr", "bin")
		env := &ToolchainEnvironment{
			path:     testPATH,
			resolved: map[string]string{},
		}
		assert.Equal(t, testPATH, env.PATH())
	})
}

// TestNewEnvironment_NoDeps tests newEnvironment with empty dependencies.
func TestNewEnvironment_NoDeps(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	env, err := newEnvironment(atmosConfig, nil)
	require.NoError(t, err)
	assert.Empty(t, env.path)
	assert.Empty(t, env.resolved)
	assert.Nil(t, env.EnvVars())
}

// TestNewEnvironment_EmptyDeps tests newEnvironment with empty map.
func TestNewEnvironment_EmptyDeps(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	env, err := newEnvironment(atmosConfig, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, env.path)
	assert.Empty(t, env.resolved)
}

// TestForComponent_NilSections tests ForComponent with nil sections (version command case).
func TestForComponent_NilSections(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	tenv, err := ForComponent(atmosConfig, "terraform", nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, tenv)
	// With nil sections, no deps are resolved, so PATH should be empty.
	assert.Empty(t, tenv.PATH())
}

// TestForSections_NilSections tests ForSections with nil sections.
func TestForSections_NilSections(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tenv, err := ForSections(atmosConfig, nil)
	require.NoError(t, err)
	assert.NotNil(t, tenv)
	assert.Empty(t, tenv.PATH())
}

// TestForSections_NoDependencies tests ForSections with sections that have no dependencies.
func TestForSections_NoDependencies(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	sections := map[string]any{
		"vars": map[string]any{"key": "value"},
	}

	tenv, err := ForSections(atmosConfig, sections)
	require.NoError(t, err)
	assert.NotNil(t, tenv)
	assert.Empty(t, tenv.PATH())
}

// TestForWorkflow_NilDef tests ForWorkflow with nil workflow definition.
func TestForWorkflow_NilDef(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	tenv, err := ForWorkflow(atmosConfig, nil)
	require.NoError(t, err)
	assert.NotNil(t, tenv)
	assert.Empty(t, tenv.PATH())
}

// TestForWorkflow_EmptyDef tests ForWorkflow with empty workflow definition.
func TestForWorkflow_EmptyDef(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	tenv, err := ForWorkflow(atmosConfig, &schema.WorkflowDefinition{})
	require.NoError(t, err)
	assert.NotNil(t, tenv)
	assert.Empty(t, tenv.PATH())
}

// TestForWorkflow_WithToolVersions tests ForWorkflow loading .tool-versions.
func TestForWorkflow_WithToolVersions(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	// Create a .tool-versions file.
	content := "terraform 1.11.4\n"
	err := os.WriteFile(filepath.Join(tempDir, ".tool-versions"), []byte(content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	tenv, err := ForWorkflow(atmosConfig, &schema.WorkflowDefinition{})
	// May fail to install in CI without network — that's fine, we're testing the code path.
	if err == nil {
		assert.NotEmpty(t, tenv.PATH(), "expected non-empty PATH when tools are installed")
	}
}

// TestNewEnvironment_EnsureToolsFailure tests that EnsureTools errors propagate.
func TestNewEnvironment_EnsureToolsFailure(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	// Save and restore toolchain config.
	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	deps := map[string]string{"terraform": "1.10.0"}

	env, err := newEnvironment(atmosConfig, deps,
		withEnsureTools(func(_ map[string]string) error {
			return errors.New("install failed")
		}),
	)
	require.Error(t, err)
	assert.Nil(t, env)
	assert.Contains(t, err.Error(), "failed to install dependencies")
	assert.Contains(t, err.Error(), "install failed")
}

// TestNewEnvironment_BuildPATHFailure tests that BuildToolchainPATH errors propagate.
func TestNewEnvironment_BuildPATHFailure(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	deps := map[string]string{"terraform": "1.10.0"}

	env, err := newEnvironment(atmosConfig, deps,
		withEnsureTools(func(_ map[string]string) error { return nil }),
		withResolveFunc(func(tool string) (string, string, error) {
			return "hashicorp", "terraform", nil
		}),
		withFindBinaryPath(func(owner, repo, version string, _ ...string) (string, error) {
			return filepath.Join(tempDir, "bin", "terraform"), nil
		}),
		withBuildPATH(func(_ *schema.AtmosConfiguration, _ map[string]string) (string, error) {
			return "", errors.New("PATH build failed")
		}),
	)
	require.Error(t, err)
	assert.Nil(t, env)
	assert.Contains(t, err.Error(), "failed to build toolchain PATH")
	assert.Contains(t, err.Error(), "PATH build failed")
}

// TestNewEnvironment_SuccessfulResolution tests the happy path with injected mocks.
func TestNewEnvironment_SuccessfulResolution(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	deps := map[string]string{
		"terraform": "1.10.0",
		"tofu":      "1.8.0",
	}

	expectedPATH := filepath.Join(tempDir, "bin") + string(os.PathListSeparator) + filepath.Join("usr", "bin")

	env, err := newEnvironment(atmosConfig, deps,
		withEnsureTools(func(_ map[string]string) error { return nil }),
		withResolveFunc(func(tool string) (string, string, error) {
			switch tool {
			case "terraform":
				return "hashicorp", "terraform", nil
			case "tofu":
				return "opentofu", "opentofu", nil
			default:
				return "", "", errors.New("unknown tool")
			}
		}),
		withFindBinaryPath(func(owner, repo, version string, _ ...string) (string, error) {
			switch repo {
			case "terraform":
				return filepath.Join(tempDir, "bin", "hashicorp", "terraform", version, "terraform"), nil
			case "opentofu":
				return filepath.Join(tempDir, "bin", "opentofu", "opentofu", version, "tofu"), nil
			default:
				return "", errors.New("not found")
			}
		}),
		withBuildPATH(func(_ *schema.AtmosConfiguration, _ map[string]string) (string, error) {
			return expectedPATH, nil
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, env)

	// Verify resolved map has entries keyed by binary basename.
	assert.Equal(t,
		filepath.Join(tempDir, "bin", "hashicorp", "terraform", "1.10.0", "terraform"),
		env.resolved["terraform"],
	)
	assert.Equal(t,
		filepath.Join(tempDir, "bin", "opentofu", "opentofu", "1.8.0", "tofu"),
		env.resolved["tofu"],
	)

	// Verify PATH.
	assert.Equal(t, expectedPATH, env.path)

	// Verify Resolve uses the resolved map.
	assert.Equal(t,
		filepath.Join(tempDir, "bin", "opentofu", "opentofu", "1.8.0", "tofu"),
		env.Resolve("tofu"),
	)

	// Verify EnvVars.
	vars := env.EnvVars()
	require.Len(t, vars, 1)
	assert.Equal(t, "PATH="+expectedPATH, vars[0])
}

// TestNewEnvironment_ResolveError tests that resolver errors are handled gracefully.
func TestNewEnvironment_ResolveError(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	deps := map[string]string{"unknown-tool": "1.0.0"}
	expectedPATH := "/some/path"

	env, err := newEnvironment(atmosConfig, deps,
		withEnsureTools(func(_ map[string]string) error { return nil }),
		withResolveFunc(func(_ string) (string, string, error) {
			return "", "", errors.New("unknown tool")
		}),
		withBuildPATH(func(_ *schema.AtmosConfiguration, _ map[string]string) (string, error) {
			return expectedPATH, nil
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, env)

	// Resolver error → tool skipped, resolved map empty.
	assert.Empty(t, env.resolved)
	// PATH still set.
	assert.Equal(t, expectedPATH, env.path)
}

// TestNewEnvironment_FindBinaryPathError tests that binary lookup errors are handled gracefully.
func TestNewEnvironment_FindBinaryPathError(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	deps := map[string]string{"terraform": "1.10.0"}
	expectedPATH := "/some/path"

	env, err := newEnvironment(atmosConfig, deps,
		withEnsureTools(func(_ map[string]string) error { return nil }),
		withResolveFunc(func(_ string) (string, string, error) {
			return "hashicorp", "terraform", nil
		}),
		withFindBinaryPath(func(_, _, _ string, _ ...string) (string, error) {
			return "", errors.New("binary not found")
		}),
		withBuildPATH(func(_ *schema.AtmosConfiguration, _ map[string]string) (string, error) {
			return expectedPATH, nil
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, env)

	// FindBinaryPath error → tool skipped, resolved map empty.
	assert.Empty(t, env.resolved)
	assert.Equal(t, expectedPATH, env.path)
}

// TestForWorkflow_LoadToolVersionsError tests ForWorkflow when .tool-versions is unreadable.
func TestForWorkflow_LoadToolVersionsError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	// Create a directory at the .tool-versions path to cause a read error.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := os.MkdirAll(toolVersionsPath, 0o755)
	require.NoError(t, err)

	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath:  filepath.Join(tempDir, ".atmos", "tools"),
			VersionsFile: toolVersionsPath,
		},
	}
	toolchain.SetAtmosConfig(atmosConfig)

	tenv, err := ForWorkflow(atmosConfig, nil)
	require.Error(t, err)
	assert.Nil(t, tenv)
	assert.Contains(t, err.Error(), "failed to load .tool-versions")
}

// TestForWorkflow_MergeConflict tests ForWorkflow when workflow deps conflict with .tool-versions.
func TestForWorkflow_MergeConflict(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	// Create .tool-versions with a constraint.
	err := os.WriteFile(filepath.Join(tempDir, ".tool-versions"), []byte("terraform ~> 1.10.0\n"), 0o644)
	require.NoError(t, err)

	origConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(origConfig) })

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath:  filepath.Join(tempDir, ".atmos", "tools"),
			VersionsFile: filepath.Join(tempDir, ".tool-versions"),
		},
	}
	toolchain.SetAtmosConfig(atmosConfig)

	// Workflow deps with conflicting version.
	workflowDef := &schema.WorkflowDefinition{
		Dependencies: &schema.Dependencies{
			Tools: map[string]string{
				"terraform": "1.9.0", // Conflicts with ~> 1.10.0.
			},
		},
	}

	tenv, err := ForWorkflow(atmosConfig, workflowDef)
	require.Error(t, err)
	assert.Nil(t, tenv)
	assert.Contains(t, err.Error(), "failed to merge dependencies")
}

// TestForComponent_WithDepsError tests ForComponent when dependency resolution fails.
func TestForComponent_WithDepsError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	// Provide conflicting deps that violate version constraints.
	stackConfig := map[string]any{
		"terraform": map[string]any{
			"dependencies": map[string]any{
				"tools": map[string]any{
					"terraform": "~> 1.10.0",
				},
			},
		},
	}
	componentConfig := map[string]any{
		"dependencies": map[string]any{
			"tools": map[string]any{
				"terraform": "1.9.8", // Violates ~> 1.10.0 constraint.
			},
		},
	}

	tenv, err := ForComponent(atmosConfig, "terraform", stackConfig, componentConfig)
	require.Error(t, err)
	assert.Nil(t, tenv)
	assert.Contains(t, err.Error(), "failed to resolve component dependencies")
}
