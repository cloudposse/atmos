//go:build mage

package build

import (
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBinaryCachePolicy(t *testing.T) {
	for _, value := range []string{"", "false", "true"} {
		t.Run("no-cache="+value, func(t *testing.T) {
			root := testutil.InitGitRepoFixture(t)
			argsFile := testutil.SetUpFakePathBinary(t, "go")
			t.Chdir(root)
			t.Setenv("ATMOS_BUILD_NO_CACHE", value)
			t.Setenv("GOFLAGS", "-trimpath")
			t.Setenv("GOCACHE", t.TempDir())

			require.NoError(t, Build{}.Binary("default", "test"))
			args := testutil.ReadFakeBinArgs(t, argsFile)
			require.NotEmpty(t, args)
			assert.Equal(t, "build", args[0])
			if value == "true" {
				assert.Contains(t, args, "-a")
			} else {
				assert.NotContains(t, args, "-a")
			}
			assert.Equal(t, "-trimpath", testutil.ReadFakeBinEnv(t, argsFile)["GOFLAGS"])
		})
	}
}
