package provisioner

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resetRegistry clears the provisioner registry for testing.
func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	provisionersByEvent = make(map[HookEvent][]Provisioner)
}

func TestRegisterProvisioner(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	event := HookEvent("before.terraform.init")

	mockFunc := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
		return nil
	}

	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func:      mockFunc,
	}

	// Register the provisioner.
	err := RegisterProvisioner(provisioner)
	require.NoError(t, err)

	// Verify it was registered.
	provisioners := GetProvisionersForEvent(event)
	require.Len(t, provisioners, 1)
	assert.Equal(t, "backend", provisioners[0].Type)
	assert.Equal(t, event, provisioners[0].HookEvent)
}

func TestRegisterProvisioner_NilFuncReturnsError(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: HookEvent("before.terraform.init"),
		Func:      nil,
	}

	// Should return error when Func is nil.
	err := RegisterProvisioner(provisioner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil Func")
}

func TestRegisterProvisioner_EmptyHookEventReturnsError(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: "",
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			return nil
		},
	}

	// Should return error when HookEvent is empty.
	err := RegisterProvisioner(provisioner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty HookEvent")
}

func TestRegisterProvisioner_MultipleForSameEvent(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	event := HookEvent("before.terraform.init")

	provisioner1 := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			return nil
		},
	}

	provisioner2 := Provisioner{
		Type:      "validation",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			return nil
		},
	}

	// Register both provisioners.
	err := RegisterProvisioner(provisioner1)
	require.NoError(t, err)
	err = RegisterProvisioner(provisioner2)
	require.NoError(t, err)

	// Verify both were registered.
	provisioners := GetProvisionersForEvent(event)
	require.Len(t, provisioners, 2)

	types := []string{provisioners[0].Type, provisioners[1].Type}
	assert.Contains(t, types, "backend")
	assert.Contains(t, types, "validation")
}

func TestGetProvisionersForEvent_NonExistentEvent(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	event := HookEvent("non.existent.event")

	provisioners := GetProvisionersForEvent(event)
	assert.Nil(t, provisioners)
}

func TestGetProvisionersForEvent_ReturnsCopy(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	event := HookEvent("before.terraform.init")

	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			return nil
		},
	}

	err := RegisterProvisioner(provisioner)
	require.NoError(t, err)

	// Get provisioners twice.
	provisioners1 := GetProvisionersForEvent(event)
	provisioners2 := GetProvisionersForEvent(event)

	// Verify we got copies (different slices).
	require.Len(t, provisioners1, 1)
	require.Len(t, provisioners2, 1)

	// Modify one slice.
	provisioners1[0].Type = "modified"

	// Verify the other slice is unchanged.
	assert.Equal(t, "backend", provisioners2[0].Type)

	// Verify the registry is unchanged.
	provisioners3 := GetProvisionersForEvent(event)
	assert.Equal(t, "backend", provisioners3[0].Type)
}

func TestExecuteProvisioners_NoProvisioners(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("non.existent.event")
	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	err := ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.NoError(t, err)
}

func TestExecuteProvisioners_SingleProvisionerSuccess(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("before.terraform.init")

	provisionerCalled := false
	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisionerCalled = true
			assert.NotNil(t, atmosConfig)
			assert.NotNil(t, componentConfig)
			return nil
		},
	}

	err := RegisterProvisioner(provisioner)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{
		"backend_type": "s3",
	}

	err = ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.NoError(t, err)
	assert.True(t, provisionerCalled, "Provisioner should have been called")
}

func TestExecuteProvisioners_MultipleProvisionersSuccess(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("before.terraform.init")

	provisioner1Called := false
	provisioner1 := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisioner1Called = true
			return nil
		},
	}

	provisioner2Called := false
	provisioner2 := Provisioner{
		Type:      "validation",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisioner2Called = true
			return nil
		},
	}

	err := RegisterProvisioner(provisioner1)
	require.NoError(t, err)
	err = RegisterProvisioner(provisioner2)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	err = ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.NoError(t, err)
	assert.True(t, provisioner1Called, "Provisioner 1 should have been called")
	assert.True(t, provisioner2Called, "Provisioner 2 should have been called")
}

func TestExecuteProvisioners_FailFast(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("before.terraform.init")

	provisioner1Called := false
	provisioner2Called := false
	expectedErr := errors.New("provisioning failed")
	provisioner1 := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisioner1Called = true
			return expectedErr
		},
	}

	provisioner2 := Provisioner{
		Type:      "validation",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisioner2Called = true
			return nil
		},
	}

	// Register provisioner1 first - execution order is deterministic (slice append order).
	err := RegisterProvisioner(provisioner1)
	require.NoError(t, err)
	err = RegisterProvisioner(provisioner2)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	err = ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionerFailed, "Should return ErrProvisionerFailed sentinel")
	assert.ErrorIs(t, err, expectedErr, "Should wrap the underlying error")
	assert.True(t, provisioner1Called, "Provisioner 1 should have been called")
	assert.False(t, provisioner2Called, "Provisioner 2 should not have been called due to fail-fast")
}

