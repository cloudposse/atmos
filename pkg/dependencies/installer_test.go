package dependencies

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Masterminds/semver/v3"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
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

// TestBuildToolchainPATH_WithAbsolutePath verifies that BuildToolchainPATH
// handles absolute paths correctly and exercises the filepath.Abs() code path.
func TestBuildToolchainPATH_WithAbsolutePath(t *testing.T) {
	testPath := "/usr/bin:/bin"
	t.Setenv("PATH", testPath)

	// Create a temp directory to use as an absolute install path.
	tmpDir := t.TempDir()

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tmpDir,
		},
	}

	result, err := BuildToolchainPATH(config, map[string]string{
		"hashicorp/terraform": "1.10.0",
	})
	require.NoError(t, err)

	// Expected absolute path for the tool.
	expectedPathComponent := filepath.Join(tmpDir, "bin", "hashicorp", "terraform", "1.10.0")

	// Verify that the PATH contains the absolute path.
	assert.Contains(t, result, expectedPathComponent,
		"PATH should contain absolute path (%s)",
		expectedPathComponent)

	// Verify that all tool paths are absolute.
	pathEntries := strings.Split(result, string(os.PathListSeparator))
	for _, entry := range pathEntries {
		if strings.Contains(entry, "hashicorp/terraform") {
			assert.Truef(t, filepath.IsAbs(entry),
				"PATH entry for terraform should be absolute, got: %s", entry)
		}
	}
}

// TestBuildToolchainPATH_WithMultipleTools verifies PATH construction with multiple tools
// and exercises the loop that converts paths to absolute.
func TestBuildToolchainPATH_WithMultipleTools(t *testing.T) {
	testPath := "/usr/bin:/bin"
	t.Setenv("PATH", testPath)

	// Use a relative path to exercise the filepath.Abs() conversion.
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: ".tools",
		},
	}

	result, err := BuildToolchainPATH(config, map[string]string{
		"hashicorp/terraform": "1.10.0",
		"cloudposse/atmos":    "1.0.0",
	})
	require.NoError(t, err)

	// Get the current working directory.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Expected absolute paths for both tools.
	expectedTerraformPath := filepath.Join(cwd, ".tools", "bin", "hashicorp", "terraform", "1.10.0")
	expectedAtmosPath := filepath.Join(cwd, ".tools", "bin", "cloudposse", "atmos", "1.0.0")

	// Verify both paths are included.
	assert.Contains(t, result, expectedTerraformPath,
		"PATH should contain terraform absolute path")
	assert.Contains(t, result, expectedAtmosPath,
		"PATH should contain atmos absolute path")

	// Verify all entries are absolute paths.
	pathEntries := strings.Split(result, string(os.PathListSeparator))
	for _, entry := range pathEntries {
		if strings.Contains(entry, ".tools") || strings.Contains(entry, "hashicorp") || strings.Contains(entry, "cloudposse") {
			assert.Truef(t, filepath.IsAbs(entry),
				"PATH entry should be absolute, got: %s", entry)
		}
	}
}

// TestBuildToolchainPATH_SkipsInvalidTools verifies that invalid tool specifications
// are skipped without causing errors, and remaining valid tools are included.
func TestBuildToolchainPATH_SkipsInvalidTools(t *testing.T) {
	testPath := "/usr/bin:/bin"
	t.Setenv("PATH", testPath)

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: ".tools",
		},
	}

	// Mix valid and invalid tool names.
	result, err := BuildToolchainPATH(config, map[string]string{
		"hashicorp/terraform": "1.10.0",
		"invalid-tool":        "1.0.0", // Will be skipped by resolver.
	})
	require.NoError(t, err)

	// Get the current working directory.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Expected absolute path for valid tool.
	expectedTerraformPath := filepath.Join(cwd, ".tools", "bin", "hashicorp", "terraform", "1.10.0")

	// Verify the valid tool path is included.
	assert.Contains(t, result, expectedTerraformPath,
		"PATH should contain terraform absolute path")

	// Verify invalid tool doesn't cause empty PATH.
	assert.NotEqual(t, testPath, result,
		"PATH should include terraform path, not just original PATH")
}

