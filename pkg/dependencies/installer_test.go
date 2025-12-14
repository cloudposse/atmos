package dependencies

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockResolver is a mock implementation of toolchain.ToolResolver for testing.
type mockResolver struct {
	resolveFunc func(toolName string) (string, string, error)
}

func (m *mockResolver) Resolve(toolName string) (string, string, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(toolName)
	}
	// Default behavior: parse owner/repo format or return error.
	if strings.Contains(toolName, "/") {
		parts := strings.Split(toolName, "/")
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}
	return "", "", errors.New("tool not found")
}

func TestNewInstaller(t *testing.T) {
	t.Run("creates installer with nil config", func(t *testing.T) {
		inst := NewInstaller(nil)
		require.NotNil(t, inst)
		assert.Nil(t, inst.atmosConfig)
		assert.NotNil(t, inst.resolver)
		assert.NotNil(t, inst.installFunc)
		assert.NotNil(t, inst.fileExistsFunc)
	})

	t.Run("creates installer with config", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				InstallPath: "/custom/path",
			},
		}
		inst := NewInstaller(config)
		require.NotNil(t, inst)
		assert.Equal(t, config, inst.atmosConfig)
	})

	t.Run("applies options", func(t *testing.T) {
		mockRes := &mockResolver{}
		mockInstall := func(string, bool, bool) error { return nil }
		mockFileExists := func(string) bool { return true }

		inst := NewInstaller(nil,
			WithResolver(mockRes),
			WithInstallFunc(mockInstall),
			WithFileExistsFunc(mockFileExists),
		)

		require.NotNil(t, inst)
		assert.Equal(t, mockRes, inst.resolver)
	})
}

func TestEnsureTools(t *testing.T) {
	tests := []struct {
		name         string
		dependencies map[string]string
		setupMock    func() (*mockResolver, InstallFunc, func(string) bool, func() bool)
		wantErr      bool
		errIs        error
	}{
		{
			name:         "empty dependencies returns nil",
			dependencies: map[string]string{},
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				return &mockResolver{}, nil, nil, nil
			},
			wantErr: false,
		},
		{
			name:         "nil dependencies returns nil",
			dependencies: nil,
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				return &mockResolver{}, nil, nil, nil
			},
			wantErr: false,
		},
		{
			name: "tool already installed - no install called",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
			},
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				installCalled := false
				installFunc := func(string, bool, bool) error {
					installCalled = true
					return nil
				}
				fileExists := func(path string) bool {
					return true // Tool exists.
				}
				// Verifier returns true if install was NOT called (expected behavior).
				verifier := func() bool { return !installCalled }
				return resolver, installFunc, fileExists, verifier
			},
			wantErr: false,
		},
		{
			name: "tool not installed - install called",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
			},
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				installFunc := func(toolSpec string, _, _ bool) error {
					if toolSpec != "hashicorp/terraform@1.10.0" {
						return errors.New("unexpected tool spec")
					}
					return nil
				}
				fileExists := func(path string) bool {
					return false // Tool does not exist.
				}
				return resolver, installFunc, fileExists, nil
			},
			wantErr: false,
		},
		{
			name: "install failure propagates error",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
			},
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				installFunc := func(string, bool, bool) error {
					return errors.New("install failed")
				}
				fileExists := func(path string) bool {
					return false
				}
				return resolver, installFunc, fileExists, nil
			},
			wantErr: true,
			errIs:   errUtils.ErrToolInstall,
		},
		{
			name: "multiple tools - all installed",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
				"cloudposse/atmos":    "1.0.0",
			},
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						parts := strings.Split(toolName, "/")
						if len(parts) == 2 {
							return parts[0], parts[1], nil
						}
						return "", "", errors.New("invalid tool")
					},
				}
				installCount := 0
				installFunc := func(string, bool, bool) error {
					installCount++
					return nil
				}
				fileExists := func(path string) bool {
					return false // All tools need install.
				}
				// Verifier returns true if install was called exactly twice (once per tool).
				verifier := func() bool { return installCount == 2 }
				return resolver, installFunc, fileExists, verifier
			},
			wantErr: false,
		},
		{
			name: "error on first tool stops processing",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
				"cloudposse/atmos":    "1.0.0",
			},
			setupMock: func() (*mockResolver, InstallFunc, func(string) bool, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						parts := strings.Split(toolName, "/")
						if len(parts) == 2 {
							return parts[0], parts[1], nil
						}
						return "", "", errors.New("invalid tool")
					},
				}
				installFunc := func(string, bool, bool) error {
					return errors.New("install failed")
				}
				fileExists := func(path string) bool {
					return false
				}
				return resolver, installFunc, fileExists, nil
			},
			wantErr: true,
			errIs:   errUtils.ErrToolInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, installFunc, fileExists, verifier := tt.setupMock()

			opts := []InstallerOption{WithResolver(resolver)}
			if installFunc != nil {
				opts = append(opts, WithInstallFunc(installFunc))
			}
			if fileExists != nil {
				opts = append(opts, WithFileExistsFunc(fileExists))
			}

			inst := NewInstaller(nil, opts...)
			err := inst.EnsureTools(tt.dependencies)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				require.NoError(t, err)
			}

			// Run verifier if provided to check mock behavior.
			if verifier != nil {
				assert.True(t, verifier(), "verifier failed: mock behavior did not match expected")
			}
		})
	}
}

