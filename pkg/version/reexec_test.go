package version

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// mockVersionFinder is a test mock for VersionFinder.
type mockVersionFinder struct {
	findBinaryPathFunc func(owner, repo, version string) (string, error)
	callCount          int
}

func (m *mockVersionFinder) FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error) {
	m.callCount++
	if m.findBinaryPathFunc != nil {
		return m.findBinaryPathFunc(owner, repo, version)
	}
	return "", errors.New("not found")
}

// mockVersionInstaller is a test mock for VersionInstaller.
type mockVersionInstaller struct {
	installFunc func(toolSpec string, force, allowPrereleases bool) error
	callCount   int
}

func (m *mockVersionInstaller) Install(toolSpec string, force, allowPrereleases bool) error {
	m.callCount++
	if m.installFunc != nil {
		return m.installFunc(toolSpec, force, allowPrereleases)
	}
	return nil
}

// testReexecConfig creates a ReexecConfig for testing.
func testReexecConfig(finder *mockVersionFinder, installer *mockVersionInstaller) *ReexecConfig {
	envVars := make(map[string]string)

	return &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			return nil
		},
		GetEnv: func(key string) string {
			return envVars[key]
		},
		SetEnv: func(key, value string) error {
			envVars[key] = value
			return nil
		},
		Args:    []string{"atmos", "version"},
		Environ: func() []string { return []string{} },
		// Default PR/SHA mocks that return errors (safe defaults for tests that don't exercise these paths).
		CheckPRCache: func(prNumber int) (toolchain.PRCacheStatus, string) {
			return toolchain.PRCacheNeedsInstall, ""
		},
		CheckPRUpdate: func(_ context.Context, _ int, _ bool) (bool, error) {
			return false, errors.New("mock: PR update check not configured")
		},
		InstallFromPR: func(_ int, _ bool) (string, error) {
			return "", errors.New("mock: PR install not configured")
		},
		CheckSHACache: func(_ string) (bool, string) {
			return false, ""
		},
		InstallFromSHA: func(_ string, _ bool) (string, error) {
			return "", errors.New("mock: SHA install not configured")
		},
	}
}

func TestCheckAndReexecWithConfig_NoVersionUse(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "", // No version specified.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when version.use is empty")
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath")
	assert.Equal(t, 0, installer.callCount, "Should not call Install")
}

func TestCheckAndReexecWithConfig_GuardActive(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		ReexecGuardEnvVar: "1.160.0",
	}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return envVars[key] },
		SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0", // Same as guard.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when guard is active for same version")
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath when guard active")
}

func TestCheckAndReexecWithConfig_VersionMatch(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.160.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0", // Same as current.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when versions match")
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath when versions match")
}

func TestCheckAndReexecWithConfig_VersionMatchWithVPrefix(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "v1.160.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0", // Without v prefix.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when versions match (v prefix normalized)")
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath when versions match")
}

func TestCheckAndReexecWithConfig_VersionMismatchExistingInstall(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			assert.Equal(t, "cloudposse", owner)
			assert.Equal(t, "atmos", repo)
			assert.Equal(t, "1.160.0", version)
			return "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	var execCalledWith string
	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			execCalledWith = argv0
			return nil
		},
		GetEnv:  func(key string) string { return "" },
		SetEnv:  func(key, value string) error { return nil },
		Args:    []string{"atmos", "terraform", "plan"},
		Environ: func() []string { return []string{"PATH=/usr/bin"} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when re-exec is triggered")
	assert.Equal(t, 1, finder.callCount, "Should call FindBinaryPath once")
	assert.Equal(t, 0, installer.callCount, "Should not call Install when binary exists")
	assert.Equal(t, "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", execCalledWith)
}

