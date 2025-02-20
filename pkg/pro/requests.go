package pro

import (
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

type LockStackRequest struct {
	Key         string         `json:"key"`
	TTL         int32          `json:"ttl"`
	LockMessage string         `json:"lockMessage,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
}

type UnlockStackRequest struct {
	Key string `json:"key"`
}

type Property struct {
	ID        string    `json:"id,omitempty"`
	LockID    string    `json:"lockId,omitempty"`
	Key       string    `json:"key,omitempty"`
	Value     string    `json:"value,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	DeletedAt time.Time `json:"deletedAt,omitempty"`
}

type AtmosApiResponse struct {
	Request      string         `json:"request"`
	Success      bool           `json:"success"`
	ErrorMessage string         `json:"errorMessage,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
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

type UnlockStackResponse struct {
	AtmosApiResponse
	Data struct{} `json:"data"`
}

type AffectedStacksUploadRequest struct {
	HeadSHA   string            `json:"head_sha"`
	BaseSHA   string            `json:"base_sha"`
	RepoURL   string            `json:"repo_url"`
	RepoName  string            `json:"repo_name"`
	RepoOwner string            `json:"repo_owner"`
	RepoHost  string            `json:"repo_host"`
	Stacks    []schema.Affected `json:"stacks"`
}
