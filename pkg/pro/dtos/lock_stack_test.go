package dtos

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLockStackRequest(t *testing.T) {
	t.Run("valid lock request with all fields", func(t *testing.T) {
		properties := map[string]interface{}{
			"user":    "test-user",
			"reason":  "deployment",
			"timeout": 3600,
		}

		req := LockStackRequest{
			Key:         "test-stack/test-component",
			TTL:         3600,
			LockMessage: "Deploying to production",
			Properties:  properties,
		}

		assert.Equal(t, "test-stack/test-component", req.Key)
		assert.Equal(t, int32(3600), req.TTL)
		assert.Equal(t, "Deploying to production", req.LockMessage)
		assert.Equal(t, properties, req.Properties)
	})

	t.Run("valid lock request with minimal fields", func(t *testing.T) {
		req := LockStackRequest{
			Key: "test-stack/test-component",
			TTL: 300,
		}

		assert.Equal(t, "test-stack/test-component", req.Key)
		assert.Equal(t, int32(300), req.TTL)
		assert.Equal(t, "", req.LockMessage)
		assert.Nil(t, req.Properties)
	})
}

func TestLockStackResponse(t *testing.T) {
	t.Run("successful lock response", func(t *testing.T) {
		now := time.Now()
		expiresAt := now.Add(time.Hour)

		response := LockStackResponse{
			AtmosApiResponse: AtmosApiResponse{
				Request: "lock-stack",
				Status:  200,
				Success: true,
			},
			Data: struct {
				ID          string    `json:"id,omitempty"`
				WorkspaceId string    `json:"workspaceId,omitempty"`
				Key         string    `json:"key,omitempty"`
				LockMessage string    `json:"lockMessage,omitempty"`
				ExpiresAt   time.Time `json:"expiresAt,omitempty"`
				CreatedAt   time.Time `json:"createdAt,omitempty"`
				UpdatedAt   time.Time `json:"updatedAt,omitempty"`
				DeletedAt   time.Time `json:"deletedAt,omitempty"`
			}{
				ID:          "lock-123",
				WorkspaceId: "workspace-456",
				Key:         "test-stack/test-component",
				LockMessage: "Deploying to production",
				ExpiresAt:   expiresAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		}

		assert.Equal(t, "lock-stack", response.Request)
		assert.Equal(t, 200, response.Status)
		assert.True(t, response.Success)
		assert.Equal(t, "lock-123", response.Data.ID)
		assert.Equal(t, "workspace-456", response.Data.WorkspaceId)
		assert.Equal(t, "test-stack/test-component", response.Data.Key)
		assert.Equal(t, "Deploying to production", response.Data.LockMessage)
		assert.Equal(t, expiresAt, response.Data.ExpiresAt)
		assert.Equal(t, now, response.Data.CreatedAt)
		assert.Equal(t, now, response.Data.UpdatedAt)
		assert.Equal(t, time.Time{}, response.Data.DeletedAt)
	})

	t.Run("error lock response", func(t *testing.T) {
		response := LockStackResponse{
			AtmosApiResponse: AtmosApiResponse{
				Request:      "lock-stack",
				Status:       409,
				Success:      false,
				ErrorMessage: "Stack is already locked",
			},
			Data: struct {
				ID          string    `json:"id,omitempty"`
				WorkspaceId string    `json:"workspaceId,omitempty"`
				Key         string    `json:"key,omitempty"`
				LockMessage string    `json:"lockMessage,omitempty"`
				ExpiresAt   time.Time `json:"expiresAt,omitempty"`
				CreatedAt   time.Time `json:"createdAt,omitempty"`
				UpdatedAt   time.Time `json:"updatedAt,omitempty"`
				DeletedAt   time.Time `json:"deletedAt,omitempty"`
			}{},
		}

		assert.Equal(t, "lock-stack", response.Request)
		assert.Equal(t, 409, response.Status)
		assert.False(t, response.Success)
		assert.Equal(t, "Stack is already locked", response.ErrorMessage)
		assert.Equal(t, "", response.Data.ID)
		assert.Equal(t, "", response.Data.WorkspaceId)
		assert.Equal(t, "", response.Data.Key)
		assert.Equal(t, "", response.Data.LockMessage)
		assert.Equal(t, time.Time{}, response.Data.ExpiresAt)
		assert.Equal(t, time.Time{}, response.Data.CreatedAt)
		assert.Equal(t, time.Time{}, response.Data.UpdatedAt)
		assert.Equal(t, time.Time{}, response.Data.DeletedAt)
	})
}

func TestUnlockStackRequest(t *testing.T) {
	t.Run("valid unlock request", func(t *testing.T) {
		req := UnlockStackRequest{
			Key: "test-stack/test-component",
		}

		assert.Equal(t, "test-stack/test-component", req.Key)
	})
}

func TestUnlockStackResponse(t *testing.T) {
	t.Run("successful unlock response", func(t *testing.T) {
		response := UnlockStackResponse{
			AtmosApiResponse: AtmosApiResponse{
				Request: "unlock-stack",
				Status:  200,
				Success: true,
			},
			Data: struct{}{},
		}

		assert.Equal(t, "unlock-stack", response.Request)
		assert.Equal(t, 200, response.Status)
		assert.True(t, response.Success)
	})

	t.Run("error unlock response", func(t *testing.T) {
		response := UnlockStackResponse{
			AtmosApiResponse: AtmosApiResponse{
				Request:      "unlock-stack",
				Status:       404,
				Success:      false,
				ErrorMessage: "Stack not found or not locked",
			},
			Data: struct{}{},
		}

		assert.Equal(t, "unlock-stack", response.Request)
		assert.Equal(t, 404, response.Status)
		assert.False(t, response.Success)
		assert.Equal(t, "Stack not found or not locked", response.ErrorMessage)
	})
}
