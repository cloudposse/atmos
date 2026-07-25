package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: a rename of any RDSDatabase field breaks the build here immediately.
var _ = schema.RDSDatabase{Host: "h", Port: 1, Username: "u", Region: "r", Database: "d", Engine: "postgres"}

func validRDSConfig() *integrations.IntegrationConfig {
	return &integrations.IntegrationConfig{
		Name: "prod-db",
		Config: &schema.Integration{
			Kind: integrations.KindAWSRDS,
			Via:  &schema.IntegrationVia{Identity: "dev-admin"},
			Spec: &schema.IntegrationSpec{
				Database: &schema.RDSDatabase{
					Host:     "mydb.abc123.us-east-2.rds.amazonaws.com",
					Port:     5432,
					Username: "app",
					Region:   "us-east-2",
					Engine:   "postgres",
				},
			},
		},
	}
}

func TestNewRDSIntegration_Valid(t *testing.T) {
	integ, err := NewRDSIntegration(validRDSConfig())
	require.NoError(t, err)
	require.NotNil(t, integ)
	assert.Equal(t, integrations.KindAWSRDS, integ.Kind())

	rds, ok := integ.(*RDSIntegration)
	require.True(t, ok)
	assert.Equal(t, "dev-admin", rds.GetIdentity())
}

func TestNewRDSIntegration_Errors(t *testing.T) {
	rdsSpec := func(db *schema.RDSDatabase) *integrations.IntegrationConfig {
		return &integrations.IntegrationConfig{
			Name:   "x",
			Config: &schema.Integration{Kind: integrations.KindAWSRDS, Spec: &schema.IntegrationSpec{Database: db}},
		}
	}
	tests := []struct {
		name string
		cfg  *integrations.IntegrationConfig
	}{
		{"nil config", nil},
		{"nil inner config", &integrations.IntegrationConfig{Name: "x"}},
		{"missing spec.database", &integrations.IntegrationConfig{
			Name:   "x",
			Config: &schema.Integration{Kind: integrations.KindAWSRDS, Spec: &schema.IntegrationSpec{}},
		}},
		{"missing host", rdsSpec(&schema.RDSDatabase{Port: 5432, Username: "app"})},
		{"missing port", rdsSpec(&schema.RDSDatabase{Host: "h", Username: "app"})},
		{"missing username", rdsSpec(&schema.RDSDatabase{Host: "h", Port: 5432})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRDSIntegration(tt.cfg)
			require.Error(t, err)
		})
	}
}

func TestRDSIntegration_NoOpBehavior(t *testing.T) {
	integ, err := NewRDSIntegration(validRDSConfig())
	require.NoError(t, err)

	// Execute is a no-op: no login-time side effects, no error.
	require.NoError(t, integ.Execute(context.Background(), nil))
	// Cleanup is a no-op.
	require.NoError(t, integ.Cleanup(context.Background()))

	// Environment contributes nothing and MUST NEVER expose a token/password.
	env, err := integ.Environment()
	require.NoError(t, err)
	assert.Nil(t, env, "Environment must contribute nothing today")

	// Regression guard: check membership of the forbidden keys DIRECTLY IN env, rather than
	// ranging over env's own keys. Ranging over env is a no-op when env is nil/empty (as it is
	// today) and would silently pass even if Environment() were later changed to leak a token —
	// the previous version of this test did exactly that and never actually executed its body.
	// Indexing a nil map is safe in Go (returns the zero value, ok=false), so this works whether
	// Environment() returns nil or a populated map.
	for _, forbiddenKey := range []string{"PGPASSWORD", "MYSQL_PWD"} {
		_, leaked := env[forbiddenKey]
		assert.False(t, leaked, "Environment() must never expose %s", forbiddenKey)
	}
}

func TestRDSIntegration_Registered(t *testing.T) {
	// init() registered the kind, so the registry can create it by dispatching on config.Config.Kind.
	assert.True(t, integrations.IsRegistered(integrations.KindAWSRDS))

	integ, err := integrations.Create(validRDSConfig())
	require.NoError(t, err)
	assert.Equal(t, integrations.KindAWSRDS, integ.Kind())

	// An unknown kind is rejected.
	_, err = integrations.Create(&integrations.IntegrationConfig{
		Name:   "x",
		Config: &schema.Integration{Kind: "aws/nope"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnknownIntegrationKind)
}
