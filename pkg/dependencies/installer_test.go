package dependencies

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Sentinel errors for test mocks.
var (
	errToolNotFound       = errors.New("tool not found")
	errInstallFailed      = errors.New("install failed")
	errUnexpectedToolSpec = errors.New("unexpected tool spec")
	errInvalidTool        = errors.New("invalid tool")
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
	return "", "", errToolNotFound
}

// mockBinaryPathFinder is a mock implementation of BinaryPathFinder for testing.
type mockBinaryPathFinder struct {
	findBinaryPathFunc func(owner, repo, version string, binaryName ...string) (string, error)
}

func (m *mockBinaryPathFinder) FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error) {
	if m.findBinaryPathFunc != nil {
		return m.findBinaryPathFunc(owner, repo, version, binaryName...)
	}
	return "", errToolNotFound
}

func TestNewInstaller(t *testing.T) {
	t.Run("creates installer with nil config", func(t *testing.T) {
		inst := NewInstaller(nil)
		require.NotNil(t, inst)
		assert.Nil(t, inst.atmosConfig)
		assert.NotNil(t, inst.resolver)
		assert.NotNil(t, inst.installFunc)
		assert.NotNil(t, inst.batchInstallFunc)
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
		mockInstall := func(string, bool, bool, bool, bool) error { return nil }
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
		setupMock    func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool)
		wantErr      bool
		errIs        error
	}{
		{
			name:         "empty dependencies returns nil",
			dependencies: map[string]string{},
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				return &mockResolver{}, nil, nil, nil
			},
			wantErr: false,
		},
		{
			name:         "nil dependencies returns nil",
			dependencies: nil,
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				return &mockResolver{}, nil, nil, nil
			},
			wantErr: false,
		},
		{
			name: "tool already installed - no install called",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
			},
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				installCalled := false
				batchInstallFunc := func([]string, bool) error {
					installCalled = true
					return nil
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "/path/to/terraform", nil // Tool exists.
					},
				}
				// Verifier returns true if install was NOT called (expected behavior).
				verifier := func() bool { return !installCalled }
				return resolver, batchInstallFunc, finder, verifier
			},
			wantErr: false,
		},
		{
			name: "tool not installed - batch install called",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
			},
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				var receivedSpecs []string
				batchInstallFunc := func(toolSpecs []string, _ bool) error {
					receivedSpecs = toolSpecs
					return nil
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "", errToolNotFound // Tool does not exist.
					},
				}
				// Verifier checks the batch received the correct tool spec.
				verifier := func() bool {
					return len(receivedSpecs) == 1 && receivedSpecs[0] == "hashicorp/terraform@1.10.0"
				}
				return resolver, batchInstallFunc, finder, verifier
			},
			wantErr: false,
		},
		{
			name: "install failure propagates error",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
			},
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				batchInstallFunc := func([]string, bool) error {
					return errInstallFailed
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "", errToolNotFound
					},
				}
				return resolver, batchInstallFunc, finder, nil
			},
			wantErr: true,
			errIs:   errUtils.ErrToolInstall,
		},
		{
			name: "multiple tools - batch install called once with all tools",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
				"cloudposse/atmos":    "1.0.0",
			},
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						parts := strings.Split(toolName, "/")
						if len(parts) == 2 {
							return parts[0], parts[1], nil
						}
						return "", "", errInvalidTool
					},
				}
				batchCallCount := 0
				var receivedSpecs []string
				batchInstallFunc := func(toolSpecs []string, _ bool) error {
					batchCallCount++
					receivedSpecs = toolSpecs
					return nil
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "", errToolNotFound // All tools need install.
					},
				}
				// Verifier: batch install called once with 2 tools.
				verifier := func() bool {
					return batchCallCount == 1 && len(receivedSpecs) == 2
				}
				return resolver, batchInstallFunc, finder, verifier
			},
			wantErr: false,
		},
		{
			name: "batch install error returns error",
			dependencies: map[string]string{
				"hashicorp/terraform": "1.10.0",
				"cloudposse/atmos":    "1.0.0",
			},
			setupMock: func() (*mockResolver, BatchInstallFunc, *mockBinaryPathFinder, func() bool) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						parts := strings.Split(toolName, "/")
						if len(parts) == 2 {
							return parts[0], parts[1], nil
						}
						return "", "", errInvalidTool
					},
				}
				batchInstallFunc := func([]string, bool) error {
					return errInstallFailed
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "", errToolNotFound
					},
				}
				return resolver, batchInstallFunc, finder, nil
			},
			wantErr: true,
			errIs:   errUtils.ErrToolInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, batchInstallFunc, finder, verifier := tt.setupMock()

			opts := []InstallerOption{WithResolver(resolver)}
			if batchInstallFunc != nil {
				opts = append(opts, WithBatchInstallFunc(batchInstallFunc))
			}
			if finder != nil {
				opts = append(opts, WithBinaryPathFinder(finder))
			}

			inst := NewInstaller(nil, opts...)
			err := inst.EnsureTools(tt.dependencies)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					// Use cockroachdb/errors.Is() because our error builder uses Mark()
					// which only works with cockroachdb/errors.Is(), not standard errors.Is().
					assert.True(t, cockroachErrors.Is(err, tt.errIs), "expected error %v in chain, got: %v", tt.errIs, err)
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
		name      string
		tool      string
		version   string
		config    *schema.AtmosConfiguration
		setupMock func() (*mockResolver, *mockBinaryPathFinder)
		want      bool
	}{
		{
			name:    "tool exists - returns true",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config:  nil,
			setupMock: func() (*mockResolver, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "/path/to/terraform", nil
					},
				}
				return resolver, finder
			},
			want: true,
		},
		{
			name:    "tool does not exist - returns false",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config:  nil,
			setupMock: func() (*mockResolver, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "", errToolNotFound
					},
				}
				return resolver, finder
			},
			want: false,
		},
		{
			name:    "resolver error - returns false",
			tool:    "unknown-tool",
			version: "1.0.0",
			config:  nil,
			setupMock: func() (*mockResolver, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "", "", errToolNotFound
					},
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "/path/to/binary", nil // Should not be called due to resolver error.
					},
				}
				return resolver, finder
			},
			want: false,
		},
		{
			name:    "FindBinaryPath receives correct arguments",
			tool:    "hashicorp/terraform",
			version: "1.10.0",
			config:  nil,
			setupMock: func() (*mockResolver, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						// Verify the correct arguments are passed.
						if owner == "hashicorp" && repo == "terraform" && version == "1.10.0" {
							return "/path/to/terraform", nil
						}
						return "", errToolNotFound
					},
				}
				return resolver, finder
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, finder := tt.setupMock()

			inst := NewInstaller(tt.config,
				WithResolver(resolver),
				WithBinaryPathFinder(finder),
			)

			got := inst.isToolInstalled(tt.tool, tt.version)
			assert.Equal(t, tt.want, got)
		})
	}
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
		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist")
		assert.False(t, fileExists(nonExistent))
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

