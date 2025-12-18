package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllTags(t *testing.T) {
	tags := AllTags()

	// Verify all expected tags are present.
	expectedTags := []string{
		TagExec,
		TagStore,
		TagStoreGet,
		TagTemplate,
		TagTerraformOutput,
		TagTerraformState,
		TagEnv,
		TagInclude,
		TagIncludeRaw,
		TagRepoRoot,
		TagRandom,
		TagLiteral,
		TagAwsAccountID,
		TagAwsCallerIdentityArn,
		TagAwsCallerIdentityUserID,
		TagAwsRegion,
	}

	assert.Equal(t, len(expectedTags), len(tags))

	for _, expected := range expectedTags {
		assert.Contains(t, tags, expected, "expected tag %s to be in AllTags()", expected)
	}
}

func TestTagsMap(t *testing.T) {
	// Verify all expected tags are in the map.
	expectedTags := []string{
		TagExec,
		TagStore,
		TagStoreGet,
		TagTemplate,
		TagTerraformOutput,
		TagTerraformState,
		TagEnv,
		TagInclude,
		TagIncludeRaw,
		TagRepoRoot,
		TagRandom,
		TagLiteral,
		TagAwsAccountID,
		TagAwsCallerIdentityArn,
		TagAwsCallerIdentityUserID,
		TagAwsRegion,
	}

	for _, tag := range expectedTags {
		assert.True(t, TagsMap[tag], "expected tag %s to be in TagsMap", tag)
	}

	// Verify non-existent tag returns false.
	assert.False(t, TagsMap["non-existent-tag"])
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
		{TagInclude, "!include"},
		{TagIncludeRaw, "!include.raw"},
		{TagRepoRoot, "!repo-root"},
		{TagRandom, "!random"},
		{TagLiteral, "!literal"},
		{TagAwsAccountID, "!aws.account_id"},
		{TagAwsCallerIdentityArn, "!aws.caller_identity_arn"},
		{TagAwsCallerIdentityUserID, "!aws.caller_identity_user_id"},
		{TagAwsRegion, "!aws.region"},
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
		{"!include", "include"},
		{"!include.raw", "include.raw"},
		{"!repo-root", "repo-root"},
		{"!random", "random"},
		{"!literal", "literal"},
		{"!aws.account_id", "aws.account_id"},
		{"!aws.caller_identity_arn", "aws.caller_identity_arn"},
		{"!aws.caller_identity_user_id", "aws.caller_identity_user_id"},
		{"!aws.region", "aws.region"},
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
	assert.Equal(t, "include", TagInclude)
	assert.Equal(t, "include.raw", TagIncludeRaw)
	assert.Equal(t, "repo-root", TagRepoRoot)
	assert.Equal(t, "random", TagRandom)
	assert.Equal(t, "literal", TagLiteral)
	assert.Equal(t, "aws.account_id", TagAwsAccountID)
	assert.Equal(t, "aws.caller_identity_arn", TagAwsCallerIdentityArn)
	assert.Equal(t, "aws.caller_identity_user_id", TagAwsCallerIdentityUserID)
	assert.Equal(t, "aws.region", TagAwsRegion)
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

func TestTagsMap_Consistency(t *testing.T) {
	// Verify TagsMap is consistent with AllTags.
	tags := AllTags()

	// All tags in AllTags should be in TagsMap.
	for _, tag := range tags {
		assert.True(t, TagsMap[tag], "tag %s in AllTags() but not in TagsMap", tag)
	}

	// Count should match.
	assert.Equal(t, len(tags), len(TagsMap))
}
