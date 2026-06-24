// Package emulator provides first-class support for cloud-API emulators —
// long-running container services (Floci, MiniStack, k3s, OpenBao, …) that stand
// in for AWS, GCP, Azure, Kubernetes, and select backing services during local
// development and testing.
//
// A driver is a built-in emulator product: it supplies container defaults
// (image, ports, services) and turns a live container endpoint into a connection
// Profile (env vars, kubeconfig, Terraform provider fragment).
//
// This core package is driver-agnostic: the concrete drivers live in the
// pkg/emulator/driver subpackage (and their profile builders in pkg/emulator/target),
// registering into this package's registry via RegisterDriver at init. Importing
// pkg/emulator/driver is what populates the registry.
package emulator

import (
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Emulator targets — what an emulator emulates. Derived from a driver's Target().
const (
	TargetAWS        = "aws"
	TargetGCP        = "gcp"
	TargetAzure      = "azure"
	TargetKubernetes = "kubernetes"
	TargetVault      = "vault"
	TargetRegistry   = "registry"
)

// ContainerDefaults are the built-in container defaults a driver supplies. Every
// field is overridable in the component's `container:` block (the hooks model).
type ContainerDefaults struct {
	// Image is the default container image, e.g. "floci/floci:latest".
	Image string
	// Ports are the default container ports the emulator listens on.
	Ports []int
	// Services are the default emulated services (informational; may drive env).
	Services []string
	// Env are default container environment variables.
	Env map[string]string
	// Privileged runs the container in privileged mode. It is a driver property,
	// not user config: some emulators (k3s, which runs a nested Kubernetes) only
	// function privileged.
	Privileged bool
	// Command is the default container command/args (e.g. k3s must run `server`).
	Command []string
	// DataDir is the in-container path where the emulator persists its state
	// (e.g. "/data" for floci, "/var/lib/registry" for the registry). When
	// persistence is enabled (the default), the manager bind-mounts a host
	// directory under the XDG cache onto this path so state survives `down`/`up`.
	// Empty means the driver has no persistent state, so persistence is a no-op.
	DataDir string
	// HealthCheck is the driver's default container health check, so the emulator
	// is readiness-aware out of the box (and `up` can gate on it). The component's
	// `container.healthcheck` overrides it; nil means the driver ships no default
	// (e.g. vault, whose readiness is handled by its bootstrap). The shape mirrors
	// the container kind's healthcheck, reusing schema.ContainerHealthCheck.
	HealthCheck *schema.ContainerHealthCheck
	// Restart is the driver's default container restart policy. The component's
	// `container.restart` overrides it; nil means no default (runtime default `no`).
	Restart *schema.ContainerRestart
}

// RootlessOverrider is an optional driver interface for emulators whose container
// must run differently under a rootless runtime. For example, k3s needs a
// cgroup-nesting entrypoint and the kubelet `KubeletInUserNamespace` feature flag
// that rootful runtimes don't require. The manager calls RootlessOverride only when
// it detects a rootless runtime; otherwise the driver's default (rootful) command
// runs. Drivers without a rootless variant simply don't implement it.
type RootlessOverrider interface {
	// RootlessOverride returns the run-args (e.g. an entrypoint override) and the
	// container command to use under a rootless runtime, and whether such an
	// override exists (false → the rootful defaults are used in all runtimes).
	RootlessOverride() (runArgs, command []string, ok bool)
}

// EmulatorDriver is a built-in emulator product (floci, k3s, openbao, …).
type EmulatorDriver interface {
	// Name is the driver identifier used as `driver:` in component config.
	Name() string
	// Target is what the driver emulates (aws|gcp|azure|kubernetes|vault|registry).
	Target() string
	// Defaults are the built-in container defaults (overridable in `container:`).
	Defaults() ContainerDefaults
	// Profile turns a live endpoint into a connection profile.
	Profile(ep *Endpoint) Profile
}

// emulatorDrivers is the package-level driver registry, populated via RegisterDriver
// by the pkg/emulator/driver subpackage at init (mirrors pkg/store/registry.go).
var emulatorDrivers = map[string]EmulatorDriver{}

// RegisterDriver adds a built-in driver to the registry. Called from the driver
// subpackage's init(). Last registration wins, allowing override in tests.
func RegisterDriver(d EmulatorDriver) {
	defer perf.Track(nil, "emulator.RegisterDriver")()

	emulatorDrivers[d.Name()] = d
}

// ResolveDriver returns the built-in driver registered for the given name.
func ResolveDriver(name string) (EmulatorDriver, error) {
	defer perf.Track(nil, "emulator.ResolveDriver")()

	d, ok := emulatorDrivers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q (available: %v)", errUtils.ErrUnknownEmulatorDriver, name, Drivers())
	}
	return d, nil
}

// Drivers returns the registered driver names, sorted.
func Drivers() []string {
	defer perf.Track(nil, "emulator.Drivers")()

	names := make([]string, 0, len(emulatorDrivers))
	for name := range emulatorDrivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
