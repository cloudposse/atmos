package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider is an interface for AWS providers that support logout.
type Provider interface {
	Logout(ctx context.Context) error
}

// testProviderLogoutWithFilesystemVerification tests provider logout with filesystem cleanup verification.
// This helper reduces code duplication between SAML and SSO provider tests.
func testProviderLogoutWithFilesystemVerification(
	t *testing.T,
	providerCfg *schema.Provider,
	providerName string,
	provider Provider,
	expectError bool,
) {
	t.Helper()

	// Get the base path from provider config.
	basePath := awsCloud.GetFilesBasePath(providerCfg)

	// Create file manager to determine the provider directory path.
	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	require.NoError(t, err)

	// Construct provider directory path.
	// GetCredentialsPath returns baseDir/providerName/credentials, so we get its parent.
	credPath := fileManager.GetCredentialsPath(providerName)
	providerDir := filepath.Dir(credPath)

	// Create provider directory with some test files to verify cleanup.
	err = os.MkdirAll(providerDir, awsCloud.PermissionRWX)
	require.NoError(t, err)

	// Create test files in provider directory.
	testFile := filepath.Join(providerDir, "credentials")
	err = os.WriteFile(testFile, []byte("test content"), awsCloud.PermissionRW)
	require.NoError(t, err)

	// Verify files exist before logout.
	_, err = os.Stat(providerDir)
	require.NoError(t, err, "provider directory should exist before logout")

	ctx := context.Background()
	err = provider.Logout(ctx)

	if expectError {
		assert.Error(t, err)
	} else {
		assert.NoError(t, err)

		// Verify filesystem cleanup - provider directory should be removed.
		_, err = os.Stat(providerDir)
		assert.True(t, os.IsNotExist(err), "provider directory should be removed after logout")
	}
}
