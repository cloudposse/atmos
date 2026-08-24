package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplySecretEnv(t *testing.T) {
	base := []string{"PATH=/bin", "EXISTING=old"}
	pairs := []secretEnvPair{
		{name: "EXISTING", value: "new"}, // replaces in place
		{name: "A", value: "1"},          // appended
		{name: "Z", value: "26"},         // appended
	}

	result := applySecretEnv(base, pairs)

	// A declared secret replaces an inherited variable of the same name.
	assert.Contains(t, result, "EXISTING=new")
	assert.NotContains(t, result, "EXISTING=old")
	// Inherited, non-conflicting entries are preserved.
	assert.Contains(t, result, "PATH=/bin")
	// New secrets are injected.
	assert.Contains(t, result, "A=1")
	assert.Contains(t, result, "Z=26")

	// Order: base entries keep their positions; new secrets are appended in order.
	assert.Equal(t, "PATH=/bin", result[0])
	assert.Equal(t, "Z=26", result[len(result)-1])
	assert.Len(t, result, 4)
}

func TestApplySecretEnv_NoSecrets(t *testing.T) {
	base := []string{"PATH=/bin"}
	result := applySecretEnv(base, nil)
	assert.Equal(t, base, result)
}
