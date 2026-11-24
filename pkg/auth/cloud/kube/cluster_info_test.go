package kube

// Tests for realm-scoped exec-plugin users (issue #3720, #3675).

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// TestUserName_Realm verifies that a non-empty realm follows "atmos-" and an
// empty realm keeps the name earlier versions wrote.
func TestUserName_Realm(t *testing.T) {
	base := ClusterInfo{UserPrefix: "eks", Name: "prod", Region: "us-east-2", AccountID: "111111111111", Identity: "admin"}

	withRealm := base
	withRealm.Realm = "acme"
	assert.Equal(t, "atmos-acme-eks-prod-us-east-2-111111111111-admin-2bb64d81", UserName(&withRealm))
	assert.Equal(t, "atmos-acme-eks-prod-us-east-2-111111111111-admin", UnhashedUserName(&withRealm))
	assert.Equal(t, "atmos-eks-prod-us-east-2-111111111111-admin-48b9c2fe", UserName(&base))
}

// TestUserName_NoRealmOrIdentityIsUnhashed verifies that names without a realm
// or identity stay exactly as v1.234.1 wrote them.
func TestUserName_NoRealmOrIdentityIsUnhashed(t *testing.T) {
	info := ClusterInfo{UserPrefix: "eks", Name: "prod", Region: "us-east-2", AccountID: "111111111111"}
	assert.Equal(t, "atmos-eks-prod-us-east-2-111111111111", UserName(&info))
	assert.Equal(t, []string{"atmos-eks-prod-us-east-2-111111111111"}, UserNameForms(&info))
}

// TestUserName_HyphenatedComponentsDoNotCollide verifies that tuples whose
// components only differ in where the "-" separators fall get distinct names.
func TestUserName_HyphenatedComponentsDoNotCollide(t *testing.T) {
	first := ClusterInfo{Realm: "r", UserPrefix: "eks", Name: "b-eks-c", Region: "us-east-2", AccountID: "111111111111", Identity: "admin"}
	second := ClusterInfo{Realm: "r-eks-b", UserPrefix: "eks", Name: "c", Region: "us-east-2", AccountID: "111111111111", Identity: "admin"}

	assert.Equal(t, UnhashedUserName(&first), UnhashedUserName(&second), "the readable forms collide")
	assert.NotEqual(t, UserName(&first), UserName(&second))
	assert.Len(t, UserNameForms(&first), 2)
	assert.Equal(t, UnhashedUserName(&first), UserNameForms(&first)[1])
}

// TestUserName_ComponentBoundariesAreHashed verifies that moving a character
// between adjacent components changes the hash.
func TestUserName_ComponentBoundariesAreHashed(t *testing.T) {
	first := ClusterInfo{UserPrefix: "eks", Name: "ab", Region: "c", Identity: "id"}
	second := ClusterInfo{UserPrefix: "eks", Name: "a", Region: "bc", Identity: "id"}
	assert.NotEqual(t, UserName(&first), UserName(&second))
}

// TestBuildClusterConfig_RealmEnv verifies that a realm-scoped entry passes the
// realm to the exec plugin exactly once and leaves the caller's ExecEnv intact.
func TestBuildClusterConfig_RealmEnv(t *testing.T) {
	identityEnv := clientcmdapi.ExecEnvVar{Name: "ATMOS_IDENTITY", Value: "admin"}
	realmEnv := clientcmdapi.ExecEnvVar{Name: "ATMOS_AUTH_REALM", Value: "acme"}

	tests := []struct {
		name    string
		realm   string
		execEnv []clientcmdapi.ExecEnvVar
		want    []clientcmdapi.ExecEnvVar
	}{
		{name: "no realm", execEnv: []clientcmdapi.ExecEnvVar{identityEnv}, want: []clientcmdapi.ExecEnvVar{identityEnv}},
		{name: "realm is appended", realm: "acme", execEnv: []clientcmdapi.ExecEnvVar{identityEnv}, want: []clientcmdapi.ExecEnvVar{identityEnv, realmEnv}},
		{name: "realm already set is kept", realm: "acme", execEnv: []clientcmdapi.ExecEnvVar{realmEnv, identityEnv}, want: []clientcmdapi.ExecEnvVar{realmEnv, identityEnv}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := testClusterInfo()
			info.Realm = tt.realm
			info.ExecEnv = append([]clientcmdapi.ExecEnvVar(nil), tt.execEnv...)

			config := BuildClusterConfig(info, "prod")
			user := config.AuthInfos[UserName(info)]
			require.NotNil(t, user)
			require.NotNil(t, user.Exec)
			assert.Equal(t, tt.want, user.Exec.Env)
			assert.Equal(t, tt.execEnv, info.ExecEnv, "the caller's ExecEnv must not be modified")
		})
	}
}
