package driver

import (
	"fmt"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
)

// registry is a local OCI / Terraform registry (the standard `registry:2` image)
// for vendoring and the terraform-registry-cache, on the conventional 5000 port.
const (
	registryImage = "registry:2"
	registryPort  = 5000
	// The registryDataDir is where registry:2 stores image blobs and metadata
	// inside the container (its declared volume / default filesystem storage root).
	registryDataDir = "/var/lib/registry"
)

func init() {
	// The registry's `/v2/` API root returns 200 once it is serving; busybox wget
	// (present in the registry:2 image) makes a dependency-free readiness probe.
	healthCheck := shellHealthCheck(fmt.Sprintf("wget -q -O /dev/null http://localhost:%d/v2/ || exit 1", registryPort))
	emu.RegisterDriver(&builtinDriver{name: "registry", target: emu.TargetRegistry, image: registryImage, ports: []int{registryPort}, dataDir: registryDataDir, healthCheck: healthCheck, restart: defaultEmulatorRestart, profile: target.RegistryProfile})
}
