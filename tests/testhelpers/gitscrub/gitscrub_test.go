package gitscrub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilter_DropsRepositoryLocationAndKeepsTheRest(t *testing.T) {
	env := []string{
		"PATH=/usr/bin:/bin",
		"GIT_DIR=/caller/.git",
		"git_work_tree=/caller",
		"Git_Index_File=/caller/.git/index",
		"GIT_CONFIG_COUNT=1",
		"GIT_SSH_COMMAND=ssh -o IdentitiesOnly=yes",
		"GIT_ASKPASS=/bin/askpass",
	}

	kept := Filter(env)

	assert.Equal(t, []string{
		"PATH=/usr/bin:/bin",
		"GIT_CONFIG_COUNT=1",
		"GIT_SSH_COMMAND=ssh -o IdentitiesOnly=yes",
		"GIT_ASKPASS=/bin/askpass",
	}, kept, "repository-location variables go, in any casing; authentication variables stay")
	assert.Equal(t, "GIT_DIR=/caller/.git", env[1], "the input is not modified")
}

// Environ must reflect the live process environment, minus the repository-location variables.
func TestEnviron_ScrubsTheProcessEnvironment(t *testing.T) {
	t.Setenv("GIT_DIR", "/caller/.git")
	t.Setenv("GITSCRUB_TEST_MARKER", "kept")

	env := Environ()

	assert.NotContains(t, env, "GIT_DIR=/caller/.git")
	assert.Contains(t, env, "GITSCRUB_TEST_MARKER=kept")
}
