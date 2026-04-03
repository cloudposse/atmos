package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewAWSClientCache(t *testing.T) {
	cache := newAWSClientCache()
	require.NotNil(t, cache)
	assert.NotNil(t, cache.securityHub)
	assert.NotNil(t, cache.tagging)
	assert.NotNil(t, cache.securityHubFn)
	assert.NotNil(t, cache.taggingFn)
	assert.Nil(t, cache.authContext)
}

func TestWithAuthContext(t *testing.T) {
	cache := newAWSClientCache()
	authCtx := &schema.AWSAuthContext{
		CredentialsFile: "/tmp/creds",
		ConfigFile:      "/tmp/config",
		Profile:         "test-profile",
		Region:          "us-east-2",
	}
	cache.WithAuthContext(authCtx)
	assert.Equal(t, authCtx, cache.authContext)
}

func TestWithAuthContext_Nil(t *testing.T) {
	cache := newAWSClientCache()
	cache.WithAuthContext(nil)
	assert.Nil(t, cache.authContext)
}

func TestNewAWSClientCache_ClientMaps_Empty(t *testing.T) {
	cache := newAWSClientCache()
	assert.Empty(t, cache.securityHub)
	assert.Empty(t, cache.tagging)
}
