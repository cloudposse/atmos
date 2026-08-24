package driver

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
)

// MiniStack and LocalStack are AWS emulators on the same 4566 edge port. MiniStack
// (MIT) is the free LocalStack-Community replacement; LocalStack is opt-in/legacy
// (its community edition was paywalled / BSL-relicensed in March 2026). All AWS
// emulators share the AWSProfile builder.
const (
	ministackImage  = "ministackorg/ministack:latest"
	localstackImage = "localstack/localstack:3"
	awsEdgePort     = 4566

	// The localstackDataDir is LocalStack's persistence root inside the container
	// (its LOCALSTACK_VOLUME_DIR / declared volume). MiniStack's persistent path
	// has not been confirmed against its image, so it leaves DataDir empty for now
	// (no persistence) rather than bind-mounting onto a wrong path.
	localstackDataDir = "/var/lib/localstack"
)

func init() {
	// No default health check: these opt-in/legacy images have not been verified
	// for an in-image probe tool, so a forced probe could brick `up`. Users can set
	// `container.healthcheck` explicitly.
	emu.RegisterDriver(&builtinDriver{name: "ministack/aws", target: emu.TargetAWS, image: ministackImage, ports: []int{awsEdgePort}, restart: defaultEmulatorRestart, profile: target.AWSProfile})
	emu.RegisterDriver(&builtinDriver{name: "localstack/aws", target: emu.TargetAWS, image: localstackImage, ports: []int{awsEdgePort}, dataDir: localstackDataDir, restart: defaultEmulatorRestart, profile: target.AWSProfile})
}
