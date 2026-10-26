package marketplace

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidationError_ErrorWithWrapped(t *testing.T) {
	inner := fmt.Errorf("inner error")
	ve := &ValidationError{
		Field:   "name",
		Message: "is required",
		Err:     inner,
	}

	result := ve.Error()

	assert.Contains(t, result, "name")
	assert.Contains(t, result, "is required")
	assert.Contains(t, result, "inner error")
}

func TestValidationError_ErrorWithoutWrapped(t *testing.T) {
	ve := &ValidationError{
		Field:   "version",
		Message: "must be semver",
		Err:     nil,
	}

	result := ve.Error()

	assert.Contains(t, result, "version")
	assert.Contains(t, result, "must be semver")
	assert.NotContains(t, result, "(")
}

func TestValidationError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("wrapped error")
	ve := &ValidationError{
		Field:   "field",
		Message: "msg",
		Err:     inner,
	}

	assert.Equal(t, inner, ve.Unwrap())
}

func TestValidationError_UnwrapNil(t *testing.T) {
	ve := &ValidationError{
		Field:   "field",
		Message: "msg",
		Err:     nil,
	}

	assert.Nil(t, ve.Unwrap())
}

func TestValidationError_IsError(t *testing.T) {
	inner := ErrInvalidMetadata
	ve := &ValidationError{
		Field:   "field",
		Message: "msg",
		Err:     inner,
	}

	assert.True(t, errors.Is(ve, ErrInvalidMetadata))
}

func TestSentinelErrors(t *testing.T) {
	// Verify all sentinel errors are distinct and non-nil.
	sentinels := []error{
		ErrSkillAlreadyInstalled,
		ErrInvalidSource,
		ErrDownloadFailed,
		ErrInvalidMetadata,
		ErrIncompatibleVersion,
		ErrMissingPromptFile,
		ErrInvalidToolConfig,
		ErrSkillNotFound,
		ErrRegistryCorrupted,
	}

	for i, err := range sentinels {
		assert.NotNil(t, err)
		assert.NotEmpty(t, err.Error())
		for j, other := range sentinels {
			if i != j {
				assert.NotEqual(t, err.Error(), other.Error(), "sentinel errors %d and %d should differ", i, j)
			}
		}
	}
}