// TestIsToolInstalled_DifferentBinaryName tests that isToolInstalled correctly
// detects tools when the binary name differs from the repo name.
// This test is expected to FAIL with the current implementation because it
// manually constructs the path using 'repo' as the binary name.
func TestIsToolInstalled_DifferentBinaryName(t *testing.T) {
	t.Run("detects tool with different binary name", func(t *testing.T) {
		// Create temp directory structure with binary named differently than repo.
		tmpDir := t.TempDir()
		toolsDir := filepath.Join(tmpDir, "tools")
		binDir := filepath.Join(toolsDir, "bin", "opentofu", "opentofu", "1.9.0")
		err := os.MkdirAll(binDir, 0o755)
		require.NoError(t, err)

		// Create binary named 'tofu' (not 'opentofu').
		// On Windows, binaries need .exe extension to be recognized as executables.
		binaryName := "tofu"
		if runtime.GOOS == "windows" {
			binaryName = "tofu.exe"
		}
		binaryPath := filepath.Join(binDir, binaryName)
		err = os.WriteFile(binaryPath, []byte("#!/bin/sh\necho tofu"), 0o755)
		require.NoError(t, err)

		// Set up resolver that maps 'opentofu' to 'opentofu/opentofu'.
		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				if toolName == "opentofu" || toolName == "opentofu/opentofu" {
					return "opentofu", "opentofu", nil
				}
				return "", "", errToolNotFound
			},
		}

		config := &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				InstallPath: toolsDir,
			},
		}

		inst := NewInstaller(config, WithResolver(resolver))

		// This should return true because the binary exists (even though named 'tofu' not 'opentofu').
		// Current implementation: FAILS because it checks for .../opentofu/opentofu/1.9.0/opentofu
		// Fixed implementation: PASSES because it uses FindBinaryPath() which auto-detects.
		result := inst.isToolInstalled("opentofu", "1.9.0")
		assert.True(t, result, "isToolInstalled should detect binary even when named differently than repo")
	})
}