func TestResolveExecutablePath(t *testing.T) {
	t.Run("returns absolute path unchanged", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{}),
			WithBinaryPathFinder(&mockBinaryPathFinder{}),
		)
		result := inst.ResolveExecutablePath(map[string]string{"opentofu": "1.8.0"}, "/usr/bin/tofu")
		assert.Equal(t, "/usr/bin/tofu", result)
	})

	t.Run("resolves bare executable from toolchain", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					if toolName == "opentofu" {
						return "opentofu", "opentofu", nil
					}
					return "", "", errToolNotFound
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{
				findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
					if owner == "opentofu" && repo == "opentofu" && version == "1.8.0" {
						return "/home/user/.local/share/atmos/toolchain/bin/opentofu/opentofu/1.8.0/tofu", nil
					}
					return "", errToolNotFound
				},
			}),
		)

		deps := map[string]string{"opentofu": "1.8.0"}
		result := inst.ResolveExecutablePath(deps, "tofu")
		assert.Equal(t, "/home/user/.local/share/atmos/toolchain/bin/opentofu/opentofu/1.8.0/tofu", result)
	})

	t.Run("returns original name when not found in toolchain or PATH", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "opentofu", "opentofu", nil
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{
				findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
					return "", errToolNotFound
				},
			}),
		)

		deps := map[string]string{"opentofu": "1.8.0"}
		result := inst.ResolveExecutablePath(deps, "nonexistent-binary")
		assert.Equal(t, "nonexistent-binary", result)
	})

	t.Run("skips deps with resolver errors", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "", "", errInvalidTool
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{}),
		)

		deps := map[string]string{"badtool": "1.0.0"}
		result := inst.ResolveExecutablePath(deps, "somebinary")
		assert.Equal(t, "somebinary", result)
	})

	t.Run("no-op with empty deps", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{}),
			WithBinaryPathFinder(&mockBinaryPathFinder{}),
		)
		result := inst.ResolveExecutablePath(map[string]string{}, "terraform")
		// Falls through to exec.LookPath, which should find terraform if on PATH,
		// otherwise returns original name.
		assert.NotEmpty(t, result)
	})

	t.Run("matches binary with platform extension against bare executable name", func(t *testing.T) {
		// On Windows, FindBinaryPath returns a path like /path/tofu.exe but the
		// executable is specified as bare "tofu". The comparison should still match.
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					if toolName == "opentofu" {
						return "opentofu", "opentofu", nil
					}
					return "", "", errToolNotFound
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{
				findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
					if owner == "opentofu" && repo == "opentofu" && version == "1.8.0" {
						return "/home/user/.atmos/bin/opentofu/opentofu/1.8.0/tofu.exe", nil
					}
					return "", errToolNotFound
				},
			}),
		)

		deps := map[string]string{"opentofu": "1.8.0"}
		result := inst.ResolveExecutablePath(deps, "tofu")
		assert.Equal(t, "/home/user/.atmos/bin/opentofu/opentofu/1.8.0/tofu.exe", result)
	})
}

// mockVersionLister is a mock implementation of VersionLister for testing.
type mockVersionLister struct {
	getAvailableVersionsFunc func(owner, repo string) ([]string, error)
}

func (m *mockVersionLister) GetAvailableVersions(owner, repo string) ([]string, error) {
	if m.getAvailableVersionsFunc != nil {
		return m.getAvailableVersionsFunc(owner, repo)
	}
	return nil, errors.New("not implemented")
}

// mockInstalledVersionLister is a mock implementation of InstalledVersionLister for testing.
type mockInstalledVersionLister struct {
	listInstalledVersionsFunc func(owner, repo string) ([]string, error)
}