func TestIsToolInstalled(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		version     string
		config      *schema.AtmosConfiguration
		setupMock   func() (*mockResolver, func(string) bool)
		want        bool
		wantPathArg string
	}{
		{
			name:    "tool exists - returns true",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config:  nil,
			setupMock: func() (*mockResolver, func(string) bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				fileExists := func(path string) bool {
					return true
				}
				return resolver, fileExists
			},
			want: true,
		},
		{
			name:    "tool does not exist - returns false",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config:  nil,
			setupMock: func() (*mockResolver, func(string) bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				fileExists := func(path string) bool {
					return false
				}
				return resolver, fileExists
			},
			want: false,
		},
		{
			name:    "resolver error - returns false",
			tool:    "unknown-tool",
			version: "1.0.0",
			config:  nil,
			setupMock: func() (*mockResolver, func(string) bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "", "", errors.New("tool not found")
					},
				}
				fileExists := func(path string) bool {
					return true // Should not be called.
				}
				return resolver, fileExists
			},
			want: false,
		},
		{
			name:    "custom install path from config",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config: &schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					InstallPath: "/custom/tools",
				},
			},
			setupMock: func() (*mockResolver, func(string) bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				var capturedPath string
				fileExists := func(path string) bool {
					capturedPath = path
					_ = capturedPath
					// Verify the custom path is used.
					return strings.HasPrefix(path, "/custom/tools")
				}
				return resolver, fileExists
			},
			want: true,
		},
		{
			name:    "default install path when config is nil",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config:  nil,
			setupMock: func() (*mockResolver, func(string) bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				fileExists := func(path string) bool {
					// Verify the default path is used.
					return strings.HasPrefix(path, ".tools")
				}
				return resolver, fileExists
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, fileExists := tt.setupMock()

			inst := NewInstaller(tt.config,
				WithResolver(resolver),
				WithFileExistsFunc(fileExists),
			)

			got := inst.isToolInstalled(tt.tool, tt.version)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsToolInstalled_PathConstruction(t *testing.T) {
	t.Run("constructs correct binary path", func(t *testing.T) {
		var capturedPath string

		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				return "hashicorp", "terraform", nil
			},
		}
		fileExists := func(path string) bool {
			capturedPath = path
			return true
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithFileExistsFunc(fileExists),
		)

		_ = inst.isToolInstalled("terraform", "1.10.0")

		// Expected path: .tools/bin/hashicorp/terraform/1.10.0/terraform.
		expectedPath := filepath.Join(".tools", "bin", "hashicorp", "terraform", "1.10.0", "terraform")
		assert.Equal(t, expectedPath, capturedPath)
	})
}

func TestBuildToolchainPATH(t *testing.T) {
	// Set a known PATH for testing using t.Setenv (auto-restores after test).
	testPath := "/usr/bin:/bin"
	t.Setenv("PATH", testPath)

	tests := []struct {
		name         string
		config       *schema.AtmosConfiguration
		dependencies map[string]string
		wantContains []string
		wantPrefix   bool
	}{
		{
			name:         "empty dependencies returns current PATH",
			config:       nil,
			dependencies: map[string]string{},
			wantContains: []string{testPath},
			wantPrefix:   false,
		},
		{
			name:         "nil dependencies returns current PATH",
			config:       nil,
			dependencies: nil,
			wantContains: []string{testPath},
			wantPrefix:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildToolchainPATH(tt.config, tt.dependencies)
			require.NoError(t, err)

			for _, s := range tt.wantContains {
				assert.Contains(t, result, s)
			}
		})
	}
}

