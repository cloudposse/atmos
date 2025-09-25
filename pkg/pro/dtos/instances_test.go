package dtos

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestInstanceStatusUploadRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := InstanceStatusUploadRequest{
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
		req := InstanceStatusUploadRequest{
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

func TestInstancesUploadRequest(t *testing.T) {
	t.Run("valid request with instances", func(t *testing.T) {
		instances := []schema.Instance{
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

		req := InstancesUploadRequest{
			RepoURL:   "https://github.com/test-owner/test-repo",
			RepoName:  "test-repo",
			RepoOwner: "test-owner",
			RepoHost:  "github.com",
			Instances: instances,
		}

		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Len(t, req.Instances, 2)

		// Test first instance
		assert.Equal(t, "web-app", req.Instances[0].Component)
		assert.Equal(t, "prod", req.Instances[0].Stack)
		assert.Equal(t, "terraform", req.Instances[0].ComponentType)
		assert.Equal(t, map[string]interface{}{"drift_detection_enabled": true}, req.Instances[0].Settings)
		assert.Equal(t, map[string]interface{}{"environment": "prod"}, req.Instances[0].Vars)
		assert.Equal(t, map[string]interface{}{"TF_VAR_region": "us-west-2"}, req.Instances[0].Env)
		assert.Equal(t, map[string]interface{}{"type": "s3"}, req.Instances[0].Backend)
		assert.Equal(t, map[string]interface{}{"description": "Web application"}, req.Instances[0].Metadata)

		// Test second instance
		assert.Equal(t, "api", req.Instances[1].Component)
		assert.Equal(t, "staging", req.Instances[1].Stack)
		assert.Equal(t, "helm", req.Instances[1].ComponentType)
		assert.Equal(t, map[string]interface{}{"drift_detection_enabled": false}, req.Instances[1].Settings)
		assert.Equal(t, map[string]interface{}{"environment": "staging"}, req.Instances[1].Vars)
		assert.Equal(t, map[string]interface{}{"HELM_NAMESPACE": "staging"}, req.Instances[1].Env)
		assert.Equal(t, map[string]interface{}{"type": "local"}, req.Instances[1].Backend)
		assert.Equal(t, map[string]interface{}{"description": "API service"}, req.Instances[1].Metadata)
	})

	t.Run("valid request with empty instances", func(t *testing.T) {
		req := InstancesUploadRequest{
			RepoURL:   "https://github.com/test-owner/test-repo",
			RepoName:  "test-repo",
			RepoOwner: "test-owner",
			RepoHost:  "github.com",
			Instances: []schema.Instance{},
		}

		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Len(t, req.Instances, 0)
	})
}
