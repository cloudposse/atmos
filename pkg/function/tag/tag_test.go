package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog(t *testing.T) {
	tags := All()
	require.NotEmpty(t, tags)

	expectedTags := []string{
		Exec,
		Secret,
		Store,
		StoreGet,
		Template,
		TerraformOutput,
		TerraformState,
		Env,
		Include,
		IncludeRaw,
		RepoRoot,
		GitRoot,
		GitSha,
		GitBranch,
		GitRef,
		GitRepository,
		GitOwner,
		GitName,
		GitHost,
		GitURL,
		Append,
		Cwd,
		Unset,
		Random,
		Literal,
		AwsAccountID,
		AwsCallerIdentityArn,
		AwsCallerIdentityUserID,
		AwsRegion,
		AwsOrganizationID,
		Emulator,
	}

	assert.Equal(t, expectedTags, tags)
	for _, tag := range expectedTags {
		assert.True(t, IsValid(tag), "expected %s to be valid", tag)
	}
	assert.False(t, IsValid("envv"))
}

func TestYAMLHelpers(t *testing.T) {
	assert.Equal(t, "!", YAMLPrefix)
	assert.Equal(t, "!env", ToYAML(Env))
	assert.Equal(t, "env", FromYAML("!env"))
	assert.Equal(t, "env", FromYAML("env"))
	assert.Equal(t, "", FromYAML(""))

	yamlTags := AllYAML()
	require.Len(t, yamlTags, len(All()))
	for _, yamlTag := range yamlTags {
		assert.True(t, IsValidYAML(yamlTag), "expected %s to be valid", yamlTag)
		assert.Equal(t, yamlTag, ToYAML(FromYAML(yamlTag)))
	}

	assert.False(t, IsValidYAML("!envv"))
	assert.False(t, IsValidYAML("!!str"))
}

func TestAtmosConfigYAML(t *testing.T) {
	configTags := AtmosConfigYAML()
	require.NotEmpty(t, configTags)

	expected := []string{
		"!env",
		"!exec",
		"!include",
		"!include.raw",
		"!repo-root",
		"!git.root",
		"!git.sha",
		"!git.branch",
		"!git.ref",
		"!git.repository",
		"!git.owner",
		"!git.name",
		"!git.host",
		"!git.url",
		"!cwd",
		"!random",
		"!unset",
	}

	assert.Equal(t, expected, configTags)
	for _, tag := range expected {
		assert.True(t, IsValidYAML(tag), "expected %s to be in the full YAML catalog", tag)
		assert.True(t, IsAtmosConfigYAML(tag), "expected %s to be valid in atmos.yaml", tag)
	}

	assert.False(t, IsAtmosConfigYAML("!store"))
	assert.False(t, IsAtmosConfigYAML("!terraform.output"))
	assert.False(t, IsAtmosConfigYAML("!envv"))
	assert.False(t, IsAtmosConfigYAML("!!str"))
}
