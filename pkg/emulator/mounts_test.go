package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestMountsTargetPath_NormalizesTargets(t *testing.T) {
	mounts := []container.Mount{{Target: "/var/lib/persist/"}}
	assert.True(t, mountsTargetPath(mounts, "/var/lib/persist"))
}