func (m *mockInstalledVersionLister) ListInstalledVersions(owner, repo string) ([]string, error) {
	if m.listInstalledVersionsFunc != nil {
		return m.listInstalledVersionsFunc(owner, repo)
	}
	return nil, errors.New("not implemented")
}

// TestIsConstraint tests the isConstraint helper function.
func TestIsConstraint(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"", false},
		{"latest", false},
		{"1.10.0", false},
		{"v1.10.0", false},
		{"0.1.0", false},
		{"^1.10.0", true},
		{"~> 1.10.0", true},
		{">= 1.9.0", true},
		{">= 1.9.0, < 2.0.0", true},
		{"1.10.0 || 1.11.0", true},
		{"> 1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("version=%q", tt.version), func(t *testing.T) {
			got := isConstraint(tt.version)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHighestMatch tests the highestMatch helper function.
func TestHighestMatch(t *testing.T) {
	tests := []struct {
		name       string
		candidates []string
		constraint string
		want       string
		wantErr    bool
	}{
		{
			name:       "finds highest matching version",
			candidates: []string{"1.10.0", "1.10.1", "1.10.3", "1.9.0"},
			constraint: "^1.10.0",
			want:       "1.10.3",
		},
		{
			name:       "handles v-prefixed versions",
			candidates: []string{"v1.10.0", "v1.10.1"},
			constraint: "^1.10.0",
			want:       "1.10.1",
		},
		{
			name:       "no matching version returns error",
			candidates: []string{"2.0.0", "3.0.0"},
			constraint: "^1.10.0",
			wantErr:    true,
		},
		{
			name:       "empty candidates returns error",
			candidates: []string{},
			constraint: "^1.10.0",
			wantErr:    true,
		},
		{
			name:       "all unparseable candidates returns error",
			candidates: []string{"invalid", "not-semver"},
			constraint: "^1.0.0",
			wantErr:    true,
		},
		{
			name:       "skips unparseable candidates and finds match",
			candidates: []string{"1.10.0", "invalid", "1.10.3"},
			constraint: "^1.10.0",
			want:       "1.10.3",
		},
		{
			name:       "sorts correctly regardless of input order",
			candidates: []string{"1.10.3", "1.10.1", "1.10.0"},
			constraint: "^1.10.0",
			want:       "1.10.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraint, err := semver.NewConstraint(tt.constraint)
			require.NoError(t, err)

			got, err := highestMatch(tt.candidates, constraint)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, cockroachErrors.Is(err, errUtils.ErrDependencyConstraint))
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestIsRetryableHTTPError tests the isRetryableHTTPError predicate.
func TestIsRetryableHTTPError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"non-HTTP error", errors.New("random error"), false},
		{"non-HTTP sentinel error", errUtils.ErrDependencyConstraint, false},
		{"HTTP connection reset", fmt.Errorf("%w: connection reset by peer", registry.ErrHTTPRequest), true},
		{"HTTP timeout", fmt.Errorf("%w: request timeout", registry.ErrHTTPRequest), true},
		{"HTTP connection refused", fmt.Errorf("%w: connection refused", registry.ErrHTTPRequest), true},
		{"HTTP EOF", fmt.Errorf("%w: unexpected EOF", registry.ErrHTTPRequest), true},
		{"HTTP too many requests", fmt.Errorf("%w: too many requests", registry.ErrHTTPRequest), true},
		{"HTTP service unavailable", fmt.Errorf("%w: service unavailable", registry.ErrHTTPRequest), true},
		{"HTTP bad gateway", fmt.Errorf("%w: bad gateway", registry.ErrHTTPRequest), true},
		{"HTTP gateway timeout", fmt.Errorf("%w: gateway timeout", registry.ErrHTTPRequest), true},
		{"HTTP internal server error", fmt.Errorf("%w: internal server error", registry.ErrHTTPRequest), true},
		{"HTTP failed to read response body", fmt.Errorf("%w: failed to read response body", registry.ErrHTTPRequest), true},
		{"HTTP TLS error", fmt.Errorf("%w: tls handshake failure", registry.ErrHTTPRequest), true},
		{"HTTP rate limit", fmt.Errorf("%w: rate limit exceeded", registry.ErrHTTPRequest), true},
		{"HTTP 404 not retryable", fmt.Errorf("%w: HTTP 404: not found", registry.ErrHTTPRequest), false},
		{"HTTP 403 not retryable", fmt.Errorf("%w: HTTP 403: forbidden", registry.ErrHTTPRequest), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableHTTPError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFindInstalledMatch tests the findInstalledMatch method.
func TestFindInstalledMatch(t *testing.T) {
	tests := []struct {
		name          string
		tool          string
		constraintStr string
		setupMock     func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder)
		wantVersion   string
		wantFound     bool
	}{
		{
			name:          "invalid constraint returns not found",
			tool:          "hashicorp/terraform",
			constraintStr: "not-a-valid-constraint[",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				return &mockResolver{}, &mockInstalledVersionLister{}, &mockBinaryPathFinder{}
			},
			wantFound: false,
		},
		{
			name:          "resolver error returns not found",
			tool:          "unknown-tool",
			constraintStr: "^1.10.0",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "", "", errToolNotFound
					},
				}
				return resolver, &mockInstalledVersionLister{}, &mockBinaryPathFinder{}
			},
			wantFound: false,
		},
		{
			name:          "no installed versions returns not found",
			tool:          "hashicorp/terraform",
			constraintStr: "^1.10.0",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				lister := &mockInstalledVersionLister{
					listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
						return nil, nil
					},
				}
				return resolver, lister, &mockBinaryPathFinder{}
			},
			wantFound: false,
		},
		{
			name:          "installed versions don't satisfy constraint",
			tool:          "hashicorp/terraform",
			constraintStr: "^1.10.0",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				lister := &mockInstalledVersionLister{
					listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
						return []string{"1.9.0", "1.8.0"}, nil
					},
				}
				return resolver, lister, &mockBinaryPathFinder{}
			},
			wantFound: false,
		},
		{
			name:          "binary not found despite version match (corrupt install)",
			tool:          "hashicorp/terraform",
			constraintStr: "^1.10.0",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				lister := &mockInstalledVersionLister{
					listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
						return []string{"1.10.3"}, nil
					},
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "", errToolNotFound
					},
				}
				return resolver, lister, finder
			},
			wantFound: false,
		},
		{
			name:          "happy path - finds highest installed match",
			tool:          "hashicorp/terraform",
			constraintStr: "^1.10.0",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				lister := &mockInstalledVersionLister{
					listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
						return []string{"1.10.0", "1.10.3", "1.9.0"}, nil
					},
				}
				finder := &mockBinaryPathFinder{
					findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
						return "/path/to/terraform", nil
					},
				}
				return resolver, lister, finder
			},
			wantVersion: "1.10.3",
			wantFound:   true,
		},
		{
			name:          "ListInstalledVersions error returns not found",
			tool:          "hashicorp/terraform",
			constraintStr: "^1.10.0",
			setupMock: func() (*mockResolver, *mockInstalledVersionLister, *mockBinaryPathFinder) {
				resolver := &mockResolver{
					resolveFunc: func(toolName string) (string, string, error) {
						return "hashicorp", "terraform", nil
					},
				}
				lister := &mockInstalledVersionLister{
					listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
						return nil, errors.New("disk error")
					},
				}
				return resolver, lister, &mockBinaryPathFinder{}
			},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, lister, finder := tt.setupMock()

			inst := NewInstaller(nil,
				WithResolver(resolver),
				WithInstalledVersionLister(lister),
				WithBinaryPathFinder(finder),
			)

			version, found := inst.findInstalledMatch(tt.tool, tt.constraintStr)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantVersion, version)
		})
	}
}

