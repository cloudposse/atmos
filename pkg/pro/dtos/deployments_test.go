package dtos

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestDeploymentStatusUploadRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := DeploymentStatusUploadRequest{
			AtmosProRunID: "run-123",
			GitSHA:        "abc123def456",
			RepoURL:       "https://github.com/test-owner/test-repo",
			RepoName:      "test-repo",
			RepoOwner:     "test-owner",
			RepoHost:      "github.com",
			Component:     "test-component",
			Stack:         "test-stack",
			HasDrift:      true,
		}

		assert.Equal(t, "run-123", req.AtmosProRunID)
		assert.Equal(t, "abc123def456", req.GitSHA)
		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Equal(t, "test-component", req.Component)
		assert.Equal(t, "test-stack", req.Stack)
		assert.True(t, req.HasDrift)
	})

	t.Run("valid request with minimal fields", func(t *testing.T) {
		req := DeploymentStatusUploadRequest{
			RepoName:  "test-repo",
			RepoOwner: "test-owner",
			Component: "test-component",
			Stack:     "test-stack",
			HasDrift:  false,
		}

		assert.Equal(t, "", req.AtmosProRunID)
		assert.Equal(t, "", req.GitSHA)
		assert.Equal(t, "", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "", req.RepoHost)
		assert.Equal(t, "test-component", req.Component)
		assert.Equal(t, "test-stack", req.Stack)
		assert.False(t, req.HasDrift)
	})
}

func TestDeploymentsUploadRequest(t *testing.T) {
	t.Run("valid request with deployments", func(t *testing.T) {
		deployments := []schema.Deployment{
			{
				Component:     "web-app",
				Stack:         "prod",
				ComponentType: "terraform",
				DriftDetectionEnabled: true,
				Workflows: []schema.DeploymentWorkflow{
					{
						Event:   "detect",
						Workflow: "atmos-pro-terraform-plan.yaml",
					},
					{
						Event:   "remediate",
						Workflow: "atmos-pro-terraform-apply.yaml",
					},
				},
			},
			{
				Component:     "api",
				Stack:         "staging",
				ComponentType: "helm",
				DriftDetectionEnabled: false,
				Workflows: []schema.DeploymentWorkflow{
					{
						Event:   "detect",
						Workflow: "atmos-pro-helm-plan.yaml",
					},
				},
			},
		}

		req := DeploymentsUploadRequest{
			RepoURL:     "https://github.com/test-owner/test-repo",
			RepoName:    "test-repo",
			RepoOwner:   "test-owner",
			RepoHost:    "github.com",
			Deployments: deployments,
		}

		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Len(t, req.Deployments, 2)
		assert.Equal(t, "web-app", req.Deployments[0].Component)
		assert.Equal(t, "prod", req.Deployments[0].Stack)
		assert.Equal(t, "terraform", req.Deployments[0].ComponentType)
		assert.True(t, req.Deployments[0].DriftDetectionEnabled)
		assert.Len(t, req.Deployments[0].Workflows, 2)
		assert.Equal(t, "detect", req.Deployments[0].Workflows[0].Event)
		assert.Equal(t, "atmos-pro-terraform-plan.yaml", req.Deployments[0].Workflows[0].Workflow)
		assert.Equal(t, "remediate", req.Deployments[0].Workflows[1].Event)
		assert.Equal(t, "atmos-pro-terraform-apply.yaml", req.Deployments[0].Workflows[1].Workflow)

		assert.Equal(t, "api", req.Deployments[1].Component)
		assert.Equal(t, "staging", req.Deployments[1].Stack)
		assert.Equal(t, "helm", req.Deployments[1].ComponentType)
		assert.False(t, req.Deployments[1].DriftDetectionEnabled)
		assert.Len(t, req.Deployments[1].Workflows, 1)
	})

	t.Run("valid request with empty deployments", func(t *testing.T) {
		req := DeploymentsUploadRequest{
			RepoURL:     "https://github.com/test-owner/test-repo",
			RepoName:    "test-repo",
			RepoOwner:   "test-owner",
			RepoHost:    "github.com",
			Deployments: []schema.Deployment{},
		}

		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Len(t, req.Deployments, 0)
	})
}
