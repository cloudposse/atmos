// Package driver holds the built-in emulator drivers (floci, k3s, openbao, …).
// Each driver supplies container defaults and a connection-profile builder (from
// pkg/emulator/target) and registers itself into the pkg/emulator registry at init.
// Importing this package is what populates that registry.
package driver

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// builtinDriver is the shared implementation for built-in emulator drivers: it
// carries a name, target, default image/ports, and a profile builder. The
// per-product files register concrete instances (floci/aws, openbao, …).
type builtinDriver struct {
	name       string
	target     string
	image      string
	ports      []int
	privileged bool
	env        map[string]string
	command    []string
	// dataDir is the in-container path the emulator persists state to (e.g.
	// "/data"). Empty => the driver has no persistent state (persistence no-op).
	dataDir string
	// rootlessRunArgs/rootlessCommand override run-args and command under a
	// rootless runtime (empty rootlessCommand → no override).
	rootlessRunArgs []string
	rootlessCommand []string
	// healthCheck/restart are the driver's default container health check and
	// restart policy (nil → none). The component's `container.healthcheck` /
	// `container.restart` override them.
	healthCheck *schema.ContainerHealthCheck
	restart     *schema.ContainerRestart
	profile     func(ep *emu.Endpoint) emu.Profile
}

// defaultEmulatorRestart is the default restart policy shared by the built-in
// emulator drivers. Emulators are long-lived local services, so they should come
// back after a daemon restart or a crash; `down` still stops and removes them.
var defaultEmulatorRestart = &schema.ContainerRestart{Policy: "unless-stopped"}

// defaultHealthCheckRetries is the number of consecutive probe failures (after the
// start period) before a driver's default health check marks the emulator unhealthy.
const defaultHealthCheckRetries = 5

// shellHealthCheck builds a driver default health check that runs cmd via the
// container shell (the Compose CMD-SHELL form), with timings tuned for a local
// emulator startup. The component's `container.healthcheck` overrides it.
func shellHealthCheck(cmd string) *schema.ContainerHealthCheck {
	return &schema.ContainerHealthCheck{
		Test:        []string{"CMD-SHELL", cmd},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     defaultHealthCheckRetries,
		StartPeriod: "10s",
	}
}

func (d *builtinDriver) Name() string {
	defer perf.Track(nil, "emulator.driver.builtinDriver.Name")()

	return d.name
}

func (d *builtinDriver) Target() string {
	defer perf.Track(nil, "emulator.driver.builtinDriver.Target")()

	return d.target
}

func (d *builtinDriver) Defaults() emu.ContainerDefaults {
	defer perf.Track(nil, "emulator.driver.builtinDriver.Defaults")()

	return emu.ContainerDefaults{Image: d.image, Ports: d.ports, Privileged: d.privileged, Env: d.env, Command: d.command, DataDir: d.dataDir, HealthCheck: d.healthCheck, Restart: d.restart}
}

func (d *builtinDriver) Profile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.driver.builtinDriver.Profile")()

	return d.profile(ep)
}

// RootlessOverride implements emu.RootlessOverrider. It returns the driver's
// rootless run-args/command when one is configured (e.g. k3s), otherwise ok=false.
func (d *builtinDriver) RootlessOverride() (runArgs, command []string, ok bool) {
	defer perf.Track(nil, "emulator.driver.builtinDriver.RootlessOverride")()

	if len(d.rootlessCommand) == 0 {
		return nil, nil, false
	}
	return d.rootlessRunArgs, d.rootlessCommand, true
}
