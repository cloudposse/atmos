package target

import (
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestGitProfile_Branches(t *testing.T) {
	t.Run("bound port sets emulator URL", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetGit, Host: "localhost", Ports: map[int]int{3000: 33000}}
		p := GitProfile(ep)

		assert.Equal(t, "http://127.0.0.1:33000", p.Env["ATMOS_GIT_EMULATOR_URL"])
	})

	t.Run("no bound port yields empty env", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetGit, Host: "localhost", Ports: map[int]int{}}
		p := GitProfile(ep)

		// Without a live endpoint, the URL key is absent.
		assert.NotContains(t, p.Env, "ATMOS_GIT_EMULATOR_URL")
		assert.Empty(t, p.Env)
	})
}