func TestCheckAndReexecWithConfig_VersionMismatchNeedsInstall(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	findCallCount := 0
	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			findCallCount++
			if findCallCount == 1 {
				// First call: not found.
				return "", errors.New("not found")
			}
			// Second call: found after install.
			return "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", nil
		},
	}
	installer := &mockVersionInstaller{
		installFunc: func(toolSpec string, force, allowPrereleases bool) error {
			assert.Equal(t, "atmos@1.160.0", toolSpec)
			assert.False(t, force)
			assert.False(t, allowPrereleases)
			return nil
		},
	}

	var execCalledWith string
	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			execCalledWith = argv0
			return nil
		},
		GetEnv:  func(key string) string { return "" },
		SetEnv:  func(key, value string) error { return nil },
		Args:    []string{"atmos", "terraform", "plan"},
		Environ: func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when re-exec is triggered")
	assert.Equal(t, 2, findCallCount, "Should call FindBinaryPath twice (before and after install)")
	assert.Equal(t, 1, installer.callCount, "Should call Install once")
	assert.Equal(t, "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", execCalledWith)
}

func TestCheckAndReexecWithConfig_InstallFails(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "", errors.New("not found")
		},
	}
	installer := &mockVersionInstaller{
		installFunc: func(toolSpec string, force, allowPrereleases bool) error {
			return errors.New("network error")
		},
	}
	cfg := testReexecConfig(finder, installer)

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when install fails")
	assert.Equal(t, 1, finder.callCount, "Should call FindBinaryPath once")
	assert.Equal(t, 1, installer.callCount, "Should call Install once")
}

func TestCheckAndReexecWithConfig_ExecFails(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			return errors.New("exec failed")
		},
		GetEnv:  func(key string) string { return "" },
		SetEnv:  func(key, value string) error { return nil },
		Args:    []string{"atmos", "version"},
		Environ: func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when exec fails")
}

func TestCheckAndReexecWithConfig_SetEnvFails(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return "" },
		SetEnv:    func(key, value string) error { return errors.New("setenv failed") },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when SetEnv fails")
}

func TestCheckAndReexecWithConfig_GuardIsSet(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	var guardValue string
	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return "" },
		SetEnv: func(key, value string) error {
			if key == ReexecGuardEnvVar {
				guardValue = value
			}
			return nil
		},
		Args:    []string{"atmos", "version"},
		Environ: func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.Equal(t, "1.160.0", guardValue, "Guard should be set to requested version")
}

func TestCheckAndReexecWithConfig_EnvVarVersions(t *testing.T) {
	tests := []struct {
		name       string
		envVarKey  string
		envVarName string
	}{
		{
			name:       "ATMOS_VERSION env var",
			envVarKey:  VersionEnvVar,
			envVarName: "ATMOS_VERSION",
		},
		{
			name:       "ATMOS_VERSION_USE env var",
			envVarKey:  VersionUseEnvVar,
			envVarName: "ATMOS_VERSION_USE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original version and restore after test.
			originalVersion := Version
			Version = "1.150.0"
			defer func() { Version = originalVersion }()

			finder := &mockVersionFinder{
				findBinaryPathFunc: func(owner, repo, version string) (string, error) {
					assert.Equal(t, "1.165.0", version, "Should use version from %s env var", tt.envVarName)
					return "/path/to/atmos", nil
				},
			}
			installer := &mockVersionInstaller{}

			envVars := map[string]string{
				tt.envVarKey: "1.165.0",
			}

			cfg := &ReexecConfig{
				Finder:    finder,
				Installer: installer,
				ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
				GetEnv:    func(key string) string { return envVars[key] },
				SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
				Args:      []string{"atmos", "version"},
				Environ:   func() []string { return []string{} },
			}

			atmosConfig := &schema.AtmosConfiguration{
				Version: schema.Version{
					Use: "1.160.0", // Config file value should be ignored.
				},
			}

			result := CheckAndReexecWithConfig(atmosConfig, cfg)

			assert.True(t, result, "Should return true when re-exec is triggered")
			assert.Equal(t, 1, finder.callCount, "Should call FindBinaryPath")
		})
	}
}

func TestCheckAndReexecWithConfig_EnvVarPrecedence(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			assert.Equal(t, "1.165.0", version, "ATMOS_VERSION_USE should take precedence over ATMOS_VERSION")
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		VersionUseEnvVar: "1.165.0", // ATMOS_VERSION_USE takes highest precedence.
		VersionEnvVar:    "1.170.0", // ATMOS_VERSION should be ignored.
	}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return envVars[key] },
		SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0", // Config file value should be ignored.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when re-exec is triggered")
	assert.Equal(t, 1, finder.callCount, "Should call FindBinaryPath")
}

