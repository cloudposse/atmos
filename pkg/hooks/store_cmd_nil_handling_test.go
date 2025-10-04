package hooks

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// TestStoreCommand_NilOutputHandling verifies proper handling of nil/missing terraform outputs.
//
// Tests three distinct scenarios:
// 1. Missing outputs (exists=false) - should error.
// 2. Empty outputs map - valid (component has no outputs).
// 3. Legitimate null values (exists=true, value=nil) - should store nil.
func TestStoreCommand_NilOutputHandling(t *testing.T) {
	tests := []struct {
		name        string
		mockOutput  any
		expectError bool
		description string
	}{
		{
			name:        "missing output returns error",
			mockOutput:  nil,
			expectError: true,
			description: "Missing outputs should error (exists=false)",
		},
		{
			name:        "empty outputs map is valid",
			mockOutput:  map[string]any{},
			expectError: false,
			description: "Empty outputs map is valid (component has no outputs)",
		},
		{
			name:        "valid output works correctly",
			mockOutput:  "vpc-12345",
			expectError: false,
			description: "Normal case: valid terraform output value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock store
			mockStore := NewMockStore()
			atmosConfig := &schema.AtmosConfiguration{
				Stores: store.StoreRegistry{
					"test-store": mockStore,
				},
			}

			// Create store command with mocked terraform output getter
			cmd := &StoreCommand{
				Name:        "store",
				atmosConfig: atmosConfig,
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				outputGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
					// Simulate different scenarios:
					// - tt.mockOutput == nil && tt.expectError: simulate missing output (exists=false)
					// - tt.mockOutput == nil && !tt.expectError: simulate legitimate null (exists=true, value=nil)
					// - Otherwise: normal value (exists=true, value=mockOutput)
					if tt.mockOutput == nil && tt.expectError {
						// Missing output scenario
						return nil, false, nil
					}
					// Valid output (may be nil if legitimate null)
					return tt.mockOutput, true, nil
				},
			}

			// Setup hook that tries to store terraform output
			hook := &Hook{
				Name: "test-store",
				Outputs: map[string]string{
					"vpc_id": ".vpc_id", // Dot prefix = fetch from terraform
				},
			}

			// Execute the store command
			err := cmd.processStoreCommand(hook)

			// Verify error expectation
			if tt.expectError {
				assert.Error(t, err, "Expected error for missing terraform output")

				// Verify nil was NOT stored (it shouldn't be stored if error occurred)
				storedData := mockStore.GetData()
				assert.Empty(t, storedData, "Should not store any data when error occurs")
				return
			}

			// No error expected - verify success
			require.NoError(t, err, tt.description)

			// Verify the value was stored correctly
			storedData := mockStore.GetData()
			if tt.mockOutput == nil {
				return
			}

			// Check if it's an empty map
			if mapVal, ok := tt.mockOutput.(map[string]any); ok && len(mapVal) == 0 {
				// Empty map is valid but stores nothing
				return
			}

			// Verify non-empty value was stored
			expectedKey := "test-stack/test-component/vpc_id"
			assert.Contains(t, storedData, expectedKey)
			assert.Equal(t, tt.mockOutput, storedData[expectedKey])
		})
	}
}

