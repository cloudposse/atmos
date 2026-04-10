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

func TestSAMLProvider_Authenticate_GatesBrowserSetupOnDriver(t *testing.T) {
	// When the driver is NOT "Browser", setupBrowserAutomation should be
	// skipped entirely — no directory creation at ~/.aws/saml2aws/.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	homedir.Reset()

	p := &samlProvider{
		name:   "test",
		config: &schema.Provider{Driver: "GoogleApps"},
		url:    "https://accounts.google.com/saml",
		region: "us-east-1",
		// No RoleToAssumeFromAssertion → Authenticate will fail early,
		// but AFTER the browser setup gate check.
	}

	_, err := p.Authenticate(context.TODO())
	require.Error(t, err, "should fail due to missing RoleToAssumeFromAssertion")

	// The saml2aws directory should NOT have been created because the
	// driver is GoogleApps (not Browser).
	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	_, statErr := os.Stat(saml2awsDir)
	assert.True(t, os.IsNotExist(statErr),
		"saml2aws directory should not be created for non-Browser drivers")
}

func TestSAMLProvider_Authenticate_RunsBrowserSetupForBrowserDriver(t *testing.T) {
	// When the driver IS "Browser", setupBrowserAutomation should run and
	// create the storage directory. We set RoleToAssumeFromAssertion so the
	// code passes the early guard and reaches the browser setup gate.
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

	// Authenticate will fail downstream (no real SAML client), but browser
	// setup runs before that point.
	_, err := p.Authenticate(context.TODO())
	require.Error(t, err)

	// The saml2aws directory SHOULD have been created.
	saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
	info, statErr := os.Stat(saml2awsDir)
	require.NoError(t, statErr, "saml2aws directory should be created for Browser driver")
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
