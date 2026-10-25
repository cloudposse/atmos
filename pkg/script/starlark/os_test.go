package starlark

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestOSPlatformAndArchitecture(t *testing.T) {
	t.Parallel()
	result, err := New().Execute(t.Context(), script.Spec{
		Source:     `output = [os.platform, os.arch]`,
		ProcessEnv: []string{"GOOS=other", "GOARCH=other"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`[%q,%q]`, runtime.GOOS, runtime.GOARCH), result.Value)
}

func TestOSPlatformAndArchitectureReadOnly(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"platform", "arch"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, "os."+name+` = "other"`)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "assign to .")
		})
	}
}
