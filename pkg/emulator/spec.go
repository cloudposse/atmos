package emulator

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	errEmulatorServicesEntriesInvalid = errors.New("all services entries must be strings")
	errEmulatorServicesTypeInvalid    = errors.New("expected string or list of strings")
)

// Spec is the resolved, per-instance emulator component configuration, parsed
// from the (merged, templated) component section. The nested `container:` block
// reuses the container kind's ContainerRunStep so emulator and container config
// stay consistent. These are first-class component sections (siblings of
// `metadata`/`env`/`composition`), NOT nested under `vars`.
type Spec struct {
	// Driver is the built-in driver kind, e.g. "floci/aws".
	Driver string
	// Cloud is the explicit target; optional, derived from the driver when empty.
	Cloud string
	// Region is the cloud region (aws/gcp/azure).
	Region string
	// Project is the GCP project.
	Project string
	// Services are the emulated services to enable (informational; may drive env).
	Services []string
	// Ephemeral, when set true, runs the emulator without persisting state: no
	// host directory is bind-mounted onto the driver's data dir, so all state is
	// lost on `down`. It is tri-state: nil (the default) means persist. The CLI
	// `--ephemeral` flag forces a throwaway instance for a single `up`.
	Ephemeral *bool
	// Container holds image/ports/mounts/env overrides for the emulator container.
	Container *schema.ContainerRunStep
}

// FromComponentSection decodes a resolved component section into a Spec, reading
// the first-class top-level keys `driver`, `cloud`, `region`, `project`,
// `services`, and the nested `container:` block.
func FromComponentSection(section map[string]any) (Spec, error) {
	defer perf.Track(nil, "emulator.FromComponentSection")()

	spec := Spec{
		Driver:  stringField(section, "driver"),
		Cloud:   stringField(section, "cloud"),
		Region:  stringField(section, "region"),
		Project: stringField(section, "project"),
	}
	servicesRaw, hasServices := section["services"]
	if err := applyServices(&spec, servicesRaw, hasServices); err != nil {
		return Spec{}, err
	}
	ephemeralRaw, hasEphemeral := section["ephemeral"]
	if err := applyEphemeral(&spec, ephemeralRaw, hasEphemeral); err != nil {
		return Spec{}, err
	}
	containerRaw, hasContainer := section["container"]
	if err := applyContainer(&spec, containerRaw, hasContainer); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func stringField(section map[string]any, key string) string {
	v, _ := section[key].(string)
	return v
}

func applyServices(spec *Spec, raw any, ok bool) error {
	if !ok {
		return nil
	}
	services, err := toStringSlice(raw)
	if err != nil {
		return fmt.Errorf("%w: invalid emulator services: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	spec.Services = services
	return nil
}

func applyEphemeral(spec *Spec, raw any, ok bool) error {
	if !ok {
		return nil
	}
	ephemeral, err := parseEphemeral(raw)
	if err != nil {
		return err
	}
	spec.Ephemeral = ephemeral
	return nil
}

func applyContainer(spec *Spec, raw any, ok bool) error {
	if !ok {
		return nil
	}
	var container schema.ContainerRunStep
	if err := decodeYAMLSection(raw, &container); err != nil {
		return fmt.Errorf("%w: decode emulator container: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	spec.Container = &container
	return nil
}

func parseEphemeral(raw any) (*bool, error) {
	v, ok := raw.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: emulator ephemeral must be a boolean", errUtils.ErrEmulatorConfigInvalid)
	}
	return &v, nil
}

// Validate checks that the spec names a registered driver and that an explicit
// cloud (if any) matches the driver's target. Abstract bases are skipped by the
// caller before Validate is reached.
func (s *Spec) Validate() error {
	defer perf.Track(nil, "emulator.Spec.Validate")()

	if s.Driver == "" {
		return fmt.Errorf("%w: emulator requires a driver", errUtils.ErrEmulatorConfigInvalid)
	}
	if _, err := s.Target(); err != nil {
		return err
	}
	// Validate the container restart/healthcheck up front so a misconfiguration is
	// a friendly Atmos error instead of an opaque docker/podman failure at `up`.
	return container.ValidateRunStep(s.Container)
}

// EffectiveHealthCheck returns the runtime health check for the emulator: the
// component's `container.healthcheck` when set, otherwise the driver's default
// (nil → no health check). It reuses the container kind's mapping so the Compose
// `test` form and disable semantics are identical across kinds.
func (s *Spec) EffectiveHealthCheck() (*container.HealthCheck, error) {
	defer perf.Track(nil, "emulator.Spec.EffectiveHealthCheck")()

	if s.Container != nil && s.Container.HealthCheck != nil {
		return container.HealthCheckFromStep(s.Container), nil
	}
	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return nil, err
	}
	return container.HealthCheckFromStep(&schema.ContainerRunStep{HealthCheck: driver.Defaults().HealthCheck}), nil
}

// EffectiveRestart returns the runtime restart policy for the emulator: the
// component's `container.restart` when set, otherwise the driver's default
// (nil → the runtime default of `no`).
func (s *Spec) EffectiveRestart() (*container.RestartPolicy, error) {
	defer perf.Track(nil, "emulator.Spec.EffectiveRestart")()

	if s.Container != nil && s.Container.Restart != nil {
		return container.RestartPolicyFromStep(s.Container), nil
	}
	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return nil, err
	}
	return container.RestartPolicyFromStep(&schema.ContainerRunStep{Restart: driver.Defaults().Restart}), nil
}

// Driver resolves the registered driver for this spec.
func (s *Spec) ResolvedDriver() (EmulatorDriver, error) {
	defer perf.Track(nil, "emulator.Spec.ResolvedDriver")()

	return ResolveDriver(s.Driver)
}

// Target returns the effective target: the explicit `cloud` when set (validated
// against the driver), otherwise the driver's own Target().
func (s *Spec) Target() (string, error) {
	defer perf.Track(nil, "emulator.Spec.Target")()

	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return "", err
	}
	if s.Cloud != "" && s.Cloud != driver.Target() {
		return "", fmt.Errorf("%w: cloud %q != %q driver target %q",
			errUtils.ErrEmulatorTargetMismatch, s.Cloud, s.Driver, driver.Target())
	}
	return driver.Target(), nil
}

// Image returns the effective container image: the explicit `container.image`
// when set, otherwise the driver's default image.
func (s *Spec) Image() (string, error) {
	defer perf.Track(nil, "emulator.Spec.Image")()

	if s.Container != nil && s.Container.Image != "" {
		return s.Container.Image, nil
	}
	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return "", err
	}
	return driver.Defaults().Image, nil
}

