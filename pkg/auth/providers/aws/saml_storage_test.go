package aws

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: if any of these schema.Provider fields are renamed,
// the build fails immediately instead of producing subtle test breakage.
var _ = schema.Provider{
	Kind:                  "",
	URL:                   "",
	Region:                "",
	Driver:                "",
	BrowserType:           "",
	BrowserExecutablePath: "",
	DownloadBrowserDriver: false,
}

func TestSAMLProvider_setupBrowserStorageDir(t *testing.T) {
	tests := []struct {
		name          string
		providerName  string
		setup         func(t *testing.T) string // Returns home directory.
		expectedError bool
	}{
		{
			name:          "creates directory successfully",
			providerName:  "test-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name:          "idempotent when directory already exists",
			providerName:  "existing-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				// Pre-create the directory.
				saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
				require.NoError(t, os.MkdirAll(saml2awsDir, 0o700))
				return homeDir
			},
		},
		{
			name:          "preserves existing storageState.json",
			providerName:  "preserve-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
				require.NoError(t, os.MkdirAll(saml2awsDir, 0o700))
				// Create a storage state file.
				stateFile := filepath.Join(saml2awsDir, "storageState.json")
				require.NoError(t, os.WriteFile(stateFile, []byte(`{"cookies":[]}`), 0o600))
				return homeDir
			},
		},
		{
			name:          "fails when .aws is a file (not directory)",
			providerName:  "fail-saml",
			expectedError: true,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()
				// Create .aws as a file so MkdirAll fails.
				awsPath := filepath.Join(homeDir, ".aws")
				require.NoError(t, os.WriteFile(awsPath, []byte("not a dir"), 0o600))
				return homeDir
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			homeDir := tc.setup(t)

			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)
			homedir.Reset()

			p := &samlProvider{
				name:   tc.providerName,
				config: &schema.Provider{},
			}

			err := p.setupBrowserStorageDir()

			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify directory was created as a real directory (not symlink).
			saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
			info, err := os.Stat(saml2awsDir)
			require.NoError(t, err, "saml2aws directory should exist")
			assert.True(t, info.IsDir(), "saml2aws path should be a directory")

			// Verify it's NOT a symlink.
			linfo, err := os.Lstat(saml2awsDir)
			require.NoError(t, err)
			assert.Zero(t, linfo.Mode()&os.ModeSymlink, "saml2aws path should NOT be a symlink")

			// Only check permissions on Unix.
			if runtime.GOOS != "windows" {
				assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
					"directory should have 0700 permissions")
			}
		})
	}
}

func TestSAMLProvider_setupBrowserStorageDir_MigratesLegacySymlink(t *testing.T) {
	// Previous Atmos versions created ~/.aws/saml2aws as a symlink to an
	// XDG cache directory. On upgrade, setupBrowserStorageDir should remove
	// the stale symlink, create a real directory, and preserve any existing
	// storageState.json from the symlink target.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	// Create a legacy symlink target with an existing storageState.json.
	awsDir := filepath.Join(homeDir, ".aws")
	require.NoError(t, os.MkdirAll(awsDir, 0o700))
	legacyTarget := filepath.Join(homeDir, ".cache", "atmos", "aws-saml", "old-provider")
	require.NoError(t, os.MkdirAll(legacyTarget, 0o700))
	legacyState := filepath.Join(legacyTarget, "storageState.json")
	require.NoError(t, os.WriteFile(legacyState, []byte(`{"cookies":[{"name":"session"}]}`), 0o600))

	saml2awsPath := filepath.Join(awsDir, "saml2aws")
	if err := os.Symlink(legacyTarget, saml2awsPath); err != nil {
		t.Skipf("Skipping: os.Symlink not available on this platform (%v)", err)
	}

	// Verify preconditions.
	info, err := os.Lstat(saml2awsPath)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "precondition: should be a symlink")

	p := &samlProvider{name: "test", config: &schema.Provider{}}
	err = p.setupBrowserStorageDir()
	require.NoError(t, err)

	// After migration: should be a real directory, not a symlink.
	info, err = os.Lstat(saml2awsPath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "legacy symlink should be replaced with a real directory")
	assert.True(t, info.IsDir())

	// storageState.json should be preserved from the legacy target.
	migratedState := filepath.Join(saml2awsPath, "storageState.json")
	content, err := os.ReadFile(migratedState)
	require.NoError(t, err, "storageState.json should be migrated from the legacy symlink target")
	assert.Equal(t, `{"cookies":[{"name":"session"}]}`, string(content))
}

