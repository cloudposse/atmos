package testdata

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

// TestWithAssertionFailure demonstrates a test that fails with testify assertions
// This simulates the TestExecuteVendorPull failure pattern
func TestWithAssertionFailure(t *testing.T) {
	// This is similar to what happens in vendor_utils_test.go:124
	err := simulateVendorPull()
	
	// This assertion will fail, just like in the actual test
	assert.NoError(t, err, "Expected no error from vendor pull")
}

func simulateVendorPull() error {
	// Simulate the vendor pull failure
	return &vendorError{msg: "failed to vendor components: 1"}
}

type vendorError struct {
	msg string
}

func (e *vendorError) Error() string {
	return e.msg
}