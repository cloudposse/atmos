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

// flociHealthCheck prefers the image's own readiness script when available (the
// strongest signal, since it's the vendor's own readiness check), falling back to
// a bare TCP connect via Bash — present in every Floci image, unlike curl — for
// images without the script. The TCP fallback works for both HTTP and TLS
// listeners without depending on an endpoint's response or certificate trust. A
// failing native probe must remain a failure, without falling back to TCP.
func flociHealthCheck(port int) *schema.ContainerHealthCheck {
	return shellHealthCheck(fmt.Sprintf(
		"if [ -x /usr/local/bin/healthcheck.sh ]; then exec /usr/local/bin/healthcheck.sh; fi; "+
			"bash -c 'exec 3<>/dev/tcp/127.0.0.1/%d'",
		port,
	))
}

func init() {
	emu.RegisterDriver(&builtinDriver{name: "floci/aws", target: emu.TargetAWS, image: flociAWSImage, ports: []int{flociAWSPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociAWSPort), restart: defaultEmulatorRestart, profile: target.AWSProfile})
	emu.RegisterDriver(&builtinDriver{name: "floci/gcp", target: emu.TargetGCP, image: flociGCPImage, ports: []int{flociGCPPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociGCPPort), restart: defaultEmulatorRestart, profile: target.GCPProfile})
	emu.RegisterDriver(&builtinDriver{name: "floci/az", target: emu.TargetAzure, image: flociAzImage, ports: []int{flociAzPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociAzPort), restart: defaultEmulatorRestart, profile: target.AzureProfile})
}
