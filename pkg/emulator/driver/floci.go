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

// flociHealthCheck probes the Floci edge port with curl (present in every Floci
// image): once the listener accepts a connection the emulator is reachable for
// the SDKs and Terraform. `-s` (not `-f`) means any HTTP response — including a
// 404 on `/` — counts as up; only a refused connection fails the probe.
func flociHealthCheck(port int, startPeriod string) *schema.ContainerHealthCheck {
	return shellHealthCheckWithStartPeriod(fmt.Sprintf("curl -s -o /dev/null http://localhost:%d/ || exit 1", port), startPeriod)
}

// flociGCPAzStartPeriod gives floci/gcp and floci/az a longer health check
// start_period than floci/aws's 10s default. Both images are consistently
// slower to start listening under CI load -- floci-gcp is a Quarkus JVM with a
// warmup cost, floci-az additionally generates a TLS cert on first boot when
// FLOCI_AZ_TLS_ENABLED is set -- and the 10s default (60s total budget: 10s +
// 5 retries * 10s interval) was observed flipping both to a terminal
// "unhealthy" state around 54-56s in back-to-back CI runs, right after the
// heavy AWS scaffold tests loaded up the Docker daemon. 40s brings the total
// budget to ~90s, matching the tolerance docs/fixes/2026-08-31-floci-azure-health-check-race.md
// already established works for the same two images at a different readiness
// gate (tests/floci_harness_test.go's requireFlociEndpoint).
const flociGCPAzStartPeriod = "40s"

func init() {
	emu.RegisterDriver(&builtinDriver{name: "floci/aws", target: emu.TargetAWS, image: flociAWSImage, ports: []int{flociAWSPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociAWSPort, "10s"), restart: defaultEmulatorRestart, profile: target.AWSProfile})
	emu.RegisterDriver(&builtinDriver{name: "floci/gcp", target: emu.TargetGCP, image: flociGCPImage, ports: []int{flociGCPPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociGCPPort, flociGCPAzStartPeriod), restart: defaultEmulatorRestart, profile: target.GCPProfile})
	emu.RegisterDriver(&builtinDriver{name: "floci/az", target: emu.TargetAzure, image: flociAzImage, ports: []int{flociAzPort}, dataDir: flociDataDir, healthCheck: flociHealthCheck(flociAzPort, flociGCPAzStartPeriod), restart: defaultEmulatorRestart, profile: target.AzureProfile})
}