// duplicateInstallTestCase defines test parameters for EnsureTools duplicate prevention tests.
type duplicateInstallTestCase struct {
	name            string
	owner           string
	repo            string
	version         string
	binaryName      string // Binary name to create (may differ from repo).
	aliases         []string
	wantInstalls    int
	wantInstallDesc string
}

// TestEnsureTools_DuplicateAliasAndCanonical tests that EnsureTools doesn't
// install the same tool twice when both an alias and canonical name are provided.
// This test exposes the issue where .tool-versions has both 'gum' and 'charmbracelet/gum'.
func TestEnsureTools_DuplicateAliasAndCanonical(t *testing.T) {
	tests := []duplicateInstallTestCase{
		{
			name:            "installs only once for alias and canonical name with real filesystem",
			owner:           "charmbracelet",
			repo:            "gum",
			version:         "0.17.0",
			binaryName:      "gum", // Binary name matches repo.
			aliases:         []string{"gum", "charmbracelet/gum"},
			wantInstalls:    1,
			wantInstallDesc: "install should be called exactly once for duplicate alias/canonical entries",
		},
		{
			name:            "installs once when binary name differs from repo",
			owner:           "opentofu",
			repo:            "opentofu",
			version:         "1.9.0",
			binaryName:      "tofu", // Binary name differs from repo.
			aliases:         []string{"opentofu", "opentofu/opentofu"},
			wantInstalls:    1,
			wantInstallDesc: "install should be called once (FindBinaryPath auto-detects binary)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runDuplicateInstallTest(t, &tc)
		})
	}
}

// runDuplicateInstallTest executes a single duplicate install test case.
func runDuplicateInstallTest(t *testing.T, tc *duplicateInstallTestCase) {
	t.Helper()

	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	binDir := filepath.Join(toolsDir, "bin", tc.owner, tc.repo, tc.version)
	err := os.MkdirAll(binDir, 0o755)
	require.NoError(t, err)

	batchCallCount := 0

	resolver := &mockResolver{
		resolveFunc: func(toolName string) (string, string, error) {
			for _, alias := range tc.aliases {
				if toolName == alias {
					return tc.owner, tc.repo, nil
				}
			}
			return "", "", errToolNotFound
		},
	}

	batchInstallFunc := func(toolSpecs []string, _ bool) error {
		batchCallCount++
		// Simulate installing all tools in the batch.
		for range toolSpecs {
			binaryPath := filepath.Join(binDir, tc.binaryName)
			if writeErr := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho "+tc.binaryName), 0o755); writeErr != nil {
				return writeErr
			}
		}
		return nil
	}

	// Mock BinaryPathFinder to detect installed binaries (needed for opentofu where binary != repo).
	finder := &mockBinaryPathFinder{
		findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
			binaryPath := filepath.Join(binDir, tc.binaryName)
			if _, err := os.Stat(binaryPath); err == nil {
				return binaryPath, nil
			}
			return "", errToolNotFound
		},
	}

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{InstallPath: toolsDir},
	}

	inst := NewInstaller(config,
		WithResolver(resolver),
		WithBatchInstallFunc(batchInstallFunc),
		WithBinaryPathFinder(finder),
	)

	deps := make(map[string]string)
	for _, alias := range tc.aliases {
		deps[alias] = tc.version
	}

	err = inst.EnsureTools(deps)
	require.NoError(t, err)
	assert.Equal(t, tc.wantInstalls, batchCallCount, tc.wantInstallDesc)
}

