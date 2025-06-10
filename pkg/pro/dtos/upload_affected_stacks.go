package dtos

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

type UploadAffectedStacksRequest struct {
	HeadSHA   string            `json:"head_sha"`
	BaseSHA   string            `json:"base_sha"`
	RepoURL   string            `json:"repo_url"`
	RepoName  string            `json:"repo_name"`
	RepoOwner string            `json:"repo_owner"`
	RepoHost  string            `json:"repo_host"`
	Stacks    []schema.Affected `json:"stacks"`
}
