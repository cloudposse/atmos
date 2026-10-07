package emulator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMountsTargetPath_NormalizesTargets(t *testing.T) {
	mounts := []container.Mount{{Target: "/var/lib/persist/"}}
	assert.True(t, mountsTargetPath(mounts, "/var/lib/persist"))
}

func TestConvertEmulatorMounts_ExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	mounts := convertEmulatorMounts([]schema.ContainerMount{{Source: "~/data", Target: "/c"}, {Source: "rel", Target: "/d"}})
	require.Len(t, mounts, 2)
	assert.Equal(t, filepath.Join(home, "data"), mounts[0].Source)
	assert.Equal(t, "rel", mounts[1].Source)
}
