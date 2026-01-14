package backend

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockaws "github.com/cloudposse/atmos/pkg/auth/providers/mock/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupFakeS3Factory configures the S3 client factory to use the fake S3 server.
// This allows E2E tests to use gofakes3 instead of real AWS S3.
func setupFakeS3Factory(t *testing.T, fake *fakeS3) {
	t.Helper()

	SetS3ClientFactory(func(_ aws.Config) S3ClientAPI {
		return fake.client
	})

	// Skip operations not supported by gofakes3 (encryption, public access, tags).
	// These operations are tested separately with mocks in s3_test.go.
	SetS3SkipUnsupportedTestOps(true)

	t.Cleanup(func() {
		SetS3SkipUnsupportedTestOps(false)
	})
}

// TestE2E_CreateS3Backend_WithMockAWSProvider tests the full flow:
// mock-aws provider → auth context → S3 bucket creation via production code path.
func TestE2E_CreateS3Backend_WithMockAWSProvider(t *testing.T) {
	// Setup environment for AWS credential files.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	homedir.Reset()

	// Setup fake S3 server.
	fake := newFakeS3(t)
	setupFakeS3Factory(t, fake)

	ctx := context.Background()

	// Create mock-aws identity.
	identity := mockaws.NewIdentity("e2e-test-identity", &schema.Identity{
		Kind: "mock-aws",
		Via:  &schema.IdentityVia{Provider: "mock-provider"},
	})

	// Authenticate (returns mock credentials).
	_, err := identity.Authenticate(ctx, nil)
	require.NoError(t, err, "Authenticate should succeed")

	// Setup auth context and stack info.
	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	// Call PostAuthenticate (like auth manager does).
	// This populates auth context with credential file paths.
	err = identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "mock-provider",
		IdentityName: "e2e-test-identity",
	})
	require.NoError(t, err, "PostAuthenticate should succeed")

	// Verify auth context was populated.
	require.NotNil(t, authContext.AWS, "AWS auth context should be populated")
	assert.NotEmpty(t, authContext.AWS.CredentialsFile, "CredentialsFile should be set")
	assert.NotEmpty(t, authContext.AWS.ConfigFile, "ConfigFile should be set")
	assert.Equal(t, "e2e-test-identity", authContext.AWS.Profile, "Profile should match identity name")
	assert.Equal(t, "us-east-1", authContext.AWS.Region, "Region should be set")

	// Call production code path: CreateS3Backend.
	backendConfig := map[string]any{
		"bucket": "e2e-test-bucket",
		"region": "us-east-1",
	}
	err = CreateS3Backend(ctx, nil, backendConfig, authContext)
	require.NoError(t, err, "CreateS3Backend should succeed")

	// Verify bucket was created.
	exists, err := bucketExists(ctx, fake.client, "e2e-test-bucket")
	require.NoError(t, err, "bucketExists should not error")
	assert.True(t, exists, "Bucket should exist after CreateS3Backend")
}

// TestE2E_DeleteS3Backend_WithMockAWSProvider tests the full deletion flow
// via production code path.
func TestE2E_DeleteS3Backend_WithMockAWSProvider(t *testing.T) {
	// Setup environment for AWS credential files.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	homedir.Reset()

	// Setup fake S3 server.
	fake := newFakeS3(t)
	setupFakeS3Factory(t, fake)

	ctx := context.Background()

	// Create mock-aws identity.
	identity := mockaws.NewIdentity("e2e-delete-identity", &schema.Identity{
		Kind: "mock-aws",
		Via:  &schema.IdentityVia{Provider: "mock-provider"},
	})

	// Authenticate and PostAuthenticate.
	_, err := identity.Authenticate(ctx, nil)
	require.NoError(t, err)

	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	err = identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "mock-provider",
		IdentityName: "e2e-delete-identity",
	})
	require.NoError(t, err)

	// Verify auth context was populated.
	require.NotNil(t, authContext.AWS, "AWS auth context should be populated")

	// Create bucket using production code path.
	bucketName := "e2e-delete-bucket"
	backendConfig := map[string]any{
		"bucket": bucketName,
		"region": "us-east-1",
	}
	err = CreateS3Backend(ctx, nil, backendConfig, authContext)
	require.NoError(t, err, "CreateS3Backend should succeed")

	// Verify bucket exists.
	exists, err := bucketExists(ctx, fake.client, bucketName)
	require.NoError(t, err)
	require.True(t, exists, "Bucket should exist before deletion")

	// Delete the bucket using production code path.
	err = DeleteS3Backend(ctx, nil, backendConfig, authContext, true)
	require.NoError(t, err, "DeleteS3Backend should succeed")

	// Verify bucket is deleted.
	exists, err = bucketExists(ctx, fake.client, bucketName)
	require.NoError(t, err)
	assert.False(t, exists, "Bucket should not exist after deletion")
}

// TestE2E_CreateBucket_NilAuthContext tests bucket creation without auth context.
// This verifies the S3 client factory injection works for tests that don't need auth.
func TestE2E_CreateBucket_NilAuthContext(t *testing.T) {
	// Setup fake S3 server.
	fake := newFakeS3(t)
	setupFakeS3Factory(t, fake)

	ctx := context.Background()

	// Create bucket without auth context using low-level function.
	bucketName := "nil-auth-bucket"
	err := createBucket(ctx, fake.client, bucketName, "us-east-1")
	require.NoError(t, err, "createBucket should succeed")

	// Verify bucket was created.
	exists, err := bucketExists(ctx, fake.client, bucketName)
	require.NoError(t, err)
	assert.True(t, exists, "Bucket should exist")
}

