package dtos

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// DriftDetectionUploadRequest represents the data structure for uploading components for drift detection.
// We call this from "atmos list deployments".
type DriftDetectionUploadRequest struct {
	RepoURL     string              `json:"repo_url"`
	RepoName    string              `json:"repo_name"`
	RepoOwner   string              `json:"repo_owner"`
	RepoHost    string              `json:"repo_host"`
	Deployments []schema.Deployment `json:"stacks"`
}