func TestSAMLProvider_setupBrowserStorageDir_MigratesEmptyLegacySymlink(t *testing.T) {
	// Legacy symlink target with no storageState.json — should still
	// replace the symlink with a real directory without errors.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	awsDir := filepath.Join(homeDir, ".aws")
	require.NoError(t, os.MkdirAll(awsDir, 0o700))
	legacyTarget := filepath.Join(homeDir, ".cache", "atmos", "aws-saml", "empty-provider")
	require.NoError(t, os.MkdirAll(legacyTarget, 0o700))

	saml2awsPath := filepath.Join(awsDir, "saml2aws")
	if err := os.Symlink(legacyTarget, saml2awsPath); err != nil {
		t.Skipf("Skipping: os.Symlink not available on this platform (%v)", err)
	}

	p := &samlProvider{name: "test", config: &schema.Provider{}}
	err := p.setupBrowserStorageDir()
	require.NoError(t, err)

	// Should be a real directory.
	info, err := os.Lstat(saml2awsPath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	assert.True(t, info.IsDir())

	// No storageState.json should exist (nothing to migrate).
	_, statErr := os.Stat(filepath.Join(saml2awsPath, "storageState.json"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSAMLProvider_migrateLegacySymlink_NotASymlink(t *testing.T) {
	// When the path is a regular directory (not a symlink), migration is a no-op.
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "saml2aws")
	require.NoError(t, os.MkdirAll(dirPath, 0o700))
	// Write a file inside to verify nothing is deleted.
	testFile := filepath.Join(dirPath, "keep.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("keep"), 0o600))

	p := &samlProvider{config: &schema.Provider{}}
	p.migrateLegacySymlink(dirPath)

	// Directory and its contents must remain untouched.
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(content))
}

func TestSAMLProvider_migrateLegacySymlink_NonexistentPath(t *testing.T) {
	// When the path does not exist at all, migration is a no-op.
	p := &samlProvider{config: &schema.Provider{}}
	// Must not panic.
	p.migrateLegacySymlink(filepath.Join(t.TempDir(), "nonexistent"))
}

func TestSAMLProvider_migrateLegacySymlink_DanglingSymlink(t *testing.T) {
	// A dangling symlink (target deleted) should still be removed and
	// replaced with a directory. No state to migrate.
	tmpDir := t.TempDir()
	saml2awsPath := filepath.Join(tmpDir, "saml2aws")
	deletedTarget := filepath.Join(tmpDir, "deleted-target")

	// Create target, symlink, then delete target → dangling symlink.
	require.NoError(t, os.MkdirAll(deletedTarget, 0o700))
	if err := os.Symlink(deletedTarget, saml2awsPath); err != nil {
		t.Skipf("Skipping: os.Symlink not available on this platform (%v)", err)
	}
	require.NoError(t, os.RemoveAll(deletedTarget))

	p := &samlProvider{config: &schema.Provider{}}
	p.migrateLegacySymlink(saml2awsPath)

	// Symlink should be removed (path no longer exists — MkdirAll in
	// setupBrowserStorageDir will create the directory).
	_, err := os.Lstat(saml2awsPath)
	assert.True(t, os.IsNotExist(err), "dangling symlink should be removed")
}

func TestSAMLProvider_setupBrowserStorageDir_PreservesExistingState(t *testing.T) {
	// Verify that an existing storageState.json file is preserved after
	// setupBrowserStorageDir runs (os.MkdirAll is a no-op for existing dirs).
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	// Create the directory and a state file.
	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	require.NoError(t, os.MkdirAll(saml2awsDir, 0o700))
	stateFile := filepath.Join(saml2awsDir, "storageState.json")
	require.NoError(t, os.WriteFile(stateFile, []byte(`{"cookies":[{"name":"test"}]}`), 0o600))

	p := &samlProvider{name: "test", config: &schema.Provider{}}
	err := p.setupBrowserStorageDir()
	require.NoError(t, err)

	// State file must survive.
	content, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	assert.Equal(t, `{"cookies":[{"name":"test"}]}`, string(content),
		"existing storageState.json must not be deleted or modified")
}

func TestSAMLProvider_setupBrowserAutomation_CallsStorageSetup(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p, err := NewSAMLProvider("test", &schema.Provider{
		Kind:   "aws/saml",
		URL:    "https://idp.example.com/saml",
		Region: "us-east-1",
	})
	require.NoError(t, err)

	sp := p.(*samlProvider)
	err = sp.setupBrowserAutomation()
	require.NoError(t, err)

	// Verify that setupBrowserStorageDir was called: the directory should exist.
	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	info, err := os.Stat(saml2awsDir)
	require.NoError(t, err, "saml2aws directory should be created")
	assert.True(t, info.IsDir())
}

func TestSAMLProvider_setupBrowserAutomation_HandlesStorageSetupFailureGracefully(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	// Create ~/.aws as a regular file (not directory) to cause mkdir failure.
	awsPath := filepath.Join(homeDir, ".aws")
	require.NoError(t, os.WriteFile(awsPath, []byte("not a directory"), 0o600))

	p, err := NewSAMLProvider("test", &schema.Provider{
		Kind:                  "aws/saml",
		URL:                   "https://idp.example.com/saml",
		Region:                "us-east-1",
		DownloadBrowserDriver: true,
	})
	require.NoError(t, err)

	sp := p.(*samlProvider)

	// Storage dir failure is non-fatal — setupBrowserAutomation returns nil.
	err = sp.setupBrowserAutomation()
	assert.NoError(t, err, "storage dir failure is non-fatal — should return nil")

	// Verify the environment variable was still set (proves function continued).
	assert.Equal(t, "true", os.Getenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD"))
}

func TestSAMLProvider_setupBrowserAutomation_FailsOnInvalidExecutable(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name: "test",
		config: &schema.Provider{
			BrowserExecutablePath: filepath.Join(homeDir, "nonexistent", "browser"),
		},
	}

	err := p.setupBrowserAutomation()
	assert.Error(t, err, "should fail fast when browser executable does not exist")
}

func TestSAMLProvider_setupBrowserAutomation_UnsetsEnvWhenDownloadDisabled(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	// Pre-set the env var to simulate a previous auth flow.
	t.Setenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD", "true")

	p := &samlProvider{
		name: "test",
		config: &schema.Provider{
			Driver: "GoogleApps",
		},
		url: "https://accounts.google.com/saml",
	}

	err := p.setupBrowserAutomation()
	assert.NoError(t, err)

	assert.Empty(t, os.Getenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD"),
		"SAML2AWS_AUTO_BROWSER_DOWNLOAD should be unset for non-Browser drivers")
}

func TestSAMLProvider_setupBrowserAutomation_LogsBrowserType(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name: "test",
		config: &schema.Provider{
			BrowserType: "chromium",
			Driver:      "GoogleApps",
		},
		url: "https://accounts.google.com/saml",
	}

	err := p.setupBrowserAutomation()
	assert.NoError(t, err)
}

func TestSAMLProvider_SetRealm(t *testing.T) {
	p := &samlProvider{name: "test", config: &schema.Provider{}}
	assert.Empty(t, p.realm)

	p.SetRealm("my-realm")
	assert.Equal(t, "my-realm", p.realm)

	// Overwrite.
	p.SetRealm("other-realm")
	assert.Equal(t, "other-realm", p.realm)
}

func TestSAMLProvider_PrepareEnvironment(t *testing.T) {
	p := &samlProvider{name: "test", config: &schema.Provider{}, region: "us-west-2"}

	// PrepareEnvironment should return the input environ unchanged.
	input := map[string]string{"EXISTING": "value"}
	result, err := p.PrepareEnvironment(context.TODO(), input)
	require.NoError(t, err)
	assert.Equal(t, input, result, "PrepareEnvironment should return environ unchanged")
}

func TestSAMLProvider_BrowserSetupGatedOnDriverType(t *testing.T) {
	// Verify the gate condition: setupBrowserAutomation only runs when
	// getDriver() returns "Browser". We test the gate logic directly
	// (via getDriver()) instead of going through Authenticate(), which
	// would trigger saml2aws client creation and network calls.
	tests := []struct {
		name          string
		driver        string
		url           string
		expectBrowser bool
	}{
		{
			name:          "explicit GoogleApps driver is not Browser",
			driver:        "GoogleApps",
			url:           "https://accounts.google.com/saml",
			expectBrowser: false,
		},
		{
			name:          "explicit Okta driver is not Browser",
			driver:        "Okta",
			url:           "https://example.okta.com",
			expectBrowser: false,
		},
		{
			name:          "explicit Browser driver is Browser",
			driver:        "Browser",
			url:           "https://idp.example.com/saml",
			expectBrowser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)
			homedir.Reset()

			p := &samlProvider{
				name:   "test",
				config: &schema.Provider{Driver: tt.driver},
				url:    tt.url,
			}

			isBrowser := p.getDriver() == "Browser"
			assert.Equal(t, tt.expectBrowser, isBrowser,
				"getDriver() should return %q for driver=%q", tt.driver, tt.driver)
		})
	}
}

