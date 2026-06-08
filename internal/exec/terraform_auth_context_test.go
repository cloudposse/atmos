package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetLastAuthContext_RoundTrips(t *testing.T) {
	t.Cleanup(ClearLastAuthContext)

	authCtx := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-profile",
			CredentialsFile: "/tmp/creds",
			Region:          "us-east-1",
		},
	}
	authMgr := "mock-manager"

	SetLastAuthContext(authCtx, authMgr)

	gotCtx, gotMgr := GetLastAuthContext()
	require.NotNil(t, gotCtx)
	assert.Equal(t, "test-profile", gotCtx.AWS.Profile)
	assert.Equal(t, "us-east-1", gotCtx.AWS.Region)
	assert.Equal(t, "mock-manager", gotMgr)
}

func TestGetLastAuthContext_ReturnsNilWhenUnset(t *testing.T) {
	t.Cleanup(ClearLastAuthContext)
	ClearLastAuthContext()

	gotCtx, gotMgr := GetLastAuthContext()
	assert.Nil(t, gotCtx)
	assert.Nil(t, gotMgr)
}

func TestClearLastAuthContext_ResetsState(t *testing.T) {
	authCtx := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "before-clear"},
	}
	SetLastAuthContext(authCtx, "mgr")

	ClearLastAuthContext()

	gotCtx, gotMgr := GetLastAuthContext()
	assert.Nil(t, gotCtx)
	assert.Nil(t, gotMgr)
}

func TestSetLastAuthContext_OverwritesPrevious(t *testing.T) {
	t.Cleanup(ClearLastAuthContext)

	first := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "first"},
	}
	second := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "second"},
	}

	SetLastAuthContext(first, "mgr1")
	SetLastAuthContext(second, "mgr2")

	gotCtx, gotMgr := GetLastAuthContext()
	require.NotNil(t, gotCtx)
	assert.Equal(t, "second", gotCtx.AWS.Profile)
	assert.Equal(t, "mgr2", gotMgr)
}
