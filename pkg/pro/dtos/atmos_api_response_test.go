package dtos

import (
	"encoding/json"
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

	t.Run("JSON round-trip test", func(t *testing.T) {
		// Test that JSON serialization/deserialization works correctly with camelCase traceId
		original := AtmosApiResponse{
			Request:      "test-operation",
			Status:       200,
			Success:      true,
			ErrorMessage: "Test error",
			Context:      map[string]interface{}{"key": "value"},
			TraceID:      "trace-123",
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		assert.NoError(t, err)

		// Verify the JSON contains camelCase traceId
		jsonStr := string(jsonData)
		assert.Contains(t, jsonStr, `"traceId":"trace-123"`)
		assert.NotContains(t, jsonStr, `"trace_id"`)

		// Unmarshal back to struct
		var unmarshaled AtmosApiResponse
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)

		// Verify all fields match
		assert.Equal(t, original.Request, unmarshaled.Request)
		assert.Equal(t, original.Status, unmarshaled.Status)
		assert.Equal(t, original.Success, unmarshaled.Success)
		assert.Equal(t, original.ErrorMessage, unmarshaled.ErrorMessage)
		assert.Equal(t, original.Context, unmarshaled.Context)
		assert.Equal(t, original.TraceID, unmarshaled.TraceID)
	})
}
