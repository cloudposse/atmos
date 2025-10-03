package hooks

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// TestStoreCommand_NilOutputBug reproduces the bug where nil terraform outputs
// (from rate limits or partial failures) are silently stored instead of erroring.
//
// This test demonstrates the root cause of intermittent mock output failures:
// 1. AWS rate limit triggers
// 2. SDK retry returns partial/empty response
// 3. Terraform output returns nil
// 4. Code silently stores nil instead of using mock/default value.
func TestStoreCommand_NilOutputBug(t *testing.T) {
	tests := []struct {
		name        string
		mockOutput  any
		expectError bool
		description string
	}{
		{
			name:        "nil output simulates rate limit response",
			mockOutput:  nil,
			expectError: true, // Should error, not silently store nil
			description: "When AWS rate limits, SDK may return nil - this should error",
		},
		{
			name:        "empty map simulates partial failure",
			mockOutput:  map[string]any{},
			expectError: false, // Empty map is valid (no outputs)
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
				outputGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
					// Simulate terraform output returning nil (rate limit case)
					return tt.mockOutput
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
				// BUG: Currently this will NOT error when mockOutput is nil
				// It will silently store nil value instead of erroring
				assert.Error(t, err, "Expected error when terraform output returns nil, but got none - THIS IS THE BUG")

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

// TestStoreCommand_IntermittentNilFailure reproduces the reported issue:
// "if I run the same test 10 times in a row, it'll fail once or twice"
//
// This simulates intermittent rate limit failures that return nil.
func TestStoreCommand_IntermittentNilFailure(t *testing.T) {
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

			// Simulate 10% failure rate (rate limit returns nil)
			mockGetter := func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
				// Simulate intermittent rate limit: 10% of calls return nil
				if iteration%10 == 0 {
					nilReturned.Add(1)
					return nil // Simulates rate limit returning empty response
				}
				return "vpc-12345" // Normal success
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

	// BUG VERIFICATION:
	// Expected: errorTotal should equal nilCount (all nil returns should error)
	// Actual: successTotal will include nil returns that were silently stored
	if nilCount > 0 && errorTotal < nilCount {
		t.Errorf("BUG DETECTED: %d nil returns but only %d errors - %d silent failures!",
			nilCount, errorTotal, nilCount-errorTotal)
	}
}

// TestStoreCommand_RateLimitSimulation simulates AWS SDK behavior during rate limiting.
//
// AWS SDK retry behavior when rate limited:
// 1. Initial call fails with throttle error
// 2. SDK retries with exponential backoff
// 3. After max retries, may return empty/nil response instead of error
// 4. Code treats this as success and stores nil.
func TestStoreCommand_RateLimitSimulation(t *testing.T) {
	// Track retry attempts
	attemptCount := 0

	mockStore := NewMockStore()
	atmosConfig := &schema.AtmosConfiguration{
		Stores: store.StoreRegistry{
			"test-store": mockStore,
		},
	}

	// Simulate AWS SDK retry behavior
	mockGetter := func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
		attemptCount++

		// Simulate SDK retry pattern:
		// - Attempts 1-2: Would fail with rate limit (SDK retries internally)
		// - Attempt 3: SDK gives up, returns nil instead of error
		if attemptCount >= 3 {
			t.Logf("SDK exhausted retries, returning nil (simulates partial failure)")
			return nil // BUG: This nil will be silently stored
		}

		// Simulate internal SDK retries (not visible to our code)
		t.Logf("Attempt %d: SDK retrying internally...", attemptCount)
		return nil
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

	// BUG: This should error when nil is returned after retries
	// Instead it silently stores nil
	if err != nil {
		t.Logf("✓ Correctly errored on nil response")
	} else {
		t.Errorf("✗ BUG: Silently accepted nil response from rate-limited SDK call")

		// Verify what was stored
		storedData := mockStore.GetData()
		key := "test-stack/test-component/vpc_id"
		t.Logf("Stored value: %v (should have errored, not stored nil)", storedData[key])
	}
}

// TestStoreCommand_NilVsError tests the difference between nil value and error.
// This clarifies the expected behavior.
func TestStoreCommand_NilVsError(t *testing.T) {
	tests := []struct {
		name          string
		mockGetter    TerraformOutputGetter
		expectError   bool
		expectStored  bool
		expectedValue any
	}{
		{
			name: "error propagates correctly",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
				// Simulate explicit error (this currently calls CheckErrorPrintAndExit internally)
				// We can't easily test this without mocking that function
				return nil // Simulates error path
			},
			expectError:  true,
			expectStored: false,
		},
		{
			name: "nil value stored silently (BUG)",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
				return nil // Returns nil without error
			},
			expectError:  true, // SHOULD error
			expectStored: false,
		},
		{
			name: "valid value stored correctly",
			mockGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
				return "vpc-12345"
			},
			expectError:   false,
			expectStored:  true,
			expectedValue: "vpc-12345",
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

	mockGetter := func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
		getterCalled = true
		assert.Equal(t, "test-stack", stack)
		assert.Equal(t, "test-component", component)
		assert.Equal(t, "vpc_id", output)
		return "mocked-vpc-id"
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

// TestStoreCommand_NilPropagation verifies nil values propagate through the system.
func TestStoreCommand_NilPropagation(t *testing.T) {
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
		outputGetter: func(cfg *schema.AtmosConfiguration, stack, component, output string, failOnError bool) any {
			return nil // Return nil to test propagation
		},
	}

	hook := &Hook{
		Name:    "test-store",
		Outputs: map[string]string{"vpc_id": ".vpc_id"},
	}

	err := cmd.processStoreCommand(hook)

	// Check if Set was called with nil
	if mockStore.setCalledWithNil {
		t.Logf("BUG CONFIRMED: Set() was called with nil value")
		t.Logf("Set() was called %d times with nil", mockStore.nilSetCount)
	}

	// Current behavior: no error, nil stored (BUG)
	// Expected behavior: error returned, nothing stored
	if err == nil && mockStore.setCalledWithNil {
		t.Errorf("BUG: processStoreCommand accepted nil value and called Set() with it")
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
