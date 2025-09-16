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
				Settings:      map[string]interface{}{"drift_detection_enabled": true},
				Vars:          map[string]interface{}{"environment": "prod"},
				Env:           map[string]interface{}{"TF_VAR_region": "us-west-2"},
				Backend:       map[string]interface{}{"type": "s3"},
				Metadata:      map[string]interface{}{"description": "Web application"},
			},
			{
				Component:     "api",
				Stack:         "staging",
				ComponentType: "helm",
				Settings:      map[string]interface{}{"drift_detection_enabled": false},
				Vars:          map[string]interface{}{"environment": "staging"},
				Env:           map[string]interface{}{"HELM_NAMESPACE": "staging"},
				Backend:       map[string]interface{}{"type": "local"},
				Metadata:      map[string]interface{}{"description": "API service"},
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

		// Test first deployment
		assert.Equal(t, "web-app", req.Deployments[0].Component)
		assert.Equal(t, "prod", req.Deployments[0].Stack)
		assert.Equal(t, "terraform", req.Deployments[0].ComponentType)
		assert.Equal(t, map[string]interface{}{"drift_detection_enabled": true}, req.Deployments[0].Settings)
		assert.Equal(t, map[string]interface{}{"environment": "prod"}, req.Deployments[0].Vars)
		assert.Equal(t, map[string]interface{}{"TF_VAR_region": "us-west-2"}, req.Deployments[0].Env)
		assert.Equal(t, map[string]interface{}{"type": "s3"}, req.Deployments[0].Backend)
		assert.Equal(t, map[string]interface{}{"description": "Web application"}, req.Deployments[0].Metadata)

		// Test second deployment
		assert.Equal(t, "api", req.Deployments[1].Component)
		assert.Equal(t, "staging", req.Deployments[1].Stack)
		assert.Equal(t, "helm", req.Deployments[1].ComponentType)
		assert.Equal(t, map[string]interface{}{"drift_detection_enabled": false}, req.Deployments[1].Settings)
		assert.Equal(t, map[string]interface{}{"environment": "staging"}, req.Deployments[1].Vars)
		assert.Equal(t, map[string]interface{}{"HELM_NAMESPACE": "staging"}, req.Deployments[1].Env)
		assert.Equal(t, map[string]interface{}{"type": "local"}, req.Deployments[1].Backend)
		assert.Equal(t, map[string]interface{}{"description": "API service"}, req.Deployments[1].Metadata)
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