func TestBuildToolchainPATH_CustomInstallPath(t *testing.T) {
	testPath := "/usr/bin"
	t.Setenv("PATH", testPath)

	t.Run("uses custom install path from config", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				InstallPath: "/my/custom/tools",
			},
		}

		// Note: BuildToolchainPATH creates its own resolver internally,
		// so we can only test with tools that would resolve successfully.
		// For invalid tools, they are skipped.
		result, err := BuildToolchainPATH(config, map[string]string{
			"invalid-tool": "1.0.0",
		})
		require.NoError(t, err)

		// Invalid tools are skipped, so we just get the original PATH.
		assert.Equal(t, testPath, result)
	})

	t.Run("nil config uses default path", func(t *testing.T) {
		result, err := BuildToolchainPATH(nil, map[string]string{
			"invalid-tool": "1.0.0",
		})
		require.NoError(t, err)

		// Invalid tools are skipped.
		assert.Equal(t, testPath, result)
	})
}

func TestUpdatePathForTools(t *testing.T) {
	testPath := "/usr/bin:/bin"

	// Note: We need to use os.Setenv inside defer/t.Cleanup blocks for manual restoration
	// because the function under test (UpdatePathForTools) actually modifies PATH.
	originalPath := os.Getenv("PATH")
	t.Cleanup(func() {
		os.Setenv("PATH", originalPath)
	})

	t.Run("empty dependencies does not modify PATH", func(t *testing.T) {
		t.Setenv("PATH", testPath)
		err := UpdatePathForTools(nil, map[string]string{})
		require.NoError(t, err)

		// PATH should remain unchanged.
		assert.Equal(t, testPath, os.Getenv("PATH"))
	})

	t.Run("nil dependencies does not modify PATH", func(t *testing.T) {
		t.Setenv("PATH", testPath)
		err := UpdatePathForTools(nil, nil)
		require.NoError(t, err)

		assert.Equal(t, testPath, os.Getenv("PATH"))
	})
}

func TestFileExists(t *testing.T) {
	t.Run("returns true for existing file", func(t *testing.T) {
		// Create a temp file using t.TempDir for automatic cleanup.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test-file")
		err := os.WriteFile(tmpFile, []byte("test"), 0o644)
		require.NoError(t, err)

		assert.True(t, fileExists(tmpFile))
	})

	t.Run("returns false for non-existing file", func(t *testing.T) {
		assert.False(t, fileExists("/nonexistent/path/to/file"))
	})

	t.Run("returns true for existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.True(t, fileExists(tmpDir))
	})
}

func TestGetPathFromEnv(t *testing.T) {
	t.Run("returns current PATH", func(t *testing.T) {
		testPath := "/test/path:/another/path"
		t.Setenv("PATH", testPath)

		result := getPathFromEnv()
		assert.Equal(t, testPath, result)
	})

	t.Run("returns empty string when PATH is unset", func(t *testing.T) {
		t.Setenv("PATH", "")

		result := getPathFromEnv()
		assert.Equal(t, "", result)
	})
}

func TestEnsureTool(t *testing.T) {
	t.Run("does not install when tool already installed", func(t *testing.T) {
		installCalled := false

		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				return "hashicorp", "terraform", nil
			},
		}
		installFunc := func(string, bool, bool) error {
			installCalled = true
			return nil
		}
		fileExists := func(path string) bool {
			return true // Tool already installed.
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithInstallFunc(installFunc),
			WithFileExistsFunc(fileExists),
		)

		err := inst.ensureTool("terraform", "1.10.0")
		require.NoError(t, err)
		assert.False(t, installCalled, "install should not be called when tool exists")
	})

	t.Run("installs when tool not installed", func(t *testing.T) {
		var installedSpec string

		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				return "hashicorp", "terraform", nil
			},
		}
		installFunc := func(toolSpec string, _, _ bool) error {
			installedSpec = toolSpec
			return nil
		}
		fileExists := func(path string) bool {
			return false // Tool not installed.
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithInstallFunc(installFunc),
			WithFileExistsFunc(fileExists),
		)

		err := inst.ensureTool("terraform", "1.10.0")
		require.NoError(t, err)
		assert.Equal(t, "terraform@1.10.0", installedSpec)
	})

	t.Run("returns error when install fails", func(t *testing.T) {
		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				return "hashicorp", "terraform", nil
			},
		}
		installFunc := func(string, bool, bool) error {
			return errors.New("network error")
		}
		fileExists := func(path string) bool {
			return false
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithInstallFunc(installFunc),
			WithFileExistsFunc(fileExists),
		)

		err := inst.ensureTool("terraform", "1.10.0")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrToolInstall)
		assert.Contains(t, err.Error(), "terraform@1.10.0")
	})
}
