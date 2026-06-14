package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---- extractHookNameAndArgs ----

func TestExtractHookNameAndArgs_Basic(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"pre-commit"})
	assert.Equal(t, "pre-commit", name)
	assert.Empty(t, args)
}

func TestExtractHookNameAndArgs_WithArgs(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"commit-msg", ".git/COMMIT_EDITMSG"})
	assert.Equal(t, "commit-msg", name)
	assert.Equal(t, []string{".git/COMMIT_EDITMSG"}, args)
}

func TestExtractHookNameAndArgs_Empty(t *testing.T) {
	name, args := extractHookNameAndArgs(nil)
	assert.Equal(t, "", name)
	assert.Nil(t, args)
}

func TestExtractHookNameAndArgs_SkipsLeadingFlags(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"--verbose", "pre-commit", "extra"})
	assert.Equal(t, "pre-commit", name)
	assert.Equal(t, []string{"extra"}, args)
}

// ---- config plumbing ----

func TestGitConfig_NilWhenUnset(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	SetAtmosConfig(nil)
	assert.Nil(t, gitConfig())
}