func TestCheckAndReexecWithConfig_EnvVarFallbackToConfig(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			assert.Equal(t, "1.160.0", version, "Should fall back to config when env vars not set")
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{} // No env vars set.

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return envVars[key] },
		SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0", // Should use this value.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when re-exec is triggered")
	assert.Equal(t, 1, finder.callCount, "Should call FindBinaryPath")
}

func TestCheckAndReexecWithConfig_UseVersionFlagPrecedence(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			assert.Equal(t, "1.175.0", version, "ATMOS_VERSION_USE should take precedence over ATMOS_VERSION")
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		VersionUseEnvVar: "1.175.0", // ATMOS_VERSION_USE takes highest precedence (set by --use-version flag).
		VersionEnvVar:    "1.170.0", // ATMOS_VERSION should be ignored.
	}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return envVars[key] },
		SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
		Args:      []string{"atmos", "terraform", "plan"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0", // Config file value should be ignored.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when re-exec is triggered")
	assert.Equal(t, 1, finder.callCount, "Should call FindBinaryPath")
}

func TestFindOrInstallVersionWithConfig_ExistingInstall(t *testing.T) {
	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	path, err := findOrInstallVersionWithConfig("1.160.0", cfg)

	assert.NoError(t, err)
	assert.Equal(t, "/path/to/atmos", path)
	assert.Equal(t, 1, finder.callCount)
	assert.Equal(t, 0, installer.callCount)
}

func TestFindOrInstallVersionWithConfig_NeedsInstall(t *testing.T) {
	findCallCount := 0
	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			findCallCount++
			if findCallCount == 1 {
				return "", errors.New("not found")
			}
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	path, err := findOrInstallVersionWithConfig("1.160.0", cfg)

	assert.NoError(t, err)
	assert.Equal(t, "/path/to/atmos", path)
	assert.Equal(t, 2, findCallCount)
	assert.Equal(t, 1, installer.callCount)
}

func TestFindOrInstallVersionWithConfig_InstallFails(t *testing.T) {
	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "", errors.New("not found")
		},
	}
	installer := &mockVersionInstaller{
		installFunc: func(toolSpec string, force, allowPrereleases bool) error {
			return errors.New("network error")
		},
	}
	cfg := testReexecConfig(finder, installer)

	path, err := findOrInstallVersionWithConfig("1.160.0", cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
	assert.Empty(t, path)
}

func TestFindOrInstallVersionWithConfig_InstallSucceedsButBinaryNotFound(t *testing.T) {
	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "", errors.New("not found")
		},
	}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	path, err := findOrInstallVersionWithConfig("1.160.0", cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolNotFound)
	assert.Empty(t, path)
}

func TestStripChdirFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no chdir flags",
			args:     []string{"atmos", "terraform", "plan", "--stack", "dev"},
			expected: []string{"atmos", "terraform", "plan", "--stack", "dev"},
		},
		{
			name:     "long form with separate value",
			args:     []string{"atmos", "--chdir", "examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "long form with equals",
			args:     []string{"atmos", "--chdir=examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "short form with separate value",
			args:     []string{"atmos", "-C", "examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "short form with equals",
			args:     []string{"atmos", "-C=examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "chdir at end with separate value",
			args:     []string{"atmos", "terraform", "plan", "--chdir", "examples/demo-stacks"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "chdir at end with equals",
			args:     []string{"atmos", "terraform", "plan", "--chdir=examples/demo-stacks"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "multiple flags mixed",
			args:     []string{"atmos", "--use-version", "1.199.0", "--chdir", "examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "--use-version", "1.199.0", "terraform", "plan"},
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: []string{},
		},
		{
			name:     "only program name",
			args:     []string{"atmos"},
			expected: []string{"atmos"},
		},
		{
			name:     "chdir without value at end",
			args:     []string{"atmos", "terraform", "plan", "--chdir"},
			expected: []string{"atmos", "terraform", "plan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripChdirFlags(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckAndReexecWithConfig_StripsChdirFlags(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	var execCalledWithArgs []string
	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			execCalledWithArgs = argv
			return nil
		},
		GetEnv:  func(key string) string { return "" },
		SetEnv:  func(key, value string) error { return nil },
		Args:    []string{"atmos", "--chdir", "examples/demo-stacks", "terraform", "plan", "--use-version", "1.160.0"},
		Environ: func() []string { return []string{"PATH=/usr/bin"} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "1.160.0",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when re-exec is triggered")
	// Verify both --chdir and --use-version flags were stripped from args.
	assert.Equal(t, []string{"atmos", "terraform", "plan"}, execCalledWithArgs)
}

func TestStripUseVersionFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no use-version flags",
			args:     []string{"atmos", "terraform", "plan", "--stack", "dev"},
			expected: []string{"atmos", "terraform", "plan", "--stack", "dev"},
		},
		{
			name:     "long form with separate value",
			args:     []string{"atmos", "--use-version", "1.199.0", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "long form with equals",
			args:     []string{"atmos", "--use-version=1.199.0", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "use-version at end with separate value",
			args:     []string{"atmos", "terraform", "plan", "--use-version", "1.199.0"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "use-version at end with equals",
			args:     []string{"atmos", "terraform", "plan", "--use-version=1.199.0"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "with other flags",
			args:     []string{"atmos", "--use-version", "1.199.0", "version", "list", "--installed"},
			expected: []string{"atmos", "version", "list", "--installed"},
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: []string{},
		},
		{
			name:     "only program name",
			args:     []string{"atmos"},
			expected: []string{"atmos"},
		},
		{
			name:     "use-version without value at end",
			args:     []string{"atmos", "terraform", "plan", "--use-version"},
			expected: []string{"atmos", "terraform", "plan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripUseVersionFlags(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripBothChdirAndUseVersionFlags(t *testing.T) {
	// Test that both flag types are stripped when used together.
	args := []string{"atmos", "--chdir", "examples/demo-stacks", "--use-version", "1.199.0", "terraform", "plan"}
	result := stripChdirFlags(args)
	result = stripUseVersionFlags(result)
	assert.Equal(t, []string{"atmos", "terraform", "plan"}, result)

	// Test with equals form.
	args = []string{"atmos", "--chdir=examples/demo-stacks", "--use-version=1.199.0", "terraform", "plan"}
	result = stripChdirFlags(args)
	result = stripUseVersionFlags(result)
	assert.Equal(t, []string{"atmos", "terraform", "plan"}, result)

	// Test with mixed forms.
	args = []string{"atmos", "-C", "examples/demo-stacks", "--use-version=1.199.0", "terraform", "plan"}
	result = stripChdirFlags(args)
	result = stripUseVersionFlags(result)
	assert.Equal(t, []string{"atmos", "terraform", "plan"}, result)
}

func TestFindOrInstallVersionWithConfig_InvalidVersionFormat(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	path, err := findOrInstallVersionWithConfig("not-a-valid-version!!!", cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVersionFormatInvalid)
	assert.Empty(t, path)
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath for invalid version")
	assert.Equal(t, 0, installer.callCount, "Should not call Install for invalid version")
}

func TestResolveRequestedVersion(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		configV  string
		expected string
	}{
		{
			name:     "ATMOS_VERSION_USE takes highest precedence",
			envVars:  map[string]string{VersionUseEnvVar: "1.100.0", VersionEnvVar: "1.200.0"},
			configV:  "1.300.0",
			expected: "1.100.0",
		},
		{
			name:     "ATMOS_VERSION is second precedence",
			envVars:  map[string]string{VersionEnvVar: "1.200.0"},
			configV:  "1.300.0",
			expected: "1.200.0",
		},
		{
			name:     "config is fallback",
			envVars:  map[string]string{},
			configV:  "1.300.0",
			expected: "1.300.0",
		},
		{
			name:     "empty when nothing set",
			envVars:  map[string]string{},
			configV:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ReexecConfig{
				GetEnv: func(key string) string { return tt.envVars[key] },
			}
			atmosConfig := &schema.AtmosConfiguration{
				Version: schema.Version{Use: tt.configV},
			}

			result := resolveRequestedVersion(atmosConfig, cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldSkipReexec(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	tests := []struct {
		name             string
		requestedVersion string
		guard            string
		expected         bool
	}{
		{
			name:             "PR version never skips",
			requestedVersion: "pr:2040",
			guard:            "",
			expected:         false,
		},
		{
			name:             "SHA version never skips",
			requestedVersion: "sha:ceb7526",
			guard:            "",
			expected:         false,
		},
		{
			name:             "auto-detect PR never skips",
			requestedVersion: "2040",
			guard:            "",
			expected:         false,
		},
		{
			name:             "auto-detect SHA never skips",
			requestedVersion: "ceb7526",
			guard:            "",
			expected:         false,
		},
		{
			name:             "guard match skips",
			requestedVersion: "1.160.0",
			guard:            "1.160.0",
			expected:         true,
		},
		{
			name:             "guard mismatch does not skip",
			requestedVersion: "1.160.0",
			guard:            "1.150.0",
			expected:         false,
		},
		{
			name:             "same version skips",
			requestedVersion: "1.150.0",
			guard:            "",
			expected:         true,
		},
		{
			name:             "same version with v prefix skips",
			requestedVersion: "v1.150.0",
			guard:            "",
			expected:         true,
		},
		{
			name:             "different version does not skip",
			requestedVersion: "1.160.0",
			guard:            "",
			expected:         false,
		},
		{
			name:             "guard takes precedence over PR version",
			requestedVersion: "pr:2040",
			guard:            "pr:2040",
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ReexecConfig{
				GetEnv: func(key string) string {
					if key == ReexecGuardEnvVar {
						return tt.guard
					}
					return ""
				},
			}

			result := shouldSkipReexec(tt.requestedVersion, cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindOrInstallVersionWithConfig_EmptyBinaryPath(t *testing.T) {
	// Test when finder returns empty path without error (edge case).
	// This triggers the install path, then after install the finder returns empty again.
	findCallCount := 0
	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			findCallCount++
			if findCallCount == 1 {
				return "", nil // First call: empty path, no error -> triggers install.
			}
			return "", errors.New("not found") // Second call: not found after install.
		},
	}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	path, err := findOrInstallVersionWithConfig("1.160.0", cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolNotFound)
	assert.Empty(t, path)
	assert.Equal(t, 2, findCallCount, "Should call FindBinaryPath twice")
	assert.Equal(t, 1, installer.callCount, "Should call Install once")
}

func TestCheckAndReexecWithConfig_PRVersionGuardActive(t *testing.T) {
	// When guard matches a PR version, re-exec is skipped.
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		ReexecGuardEnvVar: "pr:2040",
	}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return envVars[key] },
		SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "pr:2040", // Same as guard.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when guard is active for PR version")
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath when guard active")
}

func TestCheckAndReexecWithConfig_SHAVersionGuardActive(t *testing.T) {
	// When guard matches a SHA version, re-exec is skipped.
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		ReexecGuardEnvVar: "sha:ceb7526",
	}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return envVars[key] },
		SetEnv:    func(key, value string) error { envVars[key] = value; return nil },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "sha:ceb7526", // Same as guard.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.False(t, result, "Should return false when guard is active for SHA version")
	assert.Equal(t, 0, finder.callCount, "Should not call FindBinaryPath when guard active")
}

func TestCheckAndReexecWithConfig_InvalidVersionFormat(t *testing.T) {
	// When version.use has an invalid format, executeVersionSwitch should
	// call ParseVersionSpec which returns an error, and the flow should
	// fall back to continuing with current version.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			return "", errors.New("not found")
		},
	}
	installer := &mockVersionInstaller{
		installFunc: func(toolSpec string, force, allowPrereleases bool) error {
			return errors.New("install failed")
		},
	}

	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn:    func(argv0 string, argv []string, envv []string) error { return nil },
		GetEnv:    func(key string) string { return "" },
		SetEnv:    func(key, value string) error { return nil },
		Args:      []string{"atmos", "version"},
		Environ:   func() []string { return []string{} },
	}

	// "latest" is valid semver in version_spec.go.
	// Use something that's a valid semver but fails install.
	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "99.99.99", // Valid semver, but install will fail.
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	// Should return false because install fails and it's a semver (fallback to current).
	assert.False(t, result, "Should return false when install fails for semver version")
}

func TestStripUseVersionFlags_AtEnd(t *testing.T) {
	// Test --use-version without value at end of args.
	args := []string{"atmos", "--use-version"}
	result := stripUseVersionFlags(args)
	assert.Equal(t, []string{"atmos"}, result)
}

func TestStripChdirFlags_ConcatenatedC(t *testing.T) {
	// Test -C without value at end of args.
	args := []string{"atmos", "-C"}
	result := stripChdirFlags(args)
	assert.Equal(t, []string{"atmos"}, result)
}

func TestFindOrInstallVersionWithConfig_PRVersion(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	// Configure mock to return a cached binary for PR.
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		assert.Equal(t, 9999, prNumber)
		return toolchain.PRCacheValid, "/path/to/pr-9999/atmos"
	}

	// "pr:9999" is a valid PR version specifier.
	path, err := findOrInstallVersionWithConfig("pr:9999", cfg)

	require.NoError(t, err)
	assert.Equal(t, "/path/to/pr-9999/atmos", path)
	// Should not call the semver finder/installer.
	assert.Equal(t, 0, finder.callCount, "Should not use semver finder for PR version")
	assert.Equal(t, 0, installer.callCount, "Should not use semver installer for PR version")
}

func TestFindOrInstallVersionWithConfig_SHAVersion(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	// Configure mock to return a cached binary for SHA.
	cfg.CheckSHACache = func(sha string) (bool, string) {
		assert.Equal(t, "ceb7526", sha)
		return true, "/path/to/sha-ceb7526/atmos"
	}

	// "sha:ceb7526" is a valid SHA version specifier.
	path, err := findOrInstallVersionWithConfig("sha:ceb7526", cfg)

	require.NoError(t, err)
	assert.Equal(t, "/path/to/sha-ceb7526/atmos", path)
	// Should not call the semver finder/installer.
	assert.Equal(t, 0, finder.callCount, "Should not use semver finder for SHA version")
	assert.Equal(t, 0, installer.callCount, "Should not use semver installer for SHA version")
}

func TestFindOrInstallVersionWithConfig_AutoDetectPRNumber(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	// Mock fresh install path.
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		assert.Equal(t, 99999, prNumber)
		return toolchain.PRCacheNeedsInstall, ""
	}
	cfg.InstallFromPR = func(prNumber int, showProgress bool) (string, error) {
		assert.Equal(t, 99999, prNumber)
		return "/path/to/pr-99999/atmos", nil
	}

	// "99999" should be parsed as a PR number.
	path, err := findOrInstallVersionWithConfig("99999", cfg)

	require.NoError(t, err)
	assert.Equal(t, "/path/to/pr-99999/atmos", path)
	assert.Equal(t, 0, finder.callCount, "Should not use semver finder for PR version")
}

func TestFindOrInstallVersionWithConfig_AutoDetectSHA(t *testing.T) {
	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}
	cfg := testReexecConfig(finder, installer)

	// Mock fresh install path.
	cfg.CheckSHACache = func(sha string) (bool, string) {
		assert.Equal(t, "ceb7526", sha)
		return false, ""
	}
	cfg.InstallFromSHA = func(sha string, showProgress bool) (string, error) {
		assert.Equal(t, "ceb7526", sha)
		return "/path/to/sha-ceb7526/atmos", nil
	}

	// "ceb7526" should be auto-detected as a SHA.
	path, err := findOrInstallVersionWithConfig("ceb7526", cfg)

	require.NoError(t, err)
	assert.Equal(t, "/path/to/sha-ceb7526/atmos", path)
	assert.Equal(t, 0, finder.callCount, "Should not use semver finder for SHA version")
}

func TestCheckAndReexecWithConfig_PRVersionReexec(t *testing.T) {
	// Test that PR version triggers full re-exec path with mocked install.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}

	var execCalledWith string
	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			execCalledWith = argv0
			return nil
		},
		GetEnv:  func(key string) string { return "" },
		SetEnv:  func(key, value string) error { return nil },
		Args:    []string{"atmos", "version"},
		Environ: func() []string { return []string{} },
		CheckPRCache: func(prNumber int) (toolchain.PRCacheStatus, string) {
			return toolchain.PRCacheValid, "/path/to/pr-9999/atmos"
		},
		CheckPRUpdate: func(_ context.Context, _ int, _ bool) (bool, error) {
			return false, nil
		},
		InstallFromPR: func(_ int, _ bool) (string, error) {
			return "/path/to/pr-9999/atmos", nil
		},
		CheckSHACache:  func(_ string) (bool, string) { return false, "" },
		InstallFromSHA: func(_ string, _ bool) (string, error) { return "", errors.New("not used") },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "pr:9999",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when PR re-exec is triggered")
	assert.Equal(t, "/path/to/pr-9999/atmos", execCalledWith)
}

func TestCheckAndReexecWithConfig_SHAVersionReexec(t *testing.T) {
	// Test that SHA version triggers full re-exec path with mocked install.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{}
	installer := &mockVersionInstaller{}

	var execCalledWith string
	cfg := &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			execCalledWith = argv0
			return nil
		},
		GetEnv:        func(key string) string { return "" },
		SetEnv:        func(key, value string) error { return nil },
		Args:          []string{"atmos", "version"},
		Environ:       func() []string { return []string{} },
		CheckPRCache:  func(_ int) (toolchain.PRCacheStatus, string) { return toolchain.PRCacheNeedsInstall, "" },
		CheckPRUpdate: func(_ context.Context, _ int, _ bool) (bool, error) { return false, nil },
		InstallFromPR: func(_ int, _ bool) (string, error) { return "", errors.New("not used") },
		CheckSHACache: func(sha string) (bool, string) {
			return true, "/path/to/sha-ceb7526/atmos"
		},
		InstallFromSHA: func(_ string, _ bool) (string, error) { return "", errors.New("not used") },
	}

	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Use: "sha:ceb7526",
		},
	}

	result := CheckAndReexecWithConfig(atmosConfig, cfg)

	assert.True(t, result, "Should return true when SHA re-exec is triggered")
	assert.Equal(t, "/path/to/sha-ceb7526/atmos", execCalledWith)
}

func TestDefaultReexecConfig(t *testing.T) {
	// Test that DefaultReexecConfig returns a valid config with all fields populated.
	cfg := DefaultReexecConfig()
	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.Finder)
	assert.NotNil(t, cfg.Installer)
	assert.NotNil(t, cfg.ExecFn)
	assert.NotNil(t, cfg.GetEnv)
	assert.NotNil(t, cfg.SetEnv)
	assert.NotNil(t, cfg.Args)
	assert.NotNil(t, cfg.Environ)
	assert.NotNil(t, cfg.CheckPRCache)
	assert.NotNil(t, cfg.CheckPRUpdate)
	assert.NotNil(t, cfg.InstallFromPR)
	assert.NotNil(t, cfg.CheckSHACache)
	assert.NotNil(t, cfg.InstallFromSHA)
}

// TestFindOrInstallPRVersionWithConfig_CacheValid tests that a valid cached PR binary is returned immediately.
func TestFindOrInstallPRVersionWithConfig_CacheValid(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		assert.Equal(t, 2040, prNumber)
		return toolchain.PRCacheValid, "/cache/pr-2040/atmos"
	}

	path, err := findOrInstallPRVersionWithConfig(2040, cfg)

	require.NoError(t, err)
	assert.Equal(t, "/cache/pr-2040/atmos", path)
}