// TestStoreCommand_IntermittentFailureHandling verifies proper error handling
// for intermittent failures (e.g., rate limits) that cause missing outputs.
//
// Simulates 10% failure rate to verify errors are properly returned.
func TestStoreCommand_IntermittentFailureHandling(t *testing.T) {
	const iterations = 100
	var nilReturned atomic.Int32
	var successCount atomic.Int32
	var errorCount atomic.Int32

	// Setup mock store
	mockStore := NewMockStore()
	atmosConfig := &schema.AtmosConfiguration{
		Stores: store.StoreRegistry{
			"test-store": mockStore,
		},
	}

	for i := 0; i < iterations; i++ {
		iteration := i
		t.Run(t.Name(), func(t *testing.T) {
			// Clear store for each iteration
			mockStore.Clear()

			// Simulate 10% failure rate (rate limit returns missing output)
			mockGetter := func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
				// Simulate intermittent rate limit: 10% of calls return missing output
				if iteration%10 == 0 {
					nilReturned.Add(1)
					return nil, false, nil // Simulates rate limit causing missing output
				}
				return "vpc-12345", true, nil // Normal success
			}

			cmd := &StoreCommand{
				Name:         "store",
				atmosConfig:  atmosConfig,
				info:         &schema.ConfigAndStacksInfo{ComponentFromArg: "test-component", Stack: "test-stack"},
				outputGetter: mockGetter,
			}

			hook := &Hook{
				Name:    "test-store",
				Outputs: map[string]string{"vpc_id": ".vpc_id"},
			}

			err := cmd.processStoreCommand(hook)

			if err != nil {
				errorCount.Add(1)
			} else {
				successCount.Add(1)

				// Check what was stored
				storedData := mockStore.GetData()
				key := "test-stack/test-component/vpc_id"

				// BUG: When nil is returned, it might be silently stored
				if storedData[key] == nil {
					t.Logf("Iteration %d: Stored nil value (THIS IS THE BUG)", iteration)
				}
			}
		})
	}

	// Report statistics
	nilCount := nilReturned.Load()
	successTotal := successCount.Load()
	errorTotal := errorCount.Load()

	t.Logf("\n=== Intermittent Failure Statistics ===")
	t.Logf("Total iterations: %d", iterations)
	t.Logf("Nil returned (simulated rate limits): %d (%.1f%%)", nilCount, float64(nilCount)/float64(iterations)*100)
	t.Logf("Successful stores: %d (%.1f%%)", successTotal, float64(successTotal)/float64(iterations)*100)
	t.Logf("Errors: %d (%.1f%%)", errorTotal, float64(errorTotal)/float64(iterations)*100)

	// VERIFICATION:
	// All missing output cases should error (errorTotal should equal nilCount)
	if nilCount > 0 && errorTotal != nilCount {
		t.Errorf("FAILURE: %d missing outputs but only %d errors - %d silent failures!",
			nilCount, errorTotal, nilCount-errorTotal)
	}
}

// TestStoreCommand_RateLimitErrorHandling verifies proper error handling when
// rate limits cause missing outputs.
//
// AWS SDK retry behavior when rate limited:
// 1. Initial call fails with throttle error.
// 2. SDK retries with exponential backoff.
// 3. After max retries, returns missing output (exists=false).
// 4. Code properly errors instead of storing nil.
func TestStoreCommand_RateLimitErrorHandling(t *testing.T) {
	// Track retry attempts
	attemptCount := 0

	mockStore := NewMockStore()
	atmosConfig := &schema.AtmosConfiguration{
		Stores: store.StoreRegistry{
			"test-store": mockStore,
		},
	}

	// Simulate AWS SDK retry behavior
	mockGetter := func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
		attemptCount++

		// Simulate SDK retry pattern:
		// - Attempts 1-2: Would fail with rate limit (SDK retries internally)
		// - Attempt 3: SDK gives up, returns missing output (exists=false)
		if attemptCount >= 3 {
			t.Logf("SDK exhausted retries, returning missing output (simulates partial failure)")
			return nil, false, nil // Missing output after retries
		}

		// Simulate internal SDK retries (not visible to our code)
		t.Logf("Attempt %d: SDK retrying internally...", attemptCount)
		return nil, false, nil
	}

	cmd := &StoreCommand{
		Name:         "store",
		atmosConfig:  atmosConfig,
		info:         &schema.ConfigAndStacksInfo{ComponentFromArg: "test-component", Stack: "test-stack"},
		outputGetter: mockGetter,
	}

	hook := &Hook{
		Name:    "test-store",
		Outputs: map[string]string{"vpc_id": ".vpc_id"},
	}

	err := cmd.processStoreCommand(hook)

	t.Logf("Total attempts: %d", attemptCount)

	// Verify proper error handling
	if err != nil {
		t.Logf("✓ Correctly errored on missing output after rate limit")
	} else {
		t.Errorf("✗ FAILURE: Should have errored on missing output from rate-limited SDK call")

		// Verify what was stored
		storedData := mockStore.GetData()
		key := "test-stack/test-component/vpc_id"
		t.Logf("Stored value: %v (should have errored, not stored)", storedData[key])
	}
}