func TestExecuteProvisioners_WithAuthContext(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("before.terraform.init")

	var capturedAuthContext *schema.AuthContext
	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			capturedAuthContext = authContext
			return nil
		},
	}

	err := RegisterProvisioner(provisioner)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
			Region:  "us-west-2",
		},
	}

	err = ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, authContext)
	require.NoError(t, err)
	require.NotNil(t, capturedAuthContext)
	require.NotNil(t, capturedAuthContext.AWS)
	assert.Equal(t, "test-profile", capturedAuthContext.AWS.Profile)
	assert.Equal(t, "us-west-2", capturedAuthContext.AWS.Region)
}

func TestExecuteProvisioners_DifferentEvents(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event1 := HookEvent("before.terraform.init")
	event2 := HookEvent("after.terraform.apply")

	provisioner1Called := false
	provisioner1 := Provisioner{
		Type:      "backend",
		HookEvent: event1,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisioner1Called = true
			return nil
		},
	}

	provisioner2Called := false
	provisioner2 := Provisioner{
		Type:      "cleanup",
		HookEvent: event2,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			provisioner2Called = true
			return nil
		},
	}

	err := RegisterProvisioner(provisioner1)
	require.NoError(t, err)
	err = RegisterProvisioner(provisioner2)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	// Execute event1 provisioners.
	err = ExecuteProvisioners(ctx, event1, atmosConfig, componentConfig, nil)
	require.NoError(t, err)
	assert.True(t, provisioner1Called, "Event1 provisioner should have been called")
	assert.False(t, provisioner2Called, "Event2 provisioner should not have been called")

	// Execute event2 provisioners.
	provisioner1Called = false
	provisioner2Called = false
	err = ExecuteProvisioners(ctx, event2, atmosConfig, componentConfig, nil)
	require.NoError(t, err)
	assert.False(t, provisioner1Called, "Event1 provisioner should not have been called")
	assert.True(t, provisioner2Called, "Event2 provisioner should have been called")
}

func TestConcurrentRegistration(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	event := HookEvent("before.terraform.init")
	var wg sync.WaitGroup

	// Register 100 provisioners concurrently.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provisioner := Provisioner{
				Type:      "backend",
				HookEvent: event,
				Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
					return nil
				},
			}
			_ = RegisterProvisioner(provisioner)
		}()
	}

	wg.Wait()

	// Verify all provisioners were registered.
	provisioners := GetProvisionersForEvent(event)
	assert.Len(t, provisioners, 100, "All provisioners should be registered")
}

func TestExecuteProvisioners_ContextCancellation(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	event := HookEvent("before.terraform.init")

	provisioner := Provisioner{
		Type:      "backend",
		HookEvent: event,
		Func: func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, authContext *schema.AuthContext) error {
			// Check if context is cancelled.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
	}

	err := RegisterProvisioner(provisioner)
	require.NoError(t, err)

	// Create a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	err = ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionerFailed, "Should return ErrProvisionerFailed sentinel")
	assert.ErrorIs(t, err, context.Canceled, "Should wrap the context.Canceled error")
}

func TestHookEventType(t *testing.T) {
	// Test that HookEvent is a string type and can be used as map key.
	event1 := HookEvent("before.terraform.init")
	event2 := HookEvent("before.terraform.init")
	event3 := HookEvent("after.terraform.apply")

	assert.Equal(t, event1, event2)
	assert.NotEqual(t, event1, event3)

	// Test as map key.
	eventMap := make(map[HookEvent]string)
	eventMap[event1] = "init"
	eventMap[event3] = "apply"

	assert.Equal(t, "init", eventMap[event2])
	assert.Equal(t, "apply", eventMap[event3])
}

func TestExecuteProvisioners_NilFuncDefensiveCheck(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("before.terraform.init")

	// Directly inject a provisioner with nil Func into the registry.
	// This bypasses RegisterProvisioner validation to test the defensive check.
	registryMu.Lock()
	provisionersByEvent[event] = []Provisioner{
		{
			Type:      "invalid-provisioner",
			HookEvent: event,
			Func:      nil, // Invalid: nil function.
		},
	}
	registryMu.Unlock()

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	err := ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionerFailed, "Should return ErrProvisionerFailed for nil Func")
}

func TestExecuteProvisioners_NilFuncWithEmptyType(t *testing.T) {
	// Reset registry before test.
	resetRegistry()

	ctx := context.Background()
	event := HookEvent("before.terraform.init")

	// Directly inject a provisioner with nil Func and empty Type into the registry.
	registryMu.Lock()
	provisionersByEvent[event] = []Provisioner{
		{
			Type:      "", // Empty type.
			HookEvent: event,
			Func:      nil,
		},
	}
	registryMu.Unlock()

	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{}

	err := ExecuteProvisioners(ctx, event, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionerFailed, "Should return ErrProvisionerFailed for nil Func with empty type")
}