// TestE2E_EnsureBucket_IdempotentCreation tests that ensureBucket is idempotent.
func TestE2E_EnsureBucket_IdempotentCreation(t *testing.T) {
	// Setup fake S3 server.
	fake := newFakeS3(t)
	setupFakeS3Factory(t, fake)

	ctx := context.Background()
	bucketName := "idempotent-bucket"

	// First call should create the bucket.
	alreadyExisted, err := ensureBucket(ctx, fake.client, bucketName, "us-east-1")
	require.NoError(t, err, "ensureBucket should succeed on first call")
	assert.False(t, alreadyExisted, "Should indicate bucket was newly created")

	// Second call should detect bucket already exists.
	alreadyExisted, err = ensureBucket(ctx, fake.client, bucketName, "us-east-1")
	require.NoError(t, err, "ensureBucket should succeed on second call")
	assert.True(t, alreadyExisted, "Should indicate bucket already existed")
}

// TestE2E_AuthContext_EnvVarsPopulated tests that PostAuthenticate
// properly populates environment variables in stack info.
func TestE2E_AuthContext_EnvVarsPopulated(t *testing.T) {
	// Setup environment for AWS credential files.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	homedir.Reset()

	ctx := context.Background()

	// Create mock-aws identity.
	identity := mockaws.NewIdentity("env-test-identity", &schema.Identity{
		Kind: "mock-aws",
		Via:  &schema.IdentityVia{Provider: "mock-provider"},
	})

	// Authenticate.
	_, err := identity.Authenticate(ctx, nil)
	require.NoError(t, err)

	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: make(map[string]any),
	}

	err = identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "mock-provider",
		IdentityName: "env-test-identity",
	})
	require.NoError(t, err)

	// Verify environment variables were populated.
	assert.NotNil(t, stackInfo.ComponentEnvSection, "ComponentEnvSection should not be nil")

	// Check AWS-specific environment variables.
	awsProfile, ok := stackInfo.ComponentEnvSection["AWS_PROFILE"].(string)
	assert.True(t, ok, "AWS_PROFILE should be set")
	assert.Equal(t, "env-test-identity", awsProfile)

	awsRegion, ok := stackInfo.ComponentEnvSection["AWS_REGION"].(string)
	assert.True(t, ok, "AWS_REGION should be set")
	assert.Equal(t, "us-east-1", awsRegion)

	credFile, ok := stackInfo.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"].(string)
	assert.True(t, ok, "AWS_SHARED_CREDENTIALS_FILE should be set")
	assert.NotEmpty(t, credFile)
}

// TestE2E_MultipleIdentities_IsolatedCredentials tests that multiple
// mock-aws identities have isolated credential files.
func TestE2E_MultipleIdentities_IsolatedCredentials(t *testing.T) {
	// Setup environment for AWS credential files.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	homedir.Reset()

	// Setup fake S3 server.
	fake := newFakeS3(t)
	setupFakeS3Factory(t, fake)

	ctx := context.Background()

	// Create two mock-aws identities.
	identity1 := mockaws.NewIdentity("identity-1", &schema.Identity{
		Kind: "mock-aws",
		Via:  &schema.IdentityVia{Provider: "provider-1"},
	})
	identity2 := mockaws.NewIdentity("identity-2", &schema.Identity{
		Kind: "mock-aws",
		Via:  &schema.IdentityVia{Provider: "provider-2"},
	})

	// Authenticate both.
	_, err := identity1.Authenticate(ctx, nil)
	require.NoError(t, err)
	_, err = identity2.Authenticate(ctx, nil)
	require.NoError(t, err)

	// PostAuthenticate both.
	authContext1 := &schema.AuthContext{}
	err = identity1.PostAuthenticate(ctx, &types.PostAuthenticateParams{
		AuthContext:  authContext1,
		StackInfo:    &schema.ConfigAndStacksInfo{},
		ProviderName: "provider-1",
		IdentityName: "identity-1",
	})
	require.NoError(t, err)

	authContext2 := &schema.AuthContext{}
	err = identity2.PostAuthenticate(ctx, &types.PostAuthenticateParams{
		AuthContext:  authContext2,
		StackInfo:    &schema.ConfigAndStacksInfo{},
		ProviderName: "provider-2",
		IdentityName: "identity-2",
	})
	require.NoError(t, err)

	// Verify auth contexts are isolated.
	assert.NotEqual(t, authContext1.AWS.CredentialsFile, authContext2.AWS.CredentialsFile,
		"Different identities should have different credential files")
	assert.NotEqual(t, authContext1.AWS.ConfigFile, authContext2.AWS.ConfigFile,
		"Different identities should have different config files")

	// Both can create buckets (using low-level function).
	err = createBucket(ctx, fake.client, "bucket-identity-1", "us-east-1")
	require.NoError(t, err)

	err = createBucket(ctx, fake.client, "bucket-identity-2", "us-east-1")
	require.NoError(t, err)

	// Verify both buckets exist.
	exists1, err := bucketExists(ctx, fake.client, "bucket-identity-1")
	require.NoError(t, err)
	assert.True(t, exists1)

	exists2, err := bucketExists(ctx, fake.client, "bucket-identity-2")
	require.NoError(t, err)
	assert.True(t, exists2)
}
