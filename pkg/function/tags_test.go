package function

import (
	"testing"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTags(t *testing.T) {
	tags := AllTags()

	// Verify all expected tags are present.
	expectedTags := []string{
		TagExec,
		TagSecret,
		TagStore,
		TagStoreGet,
		TagTemplate,
		TagTerraformOutput,
		TagTerraformState,
		TagEnv,
		TagCEL,
		TagInclude,
		TagIncludeRaw,
		TagRepoRoot,
		TagGitRoot,
		TagGitSha,
		TagGitBranch,
		TagGitRef,
		TagGitRepository,
		TagGitOwner,
		TagGitName,
		TagGitHost,
		TagGitURL,
		TagAppend,
		TagCwd,
		TagUnset,
		TagRandom,
		TagLiteral,
		TagAwsAccountID,
		TagAwsCallerIdentityArn,
		TagAwsCallerIdentityUserID,
		TagAwsRegion,
		TagAwsOrganizationID,
		TagEmulator,
	}

	assert.Equal(t, len(expectedTags), len(tags))

	for _, expected := range expectedTags {
		assert.Contains(t, tags, expected, "expected tag %s to be in AllTags()", expected)
	}
}

func TestIsValidTag(t *testing.T) {
	// Verify all expected tags are valid.
	expectedTags := []string{
		TagExec,
		TagSecret,
		TagStore,
		TagStoreGet,
		TagTemplate,
		TagTerraformOutput,
		TagTerraformState,
		TagEnv,
		TagCEL,
		TagInclude,
		TagIncludeRaw,
		TagRepoRoot,
		TagGitRoot,
		TagGitSha,
		TagGitBranch,
		TagGitRef,
		TagGitRepository,
		TagGitOwner,
		TagGitName,
		TagGitHost,
		TagGitURL,
		TagAppend,
		TagCwd,
		TagUnset,
		TagRandom,
		TagLiteral,
		TagAwsAccountID,
		TagAwsCallerIdentityArn,
		TagAwsCallerIdentityUserID,
		TagAwsRegion,
		TagAwsOrganizationID,
		TagEmulator,
	}

	for _, tag := range expectedTags {
		assert.True(t, IsValidTag(tag), "expected tag %s to be valid", tag)
	}

	// Verify non-existent tag returns false.
	assert.False(t, IsValidTag("non-existent-tag"))
}

func TestYAMLTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{TagEnv, "!env"},
		{TagExec, "!exec"},
		{TagStore, "!store"},
		{TagStoreGet, "!store.get"},
		{TagTemplate, "!template"},
		{TagTerraformOutput, "!terraform.output"},
		{TagTerraformState, "!terraform.state"},
		{TagCEL, "!cel"},
		{TagInclude, "!include"},
		{TagIncludeRaw, "!include.raw"},
		{TagRepoRoot, "!repo-root"},
		{TagGitRoot, "!git.root"},
		{TagGitSha, "!git.sha"},
		{TagGitBranch, "!git.branch"},
		{TagGitRef, "!git.ref"},
		{TagGitRepository, "!git.repository"},
		{TagGitOwner, "!git.owner"},
		{TagGitName, "!git.name"},
		{TagGitHost, "!git.host"},
		{TagGitURL, "!git.url"},
		{TagAppend, "!append"},
		{TagCwd, "!cwd"},
		{TagUnset, "!unset"},
		{TagRandom, "!random"},
		{TagLiteral, "!literal"},
		{TagAwsAccountID, "!aws.account_id"},
		{TagAwsCallerIdentityArn, "!aws.caller_identity_arn"},
		{TagAwsCallerIdentityUserID, "!aws.caller_identity_user_id"},
		{TagAwsRegion, "!aws.region"},
		{TagAwsOrganizationID, "!aws.organization_id"},
		{TagEmulator, "!emulator"},
		{"custom", "!custom"},
		{"", "!"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := YAMLTag(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFromYAMLTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"!env", "env"},
		{"!exec", "exec"},
		{"!store", "store"},
		{"!store.get", "store.get"},
		{"!template", "template"},
		{"!terraform.output", "terraform.output"},
		{"!terraform.state", "terraform.state"},
		{"!cel", "cel"},
		{"!include", "include"},
		{"!include.raw", "include.raw"},
		{"!repo-root", "repo-root"},
		{"!git.root", "git.root"},
		{"!git.sha", "git.sha"},
		{"!git.branch", "git.branch"},
		{"!git.ref", "git.ref"},
		{"!git.repository", "git.repository"},
		{"!git.owner", "git.owner"},
		{"!git.name", "git.name"},
		{"!git.host", "git.host"},
		{"!git.url", "git.url"},
		{"!append", "append"},
		{"!cwd", "cwd"},
		{"!unset", "unset"},
		{"!random", "random"},
		{"!literal", "literal"},
		{"!aws.account_id", "aws.account_id"},
		{"!aws.caller_identity_arn", "aws.caller_identity_arn"},
		{"!aws.caller_identity_user_id", "aws.caller_identity_user_id"},
		{"!aws.region", "aws.region"},
		{"!aws.organization_id", "aws.organization_id"},
		{"!emulator", "emulator"},
		{"!custom", "custom"},
		// Without prefix - returns as-is.
		{"env", "env"},
		{"store", "store"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := FromYAMLTag(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestYAMLTagPrefix(t *testing.T) {
	assert.Equal(t, "!", YAMLTagPrefix)
}

func TestTagConstants(t *testing.T) {
	// Verify tag constant values.
	assert.Equal(t, "exec", TagExec)
	assert.Equal(t, "store", TagStore)
	assert.Equal(t, "store.get", TagStoreGet)
	assert.Equal(t, "template", TagTemplate)
	assert.Equal(t, "terraform.output", TagTerraformOutput)
	assert.Equal(t, "terraform.state", TagTerraformState)
	assert.Equal(t, "env", TagEnv)
	assert.Equal(t, "cel", TagCEL)
	assert.Equal(t, "include", TagInclude)
	assert.Equal(t, "include.raw", TagIncludeRaw)
	assert.Equal(t, "repo-root", TagRepoRoot)
	assert.Equal(t, "git.root", TagGitRoot)
	assert.Equal(t, "git.sha", TagGitSha)
	assert.Equal(t, "git.branch", TagGitBranch)
	assert.Equal(t, "git.ref", TagGitRef)
	assert.Equal(t, "git.repository", TagGitRepository)
	assert.Equal(t, "git.owner", TagGitOwner)
	assert.Equal(t, "git.name", TagGitName)
	assert.Equal(t, "git.host", TagGitHost)
	assert.Equal(t, "git.url", TagGitURL)
	assert.Equal(t, "append", TagAppend)
	assert.Equal(t, "cwd", TagCwd)
	assert.Equal(t, "unset", TagUnset)
	assert.Equal(t, "random", TagRandom)
	assert.Equal(t, "literal", TagLiteral)
	assert.Equal(t, "aws.account_id", TagAwsAccountID)
	assert.Equal(t, "aws.caller_identity_arn", TagAwsCallerIdentityArn)
	assert.Equal(t, "aws.caller_identity_user_id", TagAwsCallerIdentityUserID)
	assert.Equal(t, "aws.region", TagAwsRegion)
	assert.Equal(t, "aws.organization_id", TagAwsOrganizationID)
	assert.Equal(t, "emulator", TagEmulator)
}

func TestYAMLTag_RoundTrip(t *testing.T) {
	// Test that YAMLTag and FromYAMLTag are inverse operations.
	tags := AllTags()

	for _, tag := range tags {
		yamlTag := YAMLTag(tag)
		recovered := FromYAMLTag(yamlTag)
		assert.Equal(t, tag, recovered, "round-trip failed for tag %s", tag)
	}
}

func TestIsValidTag_Consistency(t *testing.T) {
	// Verify IsValidTag is consistent with AllTags.
	tags := AllTags()

	// All tags in AllTags should be valid.
	for _, tag := range tags {
		assert.True(t, IsValidTag(tag), "tag %s in AllTags() but IsValidTag returns false", tag)
	}
}

func TestYAMLTagHelpers(t *testing.T) {
	yamlTags := AllYAMLTags()
	require.Len(t, yamlTags, len(AllTags()))

	for _, yamlTag := range yamlTags {
		assert.True(t, IsValidYAMLTag(yamlTag), "expected %s to be valid", yamlTag)
		assert.Contains(t, yamlTags, YAMLTag(FromYAMLTag(yamlTag)))
	}

	assert.False(t, IsValidYAMLTag("!envv"))
	assert.False(t, IsValidYAMLTag("!!str"))
	assert.Equal(t, AllTags(), fntag.All(), "function and lightweight YAML tag catalogs must stay in sync")
	assert.Equal(t, AllYAMLTags(), fntag.AllYAML(), "YAML tag catalogs must stay in sync")
}
