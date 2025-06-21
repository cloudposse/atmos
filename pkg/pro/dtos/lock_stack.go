package dtos

import (
	"time"
)

type LockStackRequest struct {
	Key         string                 `json:"key"`
	TTL         int32                  `json:"ttl"`
	LockMessage string                 `json:"lockMessage,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

type LockStackResponse struct {
	AtmosApiResponse
	Data struct {
		ID          string    `json:"id,omitempty"`
		WorkspaceId string    `json:"workspaceId,omitempty"`
		Key         string    `json:"key,omitempty"`
		LockMessage string    `json:"lockMessage,omitempty"`
		ExpiresAt   time.Time `json:"expiresAt,omitempty"`
		CreatedAt   time.Time `json:"createdAt,omitempty"`
		UpdatedAt   time.Time `json:"updatedAt,omitempty"`
		DeletedAt   time.Time `json:"deletedAt,omitempty"`
	} `json:"data"`
}
