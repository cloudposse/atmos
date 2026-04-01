package dtos

import (
	"testing"

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
			Command:       "plan",
			ExitCode:      2,
		}

		assert.Equal(t, "run-123", req.AtmosProRunID)
		assert.Equal(t, "abc123def456", req.GitSHA)
		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Equal(t, "test-component", req.Component)
		assert.Equal(t, "test-stack", req.Stack)
		assert.Equal(t, "plan", req.Command)
		assert.Equal(t, 2, req.ExitCode)
	})

	t.Run("valid request with minimal fields", func(t *testing.T) {
		req := InstanceStatusUploadRequest{
			RepoName:  "test-repo",
			RepoOwner: "test-owner",
			Component: "test-component",
			Stack:     "test-stack",
			Command:   "apply",
			ExitCode:  0,
		}

		assert.Equal(t, "", req.AtmosProRunID)
		assert.Equal(t, "", req.GitSHA)
		assert.Equal(t, "", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "", req.RepoHost)
		assert.Equal(t, "test-component", req.Component)
		assert.Equal(t, "test-stack", req.Stack)
		assert.Equal(t, "apply", req.Command)
		assert.Equal(t, 0, req.ExitCode)
	})
}

func TestInstancesUploadRequest(t *testing.T) {
	t.Run("valid request with instances", func(t *testing.T) {
		instances := []UploadInstance{
			{
				Component:     "web-app",
				Stack:         "prod",
				ComponentType: "terraform",
				Settings:      map[string]interface{}{"pro": map[string]interface{}{"drift_detection": map[string]interface{}{"enabled": true}}},
			},
			{
				Component:     "api",
				Stack:         "staging",
				ComponentType: "helm",
				Settings:      map[string]interface{}{"pro": map[string]interface{}{"drift_detection": map[string]interface{}{"enabled": false}}},
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
		assert.NotNil(t, req.Instances[0].Settings)

		// Test second instance
		assert.Equal(t, "api", req.Instances[1].Component)
		assert.Equal(t, "staging", req.Instances[1].Stack)
		assert.Equal(t, "helm", req.Instances[1].ComponentType)
		assert.NotNil(t, req.Instances[1].Settings)
	})

	t.Run("valid request with empty instances", func(t *testing.T) {
		req := InstancesUploadRequest{
			RepoURL:   "https://github.com/test-owner/test-repo",
			RepoName:  "test-repo",
			RepoOwner: "test-owner",
			RepoHost:  "github.com",
			Instances: []UploadInstance{},
		}

		assert.Equal(t, "https://github.com/test-owner/test-repo", req.RepoURL)
		assert.Equal(t, "test-repo", req.RepoName)
		assert.Equal(t, "test-owner", req.RepoOwner)
		assert.Equal(t, "github.com", req.RepoHost)
		assert.Len(t, req.Instances, 0)
	})
}