// TestStoreCommand_ErrorVsLegitimateNull tests the distinction between
// error conditions (SDK errors, missing outputs) and legitimate null values.
func TestStoreCommand_ErrorVsLegitimateNull(t *testing.T) {
	tests := []struct {
		name          string
		mockGetter    TerraformOutputGetter
		expectError   bool
		expectStored  bool
		expectedValue any
	}{
		{
			name: "error propagates correctly",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
				// Simulate explicit SDK error
				return nil, false, errors.New("SDK error: rate limit exceeded")
			},
			expectError:  true,
			expectStored: false,
		},
		{
			name: "missing output returns error",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
				// Output doesn't exist
				return nil, false, nil
			},
			expectError:  true,
			expectStored: false,
		},
		{
			name: "valid value stored correctly",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
				return "vpc-12345", true, nil
			},
			expectError:   false,
			expectStored:  true,
			expectedValue: "vpc-12345",
		},
		{
			name: "legitimate null value stored correctly",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
				// Terraform output exists but has null value
				return nil, true, nil
			},
			expectError:   false,
			expectStored:  true,
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := NewMockStore()
			atmosConfig := &schema.AtmosConfiguration{
				Stores: store.StoreRegistry{
					"test-store": mockStore,
				},
			}

			cmd := &StoreCommand{
				Name:         "store",
				atmosConfig:  atmosConfig,
				info:         &schema.ConfigAndStacksInfo{ComponentFromArg: "test-component", Stack: "test-stack"},
				outputGetter: tt.mockGetter,
			}

			hook := &Hook{
				Name:    "test-store",
				Outputs: map[string]string{"vpc_id": ".vpc_id"},
			}

			err := cmd.processStoreCommand(hook)

			if tt.expectError {
				assert.Error(t, err, "Expected error for nil output")
			} else {
				assert.NoError(t, err)
			}

			storedData := mockStore.GetData()
			key := "test-stack/test-component/vpc_id"

			if tt.expectStored {
				assert.Contains(t, storedData, key)
				assert.Equal(t, tt.expectedValue, storedData[key])
			} else if err != nil {
				// Should not have stored anything if error occurred
				assert.NotContains(t, storedData, key, "Should not store value when error occurs")
			}
		})
	}
}

// TestStoreCommand_MockOutputGetter verifies the mock injection works correctly.
func TestStoreCommand_MockOutputGetter(t *testing.T) {
	var getterCalled bool

	mockStore := NewMockStore()
	atmosConfig := &schema.AtmosConfiguration{
		Stores: store.StoreRegistry{
			"test-store": mockStore,
		},
	}

	mockGetter := func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
		getterCalled = true
		assert.Equal(t, "test-stack", stack)
		assert.Equal(t, "test-component", component)
		assert.Equal(t, "vpc_id", output)
		return "mocked-vpc-id", true, nil
	}

	cmd := &StoreCommand{
		Name:         "store",
		atmosConfig:  atmosConfig,
		info:         &schema.ConfigAndStacksInfo{ComponentFromArg: "test-component", Stack: "test-stack"},
		outputGetter: mockGetter,
	}

	hook := &Hook{
		Name:    "test-store",
		Outputs: map[string]string{"vpc_id": ".vpc_id"},
	}

	err := cmd.processStoreCommand(hook)

	require.NoError(t, err)
	assert.True(t, getterCalled, "Mock getter should have been called")

	storedData := mockStore.GetData()
	assert.Equal(t, "mocked-vpc-id", storedData["test-stack/test-component/vpc_id"])
}

// TestStoreCommand_MissingOutputError verifies missing outputs properly error.
func TestStoreCommand_MissingOutputError(t *testing.T) {
	mockStore := &trackingMockStore{
		MockStore: NewMockStore(),
	}

	atmosConfig := &schema.AtmosConfiguration{
		Stores: store.StoreRegistry{
			"test-store": mockStore,
		},
	}

	cmd := &StoreCommand{
		Name:        "store",
		atmosConfig: atmosConfig,
		info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "test-component", Stack: "test-stack"},
		outputGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, skipCache bool) (any, bool, error) {
			// Return missing output to test that it errors
			return nil, false, nil
		},
	}

	hook := &Hook{
		Name:    "test-store",
		Outputs: map[string]string{"vpc_id": ".vpc_id"},
	}

	err := cmd.processStoreCommand(hook)

	// Check if Set was called with nil
	if mockStore.setCalledWithNil {
		t.Logf("Set() was called with nil value")
		t.Logf("Set() was called %d times with nil", mockStore.nilSetCount)
	}

	// Verify proper error handling for missing outputs
	if err == nil {
		t.Errorf("FAILURE: processStoreCommand should error for missing output but didn't")
	} else {
		t.Logf("✓ Correctly errored for missing output: %v", err)
	}

	if mockStore.setCalledWithNil {
		t.Errorf("FAILURE: Set() was called with nil despite error")
	}
}

// trackingMockStore wraps MockStore to track Set calls with nil values.
type trackingMockStore struct {
	*MockStore
	setCalledWithNil bool
	nilSetCount      int
}

func (t *trackingMockStore) Set(stack string, component string, key string, value any) error {
	if value == nil {
		t.setCalledWithNil = true
		t.nilSetCount++
	}
	return t.MockStore.Set(stack, component, key, value)
}
