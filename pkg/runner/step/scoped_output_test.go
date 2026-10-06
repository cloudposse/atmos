package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVariablesCloneIsolation(t *testing.T) {
	source := NewVariables()
	source.SetEnv("TEST_BRANCH", "original")
	source.SetFlag("region", "east")
	source.Set("prior", NewStepResult("saved"))
	branch := source.Clone()
	branch.SetEnv("TEST_BRANCH", "branch")
	branch.SetFlag("region", "west")
	branch.Set("later", NewStepResult("branch-result"))
	assert.Equal(t, "original", source.Env["TEST_BRANCH"])
	assert.Equal(t, "east", source.Flags["region"])
	assert.NotContains(t, source.Steps, "later")
	source.SetEnv("TEST_BRANCH", "source")
	source.SetFlag("region", "north")
	source.Set("source-only", NewStepResult("source-result"))
	assert.Equal(t, "branch", branch.Env["TEST_BRANCH"])
	assert.Equal(t, "west", branch.Flags["region"])
	assert.NotContains(t, branch.Steps, "source-only")
	resolved, err := branch.Resolve("{{ .env.TEST_BRANCH }}")
	assert.NoError(t, err)
	assert.Equal(t, "branch", resolved)
}