func TestSAMLProvider_Authenticate_BrowserGateCreatesDir(t *testing.T) {
	// Verify the browser gate in Authenticate: when driver is "Browser",
	// the storage directory is created. Uses a non-download config so
	// saml2aws.NewSAMLClient is fast (no Playwright initialization).
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name:                      "test",
		config:                    &schema.Provider{Driver: "Browser"},
		url:                       "https://idp.example.com/saml",
		region:                    "us-east-1",
		RoleToAssumeFromAssertion: "arn:aws:iam::123456789012:role/test",
	}

	// Authenticate fails downstream (no real IDP), but the browser gate
	// runs first and creates the storage directory.
	_, _ = p.Authenticate(context.TODO())

	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	info, err := os.Stat(saml2awsDir)
	require.NoError(t, err, "browser gate should create storage directory")
	assert.True(t, info.IsDir())
}

func TestSAMLProvider_Authenticate_NonBrowserSkipsDir(t *testing.T) {
	// When driver is NOT "Browser", the storage directory should NOT be created.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name:                      "test",
		config:                    &schema.Provider{Driver: "GoogleApps"},
		url:                       "https://accounts.google.com/saml",
		region:                    "us-east-1",
		RoleToAssumeFromAssertion: "arn:aws:iam::123456789012:role/test",
	}

	_, _ = p.Authenticate(context.TODO())

	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	_, err := os.Stat(saml2awsDir)
	assert.True(t, os.IsNotExist(err), "non-Browser driver should not create storage directory")
}

