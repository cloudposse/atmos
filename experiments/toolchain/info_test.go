package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoCommand_AliasResolution(t *testing.T) {
	// Test that alias resolution works correctly
	installer := NewInstaller()

	// Test with alias
	owner, repo, err := installer.parseToolSpec("terraform")
	assert.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)

	// Find the tool configuration (use "latest" as default version)
	tool, err := installer.findTool(owner, repo, "latest")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
}

func TestInfoCommand_CanonicalOrgRepo(t *testing.T) {
	// Test that canonical org/repo specification works correctly
	installer := NewInstaller()

	// Test with canonical org/repo
	owner, repo, err := installer.parseToolSpec("hashicorp/terraform")
	assert.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)

	// Find the tool configuration (use "latest" as default version)
	tool, err := installer.findTool(owner, repo, "latest")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
}

func TestInfoCommand_GitHubReleaseTool(t *testing.T) {
	// Test with a GitHub release tool
	installer := NewInstaller()

	// Test with opentofu (should be in local config)
	owner, repo, err := installer.parseToolSpec("opentofu")
	assert.NoError(t, err)
	assert.Equal(t, "opentofu", owner)
	assert.Equal(t, "opentofu", repo)

	// Find the tool configuration (use "latest" as default version)
	tool, err := installer.findTool(owner, repo, "latest")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "github_release", tool.Type)
	assert.Equal(t, "opentofu", tool.RepoOwner)
	assert.Equal(t, "opentofu", tool.RepoName)
}

func TestToolToYAML(t *testing.T) {
	// Test the toolToYAML function
	tool := &Tool{
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "test-asset",
		Format:    "zip",
	}

	yamlData, err := toolToYAML(tool)
	assert.NoError(t, err)
	assert.Contains(t, yamlData, "type: http")
	assert.Contains(t, yamlData, "repo_owner: test")
	assert.Contains(t, yamlData, "repo_name: tool")
	assert.Contains(t, yamlData, "asset: test-asset")
	assert.Contains(t, yamlData, "format: zip")
}
