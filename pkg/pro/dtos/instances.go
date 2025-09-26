package dtos

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// InstancesUploadRequest represents the data structure for uploading components for drift detection.
// We call this from "atmos list instances".
type InstancesUploadRequest struct {
	RepoURL   string            `json:"repo_url"`
	RepoName  string            `json:"repo_name"`
	RepoOwner string            `json:"repo_owner"`
	RepoHost  string            `json:"repo_host"`
	Instances []schema.Instance `json:"instances"`
}

// InstanceStatusUploadRequest represents the data structure for uploading a single instance's status.
type InstanceStatusUploadRequest struct {
	AtmosProRunID string `json:"atmos_pro_run_id"`
	GitSHA        string `json:"git_sha"`
	RepoURL       string `json:"repo_url"`
	RepoName      string `json:"repo_name"`
	RepoOwner     string `json:"repo_owner"`
	RepoHost      string `json:"repo_host"`
	Component     string `json:"component"`
	Stack         string `json:"stack"`
	HasDrift      bool   `json:"has_drift"`
}