// TestResolveConstraints tests the resolveConstraints method.
func TestResolveConstraints(t *testing.T) {
	t.Run("no constraints leaves deps unchanged", func(t *testing.T) {
		deps := map[string]string{
			"hashicorp/terraform": "1.10.0",
			"opentofu/opentofu":   "1.8.0",
		}

		inst := NewInstaller(nil,
			WithResolver(&mockResolver{}),
		)

		err := inst.resolveConstraints(deps)
		require.NoError(t, err)
		assert.Equal(t, "1.10.0", deps["hashicorp/terraform"])
		assert.Equal(t, "1.8.0", deps["opentofu/opentofu"])
	})

	t.Run("fast path - installed version satisfies constraint", func(t *testing.T) {
		deps := map[string]string{
			"hashicorp/terraform": "^1.10.0",
		}

		versionListerCalled := false
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "hashicorp", "terraform", nil
				},
			}),
			WithInstalledVersionLister(&mockInstalledVersionLister{
				listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
					return []string{"1.10.0", "1.10.3"}, nil
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{
				findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
					return "/path/to/terraform", nil
				},
			}),
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					versionListerCalled = true
					return nil, errors.New("should not be called")
				},
			}),
		)

		err := inst.resolveConstraints(deps)
		require.NoError(t, err)
		assert.Equal(t, "1.10.3", deps["hashicorp/terraform"])
		assert.False(t, versionListerCalled, "VersionLister should not be called on fast path")
	})

	t.Run("mixed constraints and concrete versions", func(t *testing.T) {
		deps := map[string]string{
			"hashicorp/terraform": "^1.10.0",
			"cloudposse/atmos":    "1.0.0", // Concrete, should not change.
		}

		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					parts := strings.Split(toolName, "/")
					if len(parts) == 2 {
						return parts[0], parts[1], nil
					}
					return "", "", errToolNotFound
				},
			}),
			WithInstalledVersionLister(&mockInstalledVersionLister{
				listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
					if repo == "terraform" {
						return []string{"1.10.5"}, nil
					}
					return nil, nil
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{
				findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
					return "/path/to/" + repo, nil
				},
			}),
		)

		err := inst.resolveConstraints(deps)
		require.NoError(t, err)
		assert.Equal(t, "1.10.5", deps["hashicorp/terraform"])
		assert.Equal(t, "1.0.0", deps["cloudposse/atmos"])
	})
}