// ContainerPorts returns the effective container ports to publish: the explicit
// `container.ports` when set, otherwise the driver's default ports. Host ports
// are left at 0 (auto-assigned) unless explicitly pinned.
func (s *Spec) ContainerPorts() ([]schema.ContainerPort, error) {
	defer perf.Track(nil, "emulator.Spec.ContainerPorts")()

	if s.Container != nil && len(s.Container.Ports) > 0 {
		return s.Container.Ports, nil
	}
	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return nil, err
	}
	ports := make([]schema.ContainerPort, 0, len(driver.Defaults().Ports))
	for _, cp := range driver.Defaults().Ports {
		ports = append(ports, schema.ContainerPort{Container: cp})
	}
	return ports, nil
}

// RootlessConfig is the run-args/command a driver substitutes under a rootless
// runtime, and whether such an override applies.
type RootlessConfig struct {
	RunArgs []string
	Command []string
	Applies bool
}

// RootlessOverride returns the rootless run-args/command for the spec's driver when
// it defines one (e.g. k3s needs a cgroup-nesting entrypoint). Drivers without a
// rootless variant return Applies=false, so the rootful defaults are used in all
// runtimes.
func (s *Spec) RootlessOverride() (RootlessConfig, error) {
	defer perf.Track(nil, "emulator.Spec.RootlessOverride")()

	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return RootlessConfig{}, err
	}
	if overrider, isOverrider := driver.(RootlessOverrider); isOverrider {
		ra, cmd, has := overrider.RootlessOverride()
		return RootlessConfig{RunArgs: ra, Command: cmd, Applies: has}, nil
	}
	return RootlessConfig{}, nil
}

// DefaultCommand returns the driver's default container command/args (e.g. k3s
// must run `server`). It is empty for emulators that use the image's entrypoint.
func (s *Spec) DefaultCommand() ([]string, error) {
	defer perf.Track(nil, "emulator.Spec.DefaultCommand")()

	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return nil, err
	}
	return driver.Defaults().Command, nil
}

// DefaultEnv returns the driver's default container environment variables (e.g.
// k3s requires a K3S_TOKEN to start). The resolved profile/component env is
// layered over these by the manager.
func (s *Spec) DefaultEnv() (map[string]string, error) {
	defer perf.Track(nil, "emulator.Spec.DefaultEnv")()

	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return nil, err
	}
	return driver.Defaults().Env, nil
}

// Privileged reports whether the emulator container must run in privileged mode.
// This is a driver property (e.g. k3s runs a nested Kubernetes), not user config.
func (s *Spec) Privileged() (bool, error) {
	defer perf.Track(nil, "emulator.Spec.Privileged")()

	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return false, err
	}
	return driver.Defaults().Privileged, nil
}

// HostRuntime reports whether the emulator container needs access to the host
// container runtime (Docker-out-of-Docker), set via `container.runtime.host: true`.
// Used, for example, by the MiniStack driver's EKS service to spawn a k3s sibling.
func (s *Spec) HostRuntime() bool {
	defer perf.Track(nil, "emulator.Spec.HostRuntime")()

	return s.Container != nil && s.Container.Runtime != nil && s.Container.Runtime.Host
}

// PersistEnabled reports whether the emulator should persist state across
// `down`/`up`. Persistence is on by default; only an explicit `ephemeral: true`
// (or the `--ephemeral` CLI flag) disables it.
func (s *Spec) PersistEnabled() bool {
	defer perf.Track(nil, "emulator.Spec.PersistEnabled")()

	return s.Ephemeral == nil || !*s.Ephemeral
}

// DataDir returns the driver's in-container persistence path (e.g. "/app/data").
// An empty string means the driver has no persistent state, so persistence is a
// no-op for it.
func (s *Spec) DataDir() (string, error) {
	defer perf.Track(nil, "emulator.Spec.DataDir")()

	driver, err := ResolveDriver(s.Driver)
	if err != nil {
		return "", err
	}
	return driver.Defaults().DataDir, nil
}

// decodeYAMLSection decodes a YAML-derived map into a struct using its `yaml`
// tags (ContainerRunStep is yaml-tagged, so snake_case keys map correctly).
func decodeYAMLSection(raw, result any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           result,
		WeaklyTypedInput: true,
		TagName:          "yaml",
	})
	if err != nil {
		return err
	}
	return decoder.Decode(raw)
}

// toStringSlice coerces a YAML-decoded value into a []string. It accepts a
// single string or a list of strings and rejects malformed lists.
func toStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case string:
		return []string{v}, nil
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, errEmulatorServicesEntriesInvalid
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, errEmulatorServicesTypeInvalid
	}
}