// TestFindOrInstallPRVersionWithConfig_CacheNeedsCheck_NoReinstall tests the TTL-expired but SHA-unchanged path.
func TestFindOrInstallPRVersionWithConfig_CacheNeedsCheck_NoReinstall(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		return toolchain.PRCacheNeedsCheck, "/cache/pr-2040/atmos"
	}
	cfg.CheckPRUpdate = func(_ context.Context, prNumber int, showProgress bool) (bool, error) {
		assert.Equal(t, 2040, prNumber)
		return false, nil // SHA unchanged.
	}

	path, err := findOrInstallPRVersionWithConfig(2040, cfg)

	require.NoError(t, err)
	assert.Equal(t, "/cache/pr-2040/atmos", path)
}

// TestFindOrInstallPRVersionWithConfig_CacheNeedsCheck_Reinstall tests the TTL-expired, SHA-changed path.
func TestFindOrInstallPRVersionWithConfig_CacheNeedsCheck_Reinstall(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		return toolchain.PRCacheNeedsCheck, "/cache/pr-2040/atmos"
	}
	cfg.CheckPRUpdate = func(_ context.Context, prNumber int, _ bool) (bool, error) {
		return true, nil // SHA changed, needs reinstall.
	}
	cfg.InstallFromPR = func(prNumber int, _ bool) (string, error) {
		assert.Equal(t, 2040, prNumber)
		return "/cache/pr-2040/atmos-new", nil
	}

	path, err := findOrInstallPRVersionWithConfig(2040, cfg)

	require.NoError(t, err)
	assert.Equal(t, "/cache/pr-2040/atmos-new", path)
}