func TestEnsureTool(t *testing.T) {
	t.Run("does not install when tool already installed", func(t *testing.T) {
		installCalled := false

		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				return "hashicorp", "terraform", nil
			},
		}
		installFunc := func(string, bool, bool, bool, bool) error {
			installCalled = true
			return nil
		}
		finder := &mockBinaryPathFinder{
			findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
				return "/path/to/terraform", nil // Tool already installed.
			},
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithInstallFunc(installFunc),
			WithBinaryPathFinder(finder),
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
		installFunc := func(toolSpec string, _, _, _, _ bool) error {
			installedSpec = toolSpec
			return nil
		}
		finder := &mockBinaryPathFinder{
			findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
				return "", errToolNotFound // Tool not installed.
			},
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithInstallFunc(installFunc),
			WithBinaryPathFinder(finder),
		)

		err := inst.ensureTool("terraform", "1.10.0")
		require.NoError(t, err)
		assert.Equal(t, "terraform@1.10.0", installedSpec)
	})

	t.Run("returns error when install fails", func(t *testing.T) {
		var calledSpec string

		resolver := &mockResolver{
			resolveFunc: func(toolName string) (string, string, error) {
				return "hashicorp", "terraform", nil
			},
		}
		installFunc := func(toolSpec string, _, _, _, _ bool) error {
			calledSpec = toolSpec
			return errInstallFailed
		}
		finder := &mockBinaryPathFinder{
			findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
				return "", errToolNotFound
			},
		}

		inst := NewInstaller(nil,
			WithResolver(resolver),
			WithInstallFunc(installFunc),
			WithBinaryPathFinder(finder),
		)

		err := inst.ensureTool("terraform", "1.10.0")
		require.Error(t, err)
		// Use cockroachdb/errors.Is() because our error builder uses Mark()
		// which only works with cockroachdb/errors.Is(), not standard errors.Is().
		assert.True(t, cockroachErrors.Is(err, errUtils.ErrToolInstall), "expected ErrToolInstall in chain, got: %v", err)
		assert.Equal(t, "terraform@1.10.0", calledSpec, "install should be called with correct tool spec")
	})
}

// TestBuildToolchainPATH_MatchesToolchainInstallPath verifies that BuildToolchainPATH
// uses the same path as toolchain.GetInstallPath() to ensure PATH points to where
// tools are actually installed.
func TestBuildToolchainPATH_MatchesToolchainInstallPath(t *testing.T) {
	testPath := "/usr/bin:/bin"
	t.Setenv("PATH", testPath)

	// Get the expected install path from toolchain package.
	expectedInstallPath := toolchain.GetInstallPath()

	// Build PATH with a tool dependency.
	result, err := BuildToolchainPATH(nil, map[string]string{
		"hashicorp/terraform": "1.10.0",
	})
	require.NoError(t, err)

	// The expected path component for the tool.
	expectedPathComponent := filepath.Join(expectedInstallPath, "bin", "hashicorp", "terraform", "1.10.0")

	// Verify that the PATH contains the correct directory.
	assert.Contains(t, result, expectedPathComponent,
		"PATH should contain the toolchain install path (%s), not a hardcoded default like '.tools'",
		expectedInstallPath)
}

// TestBuildToolchainPATH_ConvertsRelativeToAbsolute verifies that BuildToolchainPATH
// converts relative install paths to absolute paths to avoid Go 1.19+ exec.LookPath issues.
func TestBuildToolchainPATH_ConvertsRelativeToAbsolute(t *testing.T) {
	testPath := "/usr/bin:/bin"
	t.Setenv("PATH", testPath)

	// Use a relative path in config (simulating .tools).
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: ".tools",
		},
	}

	result, err := BuildToolchainPATH(config, map[string]string{
		"hashicorp/terraform": "1.10.0",
	})
	require.NoError(t, err)

	// Get the current working directory to construct expected absolute path.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Expected absolute path for the tool.
	expectedPathComponent := filepath.Join(cwd, ".tools", "bin", "hashicorp", "terraform", "1.10.0")

	// Verify that the PATH contains the absolute path, not the relative path.
	assert.Contains(t, result, expectedPathComponent,
		"PATH should contain absolute path (%s), not relative path (.tools/bin/...)",
		expectedPathComponent)

	// Verify that the path does NOT start with a relative path component.
	pathEntries := strings.Split(result, string(os.PathListSeparator))
	for _, entry := range pathEntries {
		if strings.Contains(entry, "hashicorp/terraform") {
			assert.Truef(t, filepath.IsAbs(entry),
				"PATH entry for terraform should be absolute, got: %s", entry)
		}
	}
}
