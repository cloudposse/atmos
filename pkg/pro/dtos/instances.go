package dtos

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// InstancesUploadRequest represents the data structure for uploading components for drift detection.
// We call this from "atmos list instances".
type InstancesUploadRequest struct {
	RepoURL    string            `json:"repo_url"`
	RepoName   string            `json:"repo_name"`
	RepoOwner  string            `json:"repo_owner"`
	RepoHost   string            `json:"repo_host"`
	Instances  []schema.Instance `json:"instances"`
	BatchID    string            `json:"batch_id,omitempty"`
	BatchIndex *int              `json:"batch_index,omitempty"`
	BatchTotal *int              `json:"batch_total,omitempty"`
}

// InstanceStatusUploadRequest represents the data structure for uploading a single instance's status.
type InstanceStatusUploadRequest struct {
	AtmosProRunID string `json:"atmos_pro_run_id"`
	AtmosVersion  string `json:"atmos_version"`
	AtmosOS       string `json:"atmos_os"`
	AtmosArch     string `json:"atmos_arch"`
	GitSHA        string `json:"git_sha"`
	RepoURL       string `json:"repo_url"`
	RepoName      string `json:"repo_name"`
	RepoOwner     string `json:"repo_owner"`
	RepoHost      string `json:"repo_host"`
	Component     string `json:"component"`
	Stack         string `json:"stack"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exit_code"`
	// CI contains structured plan/apply data as a flexible map.
	// Each component type populates its own keys. Omitted when ci.enabled is false.
	CI map[string]any `json:"ci,omitempty"`
}
