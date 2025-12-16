package version

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
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
	execCalled := false

	return &ReexecConfig{
		Finder:    finder,
		Installer: installer,
		ExecFn: func(argv0 string, argv []string, envv []string) error {
			execCalled = true
			_ = execCalled // Mark as used.
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

func TestCheckAndReexecWithConfig_EnvVarATMOS_VERSION(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			assert.Equal(t, "1.165.0", version, "Should use version from ATMOS_VERSION env var")
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		VersionEnvVar: "1.165.0", // ATMOS_VERSION takes precedence.
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

func TestCheckAndReexecWithConfig_EnvVarATMOS_VERSION_USE(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := Version
	Version = "1.150.0"
	defer func() { Version = originalVersion }()

	finder := &mockVersionFinder{
		findBinaryPathFunc: func(owner, repo, version string) (string, error) {
			assert.Equal(t, "1.165.0", version, "Should use version from ATMOS_VERSION_USE env var")
			return "/path/to/atmos", nil
		},
	}
	installer := &mockVersionInstaller{}

	envVars := map[string]string{
		VersionUseEnvVar: "1.165.0", // ATMOS_VERSION_USE takes precedence over config.
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
	assert.Contains(t, err.Error(), "failed to install Atmos 1.160.0")
	assert.Contains(t, err.Error(), "network error")
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
	assert.Contains(t, err.Error(), "installed Atmos 1.160.0 but could not find binary")
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
	// Verify --chdir flag was stripped from args.
	assert.Equal(t, []string{"atmos", "terraform", "plan", "--use-version", "1.160.0"}, execCalledWithArgs)
}