// TestResolveOneConstraint tests the resolveOneConstraint method.
func TestResolveOneConstraint(t *testing.T) {
	t.Run("invalid constraint returns error", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{}),
		)

		_, err := inst.resolveOneConstraint("terraform", "invalid[constraint")
		require.Error(t, err)
		assert.True(t, cockroachErrors.Is(err, errUtils.ErrDependencyConstraint))
	})

	t.Run("resolver error returns error", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "", "", errToolNotFound
				},
			}),
		)

		_, err := inst.resolveOneConstraint("unknown-tool", "^1.0.0")
		require.Error(t, err)
		assert.True(t, cockroachErrors.Is(err, errUtils.ErrDependencyResolution))
	})

	t.Run("happy path resolves highest matching version", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "hashicorp", "terraform", nil
				},
			}),
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					return []string{"1.10.0", "1.10.1", "1.10.3", "1.9.0"}, nil
				},
			}),
		)

		resolved, err := inst.resolveOneConstraint("terraform", "^1.10.0")
		require.NoError(t, err)
		assert.Equal(t, "1.10.3", resolved)
	})

	t.Run("no matching version returns error", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "hashicorp", "terraform", nil
				},
			}),
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					return []string{"2.0.0", "3.0.0"}, nil
				},
			}),
		)

		_, err := inst.resolveOneConstraint("terraform", "^1.10.0")
		require.Error(t, err)
		assert.True(t, cockroachErrors.Is(err, errUtils.ErrDependencyConstraint))
	})

	t.Run("version lister error returns error", func(t *testing.T) {
		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "hashicorp", "terraform", nil
				},
			}),
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					return nil, errors.New("network error")
				},
			}),
		)

		_, err := inst.resolveOneConstraint("terraform", "^1.10.0")
		require.Error(t, err)
		assert.True(t, cockroachErrors.Is(err, errUtils.ErrDependencyResolution))
	})
}