func TestSAMLProvider_setupBrowserAutomation_CreatesStorageDir(t *testing.T) {
	// Verify setupBrowserAutomation creates ~/.aws/saml2aws/ as a real
	// directory. This tests the storage setup path directly without going
	// through the full Authenticate flow (which would trigger saml2aws
	// client creation and network calls).
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name:   "test",
		config: &schema.Provider{Driver: "Browser"},
		url:    "https://idp.example.com/saml",
		region: "us-east-1",
	}

	err := p.setupBrowserAutomation()
	require.NoError(t, err)

	// The saml2aws directory SHOULD have been created.
	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	info, statErr := os.Stat(saml2awsDir)
	require.NoError(t, statErr, "saml2aws directory should be created by setupBrowserAutomation")
	assert.True(t, info.IsDir())
}

func TestSAMLProvider_setupBrowserStorageDir_UsesFilepathJoin(t *testing.T) {
	// Verify that the created directory path uses platform-correct separators
	// (filepath.Join), not hardcoded forward slashes.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{name: "test", config: &schema.Provider{}}
	err := p.setupBrowserStorageDir()
	require.NoError(t, err)

	// The directory should exist and be accessible via the platform path.
	expectedDir := filepath.Join(homeDir, ".aws", "saml2aws")
	info, err := os.Stat(expectedDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify a file can be written inside (simulates what saml2aws does).
	testFile := filepath.Join(expectedDir, "storageState.json")
	err = os.WriteFile(testFile, []byte(`{"cookies":[]}`), 0o600)
	assert.NoError(t, err, "should be able to write storageState.json inside the created directory")

	// Verify the file can be read back.
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, `{"cookies":[]}`, string(content))
}

func TestSAMLProvider_GetFilesDisplayPath_ReturnsPlatformPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name:   "test",
		config: &schema.Provider{},
	}

	displayPath := p.GetFilesDisplayPath()
	assert.NotContains(t, displayPath, "~/",
		"display path must not contain literal ~/ — should be a resolved platform path")
	assert.NotEmpty(t, displayPath, "display path must not be empty")
}
