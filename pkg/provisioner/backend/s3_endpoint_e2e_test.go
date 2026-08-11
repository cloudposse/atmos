package backend

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockaws "github.com/cloudposse/atmos/pkg/auth/providers/mock/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestE2E_CreateS3Backend_HonorsIdentityEndpoint is a regression test for the bug
// where the S3 backend provisioner never honored an identity's endpoint override
// (schema.AWSAuthContext.EndpointURL, populated by e.g. the aws/emulator identity
// kind). Unlike the other E2E tests in this package, it deliberately does NOT call
// setupFakeS3Factory/SetS3ClientFactory — it exercises the real default S3 client
// factory end-to-end, so it fails if authContext.AWS.EndpointURL stops reaching the
// constructed S3 client. Without the fix, this would silently attempt to hit real
// AWS S3 instead of the fake server.
func TestE2E_CreateS3Backend_HonorsIdentityEndpoint(t *testing.T) {
	// Setup environment for AWS credential files.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	homedir.Reset()

	// Setup fake S3 server. Deliberately do NOT call setupFakeS3Factory here: this
	// test must exercise the real, default s3ClientFactory to prove the endpoint
	// override is honored by production code, not bypassed by test wiring.
	fake := newFakeS3(t)

	// gofakes3 doesn't support encryption/public-access/tagging APIs; those are
	// covered separately with mocks in s3_test.go. This flag is independent of the
	// client factory override, so it doesn't affect what this test is verifying.
	SetS3SkipUnsupportedTestOps(true)
	t.Cleanup(func() { SetS3SkipUnsupportedTestOps(false) })

	ctx := context.Background()

	// Create mock-aws identity and populate its auth context, same as the other
	// E2E tests in this package.
	identity := mockaws.NewIdentity("emulator-endpoint-test-identity", &schema.Identity{
		Kind: "mock/aws",
		Via:  &schema.IdentityVia{Provider: "mock-provider"},
	})

	_, err := identity.Authenticate(ctx, nil)
	require.NoError(t, err, "Authenticate should succeed")

	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	err = identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "mock-provider",
		IdentityName: "emulator-endpoint-test-identity",
	})
	require.NoError(t, err, "PostAuthenticate should succeed")
	require.NotNil(t, authContext.AWS, "AWS auth context should be populated")

	// Simulate an aws/emulator identity: point the endpoint at the fake S3 server
	// instead of real AWS. This is the field CreateS3Backend must honor.
	authContext.AWS.EndpointURL = fake.server.URL

	backendConfig := map[string]any{
		"bucket": "emulator-endpoint-test-bucket",
		"region": "us-east-1",
		// gofakes3 (like most S3-compatible emulators) requires path-style
		// addressing since it isn't reachable via virtual-hosted-style DNS.
		"use_path_style": true,
	}

	_, err = CreateS3Backend(ctx, nil, backendConfig, authContext)
	require.NoError(t, err, "CreateS3Backend should succeed against the fake endpoint")

	// Verify the bucket landed on the fake server (not real AWS).
	exists, err := bucketExists(ctx, fake.client, "emulator-endpoint-test-bucket")
	require.NoError(t, err, "bucketExists should not error")
	assert.True(t, exists, "Bucket should exist on the fake server after CreateS3Backend")
}
