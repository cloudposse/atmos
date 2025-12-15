package integrations

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockIntegration is a test implementation of the Integration interface.
type mockIntegration struct {
	kind      string
	executeErr error
}

func (m *mockIntegration) Kind() string {
	return m.kind
}

func (m *mockIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	return m.executeErr
}

func TestRegister(t *testing.T) {
	// Clear the registry for this test.
	registryMu.Lock()
	originalRegistry := make(map[string]IntegrationFactory)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	registry = make(map[string]IntegrationFactory)
	registryMu.Unlock()

	// Restore the original registry after the test.
	t.Cleanup(func() {
		registryMu.Lock()
		registry = originalRegistry
		registryMu.Unlock()
	})

	// Register a test factory.
	testFactory := func(config *IntegrationConfig) (Integration, error) {
		return &mockIntegration{kind: "test/kind"}, nil
	}

	Register("test/kind", testFactory)

	registryMu.RLock()
	_, exists := registry["test/kind"]
	registryMu.RUnlock()

	assert.True(t, exists, "factory should be registered")
}

func TestCreate_Success(t *testing.T) {
	// Clear the registry for this test.
	registryMu.Lock()
	originalRegistry := make(map[string]IntegrationFactory)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	registry = make(map[string]IntegrationFactory)
	registryMu.Unlock()

	// Restore the original registry after the test.
	t.Cleanup(func() {
		registryMu.Lock()
		registry = originalRegistry
		registryMu.Unlock()
	})

	// Register a test factory.
	Register("test/kind", func(config *IntegrationConfig) (Integration, error) {
		return &mockIntegration{kind: "test/kind"}, nil
	})

	config := &IntegrationConfig{
		Name: "test-integration",
		Config: &schema.Integration{
			Kind: "test/kind",
		},
	}

	integration, err := Create(config)
	require.NoError(t, err)
	assert.NotNil(t, integration)
	assert.Equal(t, "test/kind", integration.Kind())
}

func TestCreate_UnknownKind(t *testing.T) {
	// Clear the registry for this test.
	registryMu.Lock()
	originalRegistry := make(map[string]IntegrationFactory)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	registry = make(map[string]IntegrationFactory)
	registryMu.Unlock()

	// Restore the original registry after the test.
	t.Cleanup(func() {
		registryMu.Lock()
		registry = originalRegistry
		registryMu.Unlock()
	})

	config := &IntegrationConfig{
		Name: "test-integration",
		Config: &schema.Integration{
			Kind: "unknown/kind",
		},
	}

	_, err := Create(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown/kind")
}

func TestCreate_FactoryError(t *testing.T) {
	// Clear the registry for this test.
	registryMu.Lock()
	originalRegistry := make(map[string]IntegrationFactory)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	registry = make(map[string]IntegrationFactory)
	registryMu.Unlock()

	// Restore the original registry after the test.
	t.Cleanup(func() {
		registryMu.Lock()
		registry = originalRegistry
		registryMu.Unlock()
	})

	// Register a factory that returns an error.
	Register("error/kind", func(config *IntegrationConfig) (Integration, error) {
		return nil, errors.New("factory error")
	})

	config := &IntegrationConfig{
		Name: "test-integration",
		Config: &schema.Integration{
			Kind: "error/kind",
		},
	}

	_, err := Create(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "factory error")
}

func TestIntegrationConfig_Fields(t *testing.T) {
	config := &IntegrationConfig{
		Name: "my-integration",
		Config: &schema.Integration{
			Kind:     "aws/ecr",
			Identity: "dev-admin",
			Spec: map[string]interface{}{
				"registries": []interface{}{
					map[string]interface{}{
						"account_id": "123456789012",
						"region":     "us-east-1",
					},
				},
			},
		},
	}

	assert.Equal(t, "my-integration", config.Name)
	assert.Equal(t, "aws/ecr", config.Config.Kind)
	assert.Equal(t, "dev-admin", config.Config.Identity)
	assert.NotNil(t, config.Config.Spec)
}
