package dtos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtmosApiResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		response := AtmosApiResponse{
			Request:      "test-operation",
			Status:       200,
			Success:      true,
			ErrorMessage: "",
			Context:      map[string]interface{}{"key": "value"},
			TraceID:      "trace-123",
		}

		assert.Equal(t, "test-operation", response.Request)
		assert.Equal(t, 200, response.Status)
		assert.True(t, response.Success)
		assert.Equal(t, "", response.ErrorMessage)
		assert.Equal(t, map[string]interface{}{"key": "value"}, response.Context)
		assert.Equal(t, "trace-123", response.TraceID)
	})

	t.Run("error response", func(t *testing.T) {
		response := AtmosApiResponse{
			Request:      "test-operation",
			Status:       400,
			Success:      false,
			ErrorMessage: "Bad request",
			Context:      map[string]interface{}{"error": "validation failed"},
			TraceID:      "trace-456",
		}

		assert.Equal(t, "test-operation", response.Request)
		assert.Equal(t, 400, response.Status)
		assert.False(t, response.Success)
		assert.Equal(t, "Bad request", response.ErrorMessage)
		assert.Equal(t, map[string]interface{}{"error": "validation failed"}, response.Context)
		assert.Equal(t, "trace-456", response.TraceID)
	})

	t.Run("minimal response", func(t *testing.T) {
		response := AtmosApiResponse{
			Request: "test-operation",
			Status:  200,
			Success: true,
		}

		assert.Equal(t, "test-operation", response.Request)
		assert.Equal(t, 200, response.Status)
		assert.True(t, response.Success)
		assert.Equal(t, "", response.ErrorMessage)
		assert.Nil(t, response.Context)
		assert.Equal(t, "", response.TraceID)
	})
}