// TestFetchAvailableVersionsWithRetry tests retry logic for version fetching.
func TestFetchAvailableVersionsWithRetry(t *testing.T) {
	t.Run("success on first attempt", func(t *testing.T) {
		expected := []string{"1.10.0", "1.10.3"}
		inst := NewInstaller(nil,
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					return expected, nil
				},
			}),
		)

		versions, err := inst.fetchAvailableVersionsWithRetry("hashicorp", "terraform", "^1.10.0")
		require.NoError(t, err)
		assert.Equal(t, expected, versions)
	})

	t.Run("non-retryable error fails immediately", func(t *testing.T) {
		callCount := int32(0)
		inst := NewInstaller(nil,
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					atomic.AddInt32(&callCount, 1)
					// Non-HTTP error: isRetryableHTTPError returns false.
					return nil, errors.New("permanent error")
				},
			}),
		)

		_, err := inst.fetchAvailableVersionsWithRetry("hashicorp", "terraform", "^1.10.0")
		require.Error(t, err)
		assert.True(t, cockroachErrors.Is(err, errUtils.ErrDependencyResolution))
		// Should only be called once since error is not retryable.
		assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
	})

	t.Run("HTTP 404 not retried", func(t *testing.T) {
		callCount := int32(0)
		inst := NewInstaller(nil,
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					atomic.AddInt32(&callCount, 1)
					return nil, fmt.Errorf("%w: HTTP 404: not found", registry.ErrHTTPRequest)
				},
			}),
		)

		_, err := inst.fetchAvailableVersionsWithRetry("hashicorp", "terraform", "^1.10.0")
		require.Error(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
	})
}

// TestEnsureTools_WithConstraints tests EnsureTools with version constraint resolution.
func TestEnsureTools_WithConstraints(t *testing.T) {
	t.Run("constraint resolved and installed", func(t *testing.T) {
		var installedSpecs []string

		inst := NewInstaller(nil,
			WithResolver(&mockResolver{
				resolveFunc: func(toolName string) (string, string, error) {
					return "hashicorp", "terraform", nil
				},
			}),
			WithInstalledVersionLister(&mockInstalledVersionLister{
				listInstalledVersionsFunc: func(owner, repo string) ([]string, error) {
					// No installed versions matching the constraint.
					return nil, nil
				},
			}),
			WithVersionLister(&mockVersionLister{
				getAvailableVersionsFunc: func(owner, repo string) ([]string, error) {
					return []string{"1.10.0", "1.10.3"}, nil
				},
			}),
			WithBinaryPathFinder(&mockBinaryPathFinder{
				findBinaryPathFunc: func(owner, repo, version string, binaryName ...string) (string, error) {
					return "", errToolNotFound // Not installed yet.
				},
			}),
			WithBatchInstallFunc(func(toolSpecs []string, _ bool) error {
				installedSpecs = toolSpecs
				return nil
			}),
		)

		deps := map[string]string{"hashicorp/terraform": "^1.10.0"}
		err := inst.EnsureTools(deps)
		require.NoError(t, err)

		// Constraint should be resolved to concrete version.
		assert.Equal(t, "1.10.3", deps["hashicorp/terraform"])
		// The resolved version should have been passed to batch install.
		require.Len(t, installedSpecs, 1)
		assert.Equal(t, "hashicorp/terraform@1.10.3", installedSpecs[0])
	})
}
