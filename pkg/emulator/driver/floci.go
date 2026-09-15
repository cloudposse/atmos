package driver

import (
	"fmt"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Floci is a free, MIT-licensed cloud-API emulator and a drop-in replacement for
// LocalStack Community Edition (which was EOL'd / paywalled in March 2026). Driver
// names follow the `<product>/<cloud>` convention: floci/aws (the default for
// `cloud: aws`), floci/gcp, and floci/az.
const (
	flociAWSImage = "floci/floci:latest"
	flociAWSPort  = 4566
	flociGCPImage = "floci/floci-gcp:latest"
	flociGCPPort  = 4588
	flociAzImage  = "floci/floci-az:latest"
	flociAzPort   = 4577

	// The flociDataDir is where every Floci variant persists state inside the
	// container (the image's FLOCI*_STORAGE_PERSISTENT_PATH and declared volume).
	flociDataDir = "/app/data"
)

// flociHealthCheck probes the Floci edge port with a bash `/dev/tcp` connect
// test: once the listener accepts a connection the emulator is reachable for
// the SDKs and Terraform, regardless of what it returns. This intentionally
// does NOT use curl/wget -- floci-gcp and floci-az are GraalVM native-image
// builds with no HTTP client binary at all (confirmed by exec'ing into a
// local container: `command -v curl` finds nothing, only a bare
// coreutils+bash userland), which made every floci/gcp and floci/az health
// check fail with "curl: command not found" regardless of how long the
// start_period was. `/bin/sh` in these images happens to be a symlink to
// bash, but CMD-SHELL health checks are documented to always run via
// `/bin/sh -c`, so this invokes bash explicitly rather than relying on that
// symlink; floci/aws's image does still ship curl, but is probed the same
// way here for consistency across the family and so a future floci/aws image
// change can't silently reintroduce this failure mode.
func flociHealthCheck(port int) *schema.ContainerHealthCheck {
	return shellHealthCheck(fmt.Sprintf("bash -c '(echo > /dev/tcp/127.0.0.1/%d)' || exit 1", port))
}

func init() {
	emu.RegisterDriver(&builtinDriver{name: "floci/aws", target: emu.TargetAWS, image: flociAWSImage, ports: []int{flociAWSPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociAWSPort), restart: defaultEmulatorRestart, profile: target.AWSProfile})
	emu.RegisterDriver(&builtinDriver{name: "floci/gcp", target: emu.TargetGCP, image: flociGCPImage, ports: []int{flociGCPPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociGCPPort), restart: defaultEmulatorRestart, profile: target.GCPProfile})
	emu.RegisterDriver(&builtinDriver{name: "floci/az", target: emu.TargetAzure, image: flociAzImage, ports: []int{flociAzPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociAzPort), restart: defaultEmulatorRestart, profile: target.AzureProfile})
}
