package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRegistryWithoutExecutor(t *testing.T) {
	r := DefaultRegistry(nil)

	// Pre-merge functions.
	assert.True(t, r.Has("env"))
	assert.True(t, r.Has("template"))
	assert.True(t, r.Has("repo-root"))
	assert.True(t, r.Has("include"))
	assert.True(t, r.Has("include.raw"))
	assert.True(t, r.Has("random"))
	assert.False(t, r.Has("exec")) // Not registered without executor.

	// Post-merge placeholder functions.
	assert.True(t, r.Has("terraform.output"))
	assert.True(t, r.Has("terraform.state"))
	assert.True(t, r.Has("store"))
	assert.True(t, r.Has("store.get"))
	assert.True(t, r.Has("aws.account_id"))
	assert.True(t, r.Has("aws.caller_identity_arn"))
	assert.True(t, r.Has("aws.caller_identity_user_id"))
	assert.True(t, r.Has("aws.region"))

	// 6 pre-merge + 8 post-merge = 14, minus exec = 13.
	assert.Equal(t, 13, r.Len())
}

func TestDefaultRegistryWithExecutor(t *testing.T) {
	executor := &mockShellExecutor{}
	r := DefaultRegistry(executor)

	// Pre-merge functions.
	assert.True(t, r.Has("env"))
	assert.True(t, r.Has("template"))
	assert.True(t, r.Has("repo-root"))
	assert.True(t, r.Has("include"))
	assert.True(t, r.Has("include.raw"))
	assert.True(t, r.Has("random"))
	assert.True(t, r.Has("exec"))

	// Post-merge placeholder functions.
	assert.True(t, r.Has("terraform.output"))
	assert.True(t, r.Has("terraform.state"))
	assert.True(t, r.Has("store"))
	assert.True(t, r.Has("store.get"))
	assert.True(t, r.Has("aws.account_id"))
	assert.True(t, r.Has("aws.caller_identity_arn"))
	assert.True(t, r.Has("aws.caller_identity_user_id"))
	assert.True(t, r.Has("aws.region"))

	// 7 pre-merge + 8 post-merge = 15, but store has alias so registry counts 14.
	assert.Equal(t, 14, r.Len())
}

func TestTags(t *testing.T) {
	tags := Tags()

	// Pre-merge functions.
	assert.Equal(t, "env", tags[TagEnv])
	assert.Equal(t, "exec", tags[TagExec])
	assert.Equal(t, "template", tags[TagTemplate])
	assert.Equal(t, "repo-root", tags[TagRepoRoot])
	assert.Equal(t, "include", tags[TagInclude])
	assert.Equal(t, "include.raw", tags[TagIncludeRaw])
	assert.Equal(t, "random", tags[TagRandom])

	// Post-merge functions.
	assert.Equal(t, "terraform.output", tags[TagTerraformOutput])
	assert.Equal(t, "terraform.state", tags[TagTerraformState])
	assert.Equal(t, "store", tags[TagStore])
	assert.Equal(t, "store.get", tags[TagStoreGet])
	assert.Equal(t, "aws.account_id", tags[TagAwsAccountID])
	assert.Equal(t, "aws.caller_identity_arn", tags[TagAwsCallerIdentityArn])
	assert.Equal(t, "aws.caller_identity_user_id", tags[TagAwsCallerIdentityUserID])
	assert.Equal(t, "aws.region", tags[TagAwsRegion])
}

func TestAllTags(t *testing.T) {
	tags := AllTags()

	// Pre-merge tags.
	assert.Contains(t, tags, TagEnv)
	assert.Contains(t, tags, TagExec)
	assert.Contains(t, tags, TagTemplate)
	assert.Contains(t, tags, TagRepoRoot)
	assert.Contains(t, tags, TagInclude)
	assert.Contains(t, tags, TagIncludeRaw)
	assert.Contains(t, tags, TagRandom)

	// Post-merge tags.
	assert.Contains(t, tags, TagTerraformOutput)
	assert.Contains(t, tags, TagTerraformState)
	assert.Contains(t, tags, TagStore)
	assert.Contains(t, tags, TagStoreGet)
	assert.Contains(t, tags, TagAwsAccountID)
	assert.Contains(t, tags, TagAwsCallerIdentityArn)
	assert.Contains(t, tags, TagAwsCallerIdentityUserID)
	assert.Contains(t, tags, TagAwsRegion)

	assert.Len(t, tags, 15)
}
