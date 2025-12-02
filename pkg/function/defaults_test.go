package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRegistryWithoutExecutor(t *testing.T) {
	r := DefaultRegistry(nil)

	assert.True(t, r.Has("env"))
	assert.True(t, r.Has("template"))
	assert.True(t, r.Has("repo-root"))
	assert.False(t, r.Has("exec")) // Not registered without executor.
	assert.Equal(t, 3, r.Len())
}

func TestDefaultRegistryWithExecutor(t *testing.T) {
	executor := &mockShellExecutor{}
	r := DefaultRegistry(executor)

	assert.True(t, r.Has("env"))
	assert.True(t, r.Has("template"))
	assert.True(t, r.Has("repo-root"))
	assert.True(t, r.Has("exec"))
	assert.Equal(t, 4, r.Len())
}

func TestTags(t *testing.T) {
	tags := Tags()

	assert.Equal(t, "env", tags[TagEnv])
	assert.Equal(t, "exec", tags[TagExec])
	assert.Equal(t, "template", tags[TagTemplate])
	assert.Equal(t, "repo-root", tags[TagGitRoot])
}

func TestAllTags(t *testing.T) {
	tags := AllTags()

	assert.Contains(t, tags, TagEnv)
	assert.Contains(t, tags, TagExec)
	assert.Contains(t, tags, TagTemplate)
	assert.Contains(t, tags, TagGitRoot)
	assert.Len(t, tags, 4)
}
