package dtos

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// DeploymentsUploadRequest represents the data structure for uploading components for drift detection.
// We call this from "atmos list deployments".
type DeploymentsUploadRequest struct {
	RepoURL     string              `json:"repo_url"`
	RepoName    string              `json:"repo_name"`
	RepoOwner   string              `json:"repo_owner"`
	RepoHost    string              `json:"repo_host"`
	Deployments []schema.Deployment `json:"deployments"`
}

// DeploymentStatusUploadRequest represents the data structure for uploading a single deployment's status.
type DeploymentStatusUploadRequest struct {
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
