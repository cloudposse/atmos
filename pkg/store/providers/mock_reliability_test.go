package providers

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// TestMockReliability_GomockMock tests the reliability of the gomock framework.
// This test attempts to reproduce intermittent failures where mock expectations are not honored.
func TestMockReliability_GomockMock(t *testing.T) {
	const iterations = 100
	var failures atomic.Int32
	var successes atomic.Int32

	for i := 0; i < iterations; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Create a fresh mock for each iteration.
			ctrl := gomock.NewController(t)
			mockClient := NewMockRedisClient(ctrl)

			// Set up expectations.
			testKey := "test-key"
			expectedValue := "test-value"

			mockClient.EXPECT().Get(gomock.Any(), testKey).Return(stringCmd(expectedValue, nil))

			// Execute the operation.
			cmd := mockClient.Get(context.Background(), testKey)
			actualValue, err := cmd.Result()
			// Verify the result.
			if err != nil {
				t.Logf("Iteration %d: Unexpected error: %v", i, err)
				failures.Add(1)
				return
			}

			if actualValue != expectedValue {
				t.Logf("Iteration %d: Expected %q but got %q", i, expectedValue, actualValue)
				failures.Add(1)
				return
			}

			successes.Add(1)
		})
	}

	// Report statistics
	failureCount := failures.Load()
	successCount := successes.Load()

	t.Logf("\n=== Gomock Reliability Statistics ===")
	t.Logf("Total iterations: %d", iterations)
	t.Logf("Successes: %d (%.1f%%)", successCount, float64(successCount)/float64(iterations)*100)
	t.Logf("Failures: %d (%.1f%%)", failureCount, float64(failureCount)/float64(iterations)*100)

	if failureCount > 0 {
		t.Errorf("Detected %d failures out of %d iterations (%.1f%% failure rate)",
			failureCount, iterations, float64(failureCount)/float64(iterations)*100)
	}
}

// TestMockReliability_GomockMock_Parallel tests gomock with parallel execution.
func TestMockReliability_GomockMock_Parallel(t *testing.T) {
	t.Parallel()

	const iterations = 100
	var failures atomic.Int32
	var successes atomic.Int32

	for i := 0; i < iterations; i++ {
		i := i // Capture loop variable
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			t.Parallel()

			// Create a fresh mock for each iteration.
			ctrl := gomock.NewController(t)
			mockClient := NewMockRedisClient(ctrl)

			// Set up expectations.
			testKey := fmt.Sprintf("test-key-%d", i)
			expectedValue := fmt.Sprintf("test-value-%d", i)

			mockClient.EXPECT().Get(gomock.Any(), testKey).Return(stringCmd(expectedValue, nil))

			// Execute the operation.
			cmd := mockClient.Get(context.Background(), testKey)
			actualValue, err := cmd.Result()
			// Verify the result.
			if err != nil {
				t.Logf("Iteration %d: Unexpected error: %v", i, err)
				failures.Add(1)
				return
			}

			if actualValue != expectedValue {
				t.Logf("Iteration %d: Expected %q but got %q", i, expectedValue, actualValue)
				failures.Add(1)
				return
			}

			successes.Add(1)
		})
	}

	// Wait for all parallel tests to complete
	// (t.Run with t.Parallel() will block until all complete)

	// Report statistics
	failureCount := failures.Load()
	successCount := successes.Load()

	t.Logf("\n=== Gomock Parallel Reliability Statistics ===")
	t.Logf("Total iterations: %d", iterations)
	t.Logf("Successes: %d (%.1f%%)", successCount, float64(successCount)/float64(iterations)*100)
	t.Logf("Failures: %d (%.1f%%)", failureCount, float64(failureCount)/float64(iterations)*100)

	if failureCount > 0 {
		t.Errorf("Detected %d failures out of %d parallel iterations (%.1f%% failure rate)",
			failureCount, iterations, float64(failureCount)/float64(iterations)*100)
	}
}

// TestMockReliability_GomockMock_MultipleExpectations tests multiple mock expectations.
func TestMockReliability_GomockMock_MultipleExpectations(t *testing.T) {
	const iterations = 100
	var failures atomic.Int32
	var successes atomic.Int32

	for i := 0; i < iterations; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Create a fresh mock for each iteration.
			ctrl := gomock.NewController(t)
			mockClient := NewMockRedisClient(ctrl)

			// Set up multiple expectations.
			mockClient.EXPECT().Get(gomock.Any(), "key1").Return(stringCmd("value1", nil))
			mockClient.EXPECT().Get(gomock.Any(), "key2").Return(stringCmd("value2", nil))
			mockClient.EXPECT().Get(gomock.Any(), "key3").Return(stringCmd("value3", nil))
			mockClient.EXPECT().Set(gomock.Any(), "key4", "value4", time.Duration(0)).Return(statusCmd("OK", nil))

			// Execute operations
			cmd1 := mockClient.Get(context.Background(), "key1")
			val1, err1 := cmd1.Result()

			cmd2 := mockClient.Get(context.Background(), "key2")
			val2, err2 := cmd2.Result()

			cmd3 := mockClient.Get(context.Background(), "key3")
			val3, err3 := cmd3.Result()

			cmd4 := mockClient.Set(context.Background(), "key4", "value4", time.Duration(0))
			_, err4 := cmd4.Result()

			// Verify results
			success := true
			if err1 != nil || val1 != "value1" {
				t.Logf("Iteration %d: key1 failed: err=%v, val=%q", i, err1, val1)
				success = false
			}
			if err2 != nil || val2 != "value2" {
				t.Logf("Iteration %d: key2 failed: err=%v, val=%q", i, err2, val2)
				success = false
			}
			if err3 != nil || val3 != "value3" {
				t.Logf("Iteration %d: key3 failed: err=%v, val=%q", i, err3, val3)
				success = false
			}
			if err4 != nil {
				t.Logf("Iteration %d: key4 Set failed: err=%v", i, err4)
				success = false
			}

			if success {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		})
	}

	// Report statistics
	failureCount := failures.Load()
	successCount := successes.Load()

	t.Logf("\n=== Gomock Multiple Expectations Reliability Statistics ===")
	t.Logf("Total iterations: %d", iterations)
	t.Logf("Successes: %d (%.1f%%)", successCount, float64(successCount)/float64(iterations)*100)
	t.Logf("Failures: %d (%.1f%%)", failureCount, float64(failureCount)/float64(iterations)*100)

	if failureCount > 0 {
		t.Errorf("Detected %d failures out of %d iterations with multiple expectations (%.1f%% failure rate)",
			failureCount, iterations, float64(failureCount)/float64(iterations)*100)
	}
}

// TestMockReliability_VerifyCalledValues tests that mock actually returns expected values.
func TestMockReliability_VerifyCalledValues(t *testing.T) {
	const iterations = 100
	successCount := 0

	for i := 0; i < iterations; i++ {
		ctrl := gomock.NewController(t)
		mockClient := NewMockRedisClient(ctrl)

		// Set expectation.
		expectedValue := fmt.Sprintf("value-%d", i)
		mockClient.EXPECT().Get(gomock.Any(), "test-key").Return(stringCmd(expectedValue, nil))

		// Call the mock.
		cmd := mockClient.Get(context.Background(), "test-key")
		actualValue, err := cmd.Result()
		// Strict verification.
		if err != nil {
			t.Fatalf("Iteration %d: unexpected error: %v", i, err)
		}
		if actualValue != expectedValue {
			t.Fatalf("Iteration %d: expected %q but got %q", i, expectedValue, actualValue)
		}

		successCount++
	}

	t.Logf("All %d iterations passed with strict verification", successCount)
}