// TestFindOrInstallPRVersionWithConfig_CacheNeedsInstall tests fresh PR install.
func TestFindOrInstallPRVersionWithConfig_CacheNeedsInstall(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		return toolchain.PRCacheNeedsInstall, ""
	}
	cfg.InstallFromPR = func(prNumber int, _ bool) (string, error) {
		assert.Equal(t, 2040, prNumber)
		return "/installed/pr-2040/atmos", nil
	}

	path, err := findOrInstallPRVersionWithConfig(2040, cfg)

	require.NoError(t, err)
	assert.Equal(t, "/installed/pr-2040/atmos", path)
}

// TestFindOrInstallPRVersionWithConfig_InstallFails tests PR install failure wrapping.
func TestFindOrInstallPRVersionWithConfig_InstallFails(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		return toolchain.PRCacheNeedsInstall, ""
	}
	cfg.InstallFromPR = func(prNumber int, _ bool) (string, error) {
		return "", errors.New("artifact not found")
	}

	path, err := findOrInstallPRVersionWithConfig(2040, cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
	assert.Empty(t, path)
}

// TestFindOrInstallPRVersionWithConfig_CheckUpdateFails tests PR update check failure wrapping.
func TestFindOrInstallPRVersionWithConfig_CheckUpdateFails(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckPRCache = func(prNumber int) (toolchain.PRCacheStatus, string) {
		return toolchain.PRCacheNeedsCheck, "/cache/pr-2040/atmos"
	}
	cfg.CheckPRUpdate = func(_ context.Context, _ int, _ bool) (bool, error) {
		return false, errors.New("API rate limit")
	}

	path, err := findOrInstallPRVersionWithConfig(2040, cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVersionCheckFailed)
	assert.Empty(t, path)
}

