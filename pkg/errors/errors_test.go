package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtmosError(t *testing.T) {
	// Test Error() method
	err := &AtmosError{
		Message: "test error message",
		Base:    fmt.Errorf("base error"),
		Meta: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
		Tips: []string{"tip1", "tip2"},
	}

	// Test nil error case
	nilErr := &AtmosError{
		Message: "nil base error",
		Base:    nil,
	}

	// Test nil error's Unwrap method
	assert.Nil(t, nilErr.Unwrap())

	// Test Error() method
	assert.Equal(t, "test error message", err.Error())

	// Test Unwrap() method
	baseErr := err.Unwrap()
	assert.Equal(t, "base error", baseErr.Error())

	// Test Fields() method
	fields := err.Fields()
	assert.Equal(t, 2, len(fields))
	assert.Equal(t, "value1", fields["key1"])
	assert.Equal(t, 42, fields["key2"])

	// Test WithContext() method - even number of args
	err2 := err.WithContext("key3", "value3", "key4", 84)
	assert.Equal(t, 4, len(err2.Meta))
	assert.Equal(t, "value3", err2.Meta["key3"])
	assert.Equal(t, 84, err2.Meta["key4"])

	// Test WithContext() method - odd number of args
	err3 := err.WithContext("key3", "value3", "key4")
	assert.Equal(t, 4, len(err3.Meta))
	assert.Equal(t, "value3", err3.Meta["key3"])
	assert.Equal(t, "", err3.Meta["key4"])

	// Test WithContext() method with non-string key
	err5 := err.WithContext(123, "value5")
	assert.Equal(t, 5, len(err5.Meta)) // Original 2 keys + key3 + key4 + 123
	assert.Equal(t, "value5", err5.Meta["123"])

	// Test WithTips() method
	err4 := err.WithTips("tip3", "tip4")
	assert.Equal(t, 4, len(err4.Tips))
	assert.Equal(t, "tip3", err4.Tips[2])
	assert.Equal(t, "tip4", err4.Tips[3])
}

func TestNewBaseError(t *testing.T) {
	// Test creating a base error
	baseErr := NewBaseError("test base error")

	// Verify properties
	assert.Equal(t, "test base error", baseErr.Error())
	assert.Nil(t, baseErr.Unwrap())
	assert.NotNil(t, baseErr.Meta)
	assert.Empty(t, baseErr.Meta)

	// Test that it can be used with WithContext and WithTips
	err := baseErr.WithContext("key", "value").WithTips("helpful tip")

	assert.Equal(t, "test base error", err.Error())
	assert.Equal(t, "value", err.Meta["key"])
	assert.Equal(t, "helpful tip", err.Tips[0])
}

func TestNewAtmosError(t *testing.T) {
	// Test with nil base error
	err1 := NewAtmosError("error message", nil)
	assert.Equal(t, "error message", err1.Error())
	assert.Nil(t, err1.Unwrap())
	assert.NotNil(t, err1.Meta)

	// Test with base error
	baseErr := fmt.Errorf("base error")
	err2 := NewAtmosError("wrapped error", baseErr)
	assert.Equal(t, "wrapped error", err2.Error())
	assert.Equal(t, baseErr, err2.Unwrap())

	// Test with context and tips
	err3 := NewAtmosError("error with context", nil).
		WithContext("key", "value").
		WithTips("tip1", "tip2")

	assert.Equal(t, "value", err3.Meta["key"])
	assert.Equal(t, 2, len(err3.Tips))
}

func TestAtmosError_WithContextNilMap(t *testing.T) {
	// Create an AtmosError with nil Meta
	err := &AtmosError{
		Message: "test error",
		Meta:    nil,
	}

	// Should create a new map if Meta is nil
	err2 := err.WithContext("key", "value")
	assert.NotNil(t, err2.Meta)
	assert.Equal(t, "value", err2.Meta["key"])
}