// TestFindOrInstallSHAVersionWithConfig_CacheHit tests SHA cache hit path.
func TestFindOrInstallSHAVersionWithConfig_CacheHit(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckSHACache = func(sha string) (bool, string) {
		assert.Equal(t, "abc1234", sha)
		return true, "/cache/sha-abc1234/atmos"
	}

	path, err := findOrInstallSHAVersionWithConfig("abc1234", cfg)

	require.NoError(t, err)
	assert.Equal(t, "/cache/sha-abc1234/atmos", path)
}

// TestFindOrInstallSHAVersionWithConfig_CacheMiss tests SHA fresh install path.
func TestFindOrInstallSHAVersionWithConfig_CacheMiss(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckSHACache = func(sha string) (bool, string) {
		return false, ""
	}
	cfg.InstallFromSHA = func(sha string, _ bool) (string, error) {
		assert.Equal(t, "abc1234", sha)
		return "/installed/sha-abc1234/atmos", nil
	}

	path, err := findOrInstallSHAVersionWithConfig("abc1234", cfg)

	require.NoError(t, err)
	assert.Equal(t, "/installed/sha-abc1234/atmos", path)
}

// TestFindOrInstallSHAVersionWithConfig_InstallFails tests SHA install failure wrapping.
func TestFindOrInstallSHAVersionWithConfig_InstallFails(t *testing.T) {
	cfg := testReexecConfig(&mockVersionFinder{}, &mockVersionInstaller{})
	cfg.CheckSHACache = func(sha string) (bool, string) {
		return false, ""
	}
	cfg.InstallFromSHA = func(sha string, _ bool) (string, error) {
		return "", errors.New("download failed")
	}

	path, err := findOrInstallSHAVersionWithConfig("abc1234", cfg)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
	assert.Empty(t, path)
}
