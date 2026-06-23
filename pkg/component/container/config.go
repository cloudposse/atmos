package container

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultBasePath is the conventional location for container component build
	// contexts and assets.
	defaultBasePath = "components/container"
	// DefaultProtocol is assumed for a port mapping with no explicit protocol.
	defaultProtocol = "tcp"
	// DefaultMountType is assumed for a mount with no explicit type.
	defaultMountType = "bind"
)

// Config is the global configuration for the container component kind, stored
// under `components.container` in atmos.yaml and read via the Plugins map.
type Config struct {
	// BasePath is the base directory for container component assets.
	BasePath string `mapstructure:"base_path"`
}

// DefaultConfig returns the default global container component configuration.
func DefaultConfig() Config {
	defer perf.Track(nil, "container.DefaultConfig")()

	return Config{BasePath: defaultBasePath}
}

// parseConfig decodes a raw global-config value (from the Plugins map) into a
// typed Config.
func parseConfig(raw any) (Config, error) {
	var config Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &config,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
	})
	if err != nil {
		return Config{}, fmt.Errorf("%w: create config decoder: %w", errUtils.ErrComponentConfigInvalid, err)
	}
	if err := decoder.Decode(raw); err != nil {
		return Config{}, fmt.Errorf("%w: decode container config: %w", errUtils.ErrComponentConfigInvalid, err)
	}
	return config, nil
}

// ContainerSpec is the resolved, per-instance container component configuration.
// Its build/run shapes reuse the workflow container-step structs so component
// and step configuration stay consistent. These are first-class component
// sections (siblings of `composition`/`env`/`metadata`), NOT nested under `vars`.
type ContainerSpec struct {
	Image       string
	Build       *schema.ContainerBuildStep
	Run         *schema.ContainerRunStep
	Composition string
}

// FromComponentSection decodes the resolved (merged, templated) component section
// into a ContainerSpec, reading the first-class top-level keys `image`, `build`,
// `run`, and `composition`. Container application env comes from the component's
// `env:` section (resolved with secrets), not from `run`.
func FromComponentSection(section map[string]any) (ContainerSpec, error) {
	defer perf.Track(nil, "container.FromComponentSection")()

	var spec ContainerSpec

	if image, ok := section["image"].(string); ok {
		spec.Image = image
	}
	if composition, ok := section["composition"].(string); ok {
		spec.Composition = composition
	}

	if raw, ok := section["build"]; ok {
		var build schema.ContainerBuildStep
		if err := decodeSection(raw, &build); err != nil {
			return ContainerSpec{}, fmt.Errorf("%w: decode container build: %w", errUtils.ErrComponentConfigInvalid, err)
		}
		spec.Build = &build
	}
	if raw, ok := section["run"]; ok {
		var run schema.ContainerRunStep
		if err := decodeSection(raw, &run); err != nil {
			return ContainerSpec{}, fmt.Errorf("%w: decode container run: %w", errUtils.ErrComponentConfigInvalid, err)
		}
		spec.Run = &run
	}

	return spec, nil
}

// decodeSection decodes a YAML-derived map into a struct using its `yaml` tags
// (the workflow container structs are tagged for yaml, so snake_case keys like
// `build_args` and `read_only` map correctly).
func decodeSection(raw any, result any) error {
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

// PushRefs returns the image references that `push` should send. When a build
// with tags is configured it returns every build tag (deduped, order-preserving)
// — so listing registry-qualified tags in `build.tags` pushes the image to
// multiple registries in one operation. Otherwise it returns the single
// top-level image (or nil when neither is set).
func (s *ContainerSpec) PushRefs() []string {
	defer perf.Track(nil, "container.ContainerSpec.PushRefs")()

	if s.Build != nil && len(s.Build.Tags) > 0 {
		seen := make(map[string]struct{}, len(s.Build.Tags))
		refs := make([]string, 0, len(s.Build.Tags))
		for _, tag := range s.Build.Tags {
			if tag == "" {
				continue
			}
			if _, dup := seen[tag]; dup {
				continue
			}
			seen[tag] = struct{}{}
			refs = append(refs, tag)
		}
		if len(refs) > 0 {
			return refs
		}
	}
	if strings.TrimSpace(s.Image) != "" {
		return []string{s.Image}
	}
	return nil
}

// ToBuildConfig maps the build spec onto a runtime BuildConfig.
func (s *ContainerSpec) ToBuildConfig() *ctr.BuildConfig {
	defer perf.Track(nil, "container.ContainerSpec.ToBuildConfig")()

	if s.Build == nil {
		return nil
	}
	return &ctr.BuildConfig{
		Dockerfile: s.Build.Dockerfile,
		Context:    s.Build.Context,
		Engine:     s.Build.Engine,
		Args:       s.Build.BuildArgs,
		Tags:       s.Build.Tags,
		Target:     s.Build.Target,
		NoCache:    s.Build.NoCache,
		Pull:       s.Build.Pull,
	}
}

// CommandArgs tokenizes the run command string into argv using shell-style
// quoting so quoted arguments (e.g. `sh -c "echo hi"`) survive intact when passed
// directly as argv (as `up` does, unlike `run` which wraps in `/bin/sh -lc`). An
// empty command yields nil so the image's default ENTRYPOINT/CMD is used. A
// malformed command (e.g. an unbalanced quote) is a config error.
func (s *ContainerSpec) CommandArgs() ([]string, error) {
	defer perf.Track(nil, "container.ContainerSpec.CommandArgs")()

	if s.Run == nil || strings.TrimSpace(s.Run.Command) == "" {
		return nil, nil
	}
	args, err := shlex.Split(s.Run.Command)
	if err != nil {
		return nil, fmt.Errorf("%w: run.command %q: %w", errUtils.ErrComponentConfigInvalid, s.Run.Command, err)
	}
	return args, nil
}

// Ports maps the structured run port specs onto runtime PortBindings.
func (s *ContainerSpec) Ports() []ctr.PortBinding {
	defer perf.Track(nil, "container.ContainerSpec.Ports")()

	if s.Run == nil {
		return nil
	}
	ports := make([]ctr.PortBinding, 0, len(s.Run.Ports))
	for _, p := range s.Run.Ports {
		protocol := p.Protocol
		if protocol == "" {
			protocol = defaultProtocol
		}
		ports = append(ports, ctr.PortBinding{
			HostPort:      p.Host,
			ContainerPort: p.Container,
			Protocol:      protocol,
		})
	}
	return ports
}

// RestartPolicy maps the run restart spec onto a runtime RestartPolicy, or nil
// when none is configured (the runtime then uses its default of `no`).
func (s *ContainerSpec) RestartPolicy() *ctr.RestartPolicy {
	defer perf.Track(nil, "container.ContainerSpec.RestartPolicy")()

	if s.Run == nil || s.Run.Restart == nil || s.Run.Restart.Policy == "" {
		return nil
	}
	return &ctr.RestartPolicy{
		Policy:     s.Run.Restart.Policy,
		MaxRetries: s.Run.Restart.MaxRetries,
	}
}

// HealthCheck maps the run healthcheck spec onto a runtime HealthCheck, resolving
// the Compose `test` form (a bare string or a list whose first element is `NONE`,
// `CMD`, or `CMD-SHELL`) into a single shell command. Returns nil when no
// healthcheck is configured (the container then inherits any image healthcheck).
func (s *ContainerSpec) HealthCheck() *ctr.HealthCheck {
	defer perf.Track(nil, "container.ContainerSpec.HealthCheck")()

	if s.Run == nil || s.Run.HealthCheck == nil {
		return nil
	}
	hc := s.Run.HealthCheck
	cmd, disable := resolveHealthTest(hc.Test)
	if hc.Disable {
		disable = true
	}
	if disable {
		return &ctr.HealthCheck{Disable: true}
	}
	return &ctr.HealthCheck{
		Cmd:           cmd,
		Interval:      hc.Interval,
		Timeout:       hc.Timeout,
		Retries:       hc.Retries,
		StartPeriod:   hc.StartPeriod,
		StartInterval: hc.StartInterval,
	}
}

// resolveHealthTest resolves a Compose `test` value into a single shell command
// for `--health-cmd`. The leading `NONE`/`CMD`/`CMD-SHELL` token is interpreted
// per Compose: `NONE` disables the check; `CMD` and `CMD-SHELL` strip the prefix;
// an unprefixed value (or bare string) is treated as `CMD-SHELL`. The CLI runs
// `--health-cmd` via the shell, so `CMD` exec-form args are joined with spaces.
func resolveHealthTest(test []string) (cmd string, disable bool) {
	if len(test) == 0 {
		return "", false
	}
	switch {
	case strings.EqualFold(test[0], "NONE"):
		return "", true
	case strings.EqualFold(test[0], "CMD"), strings.EqualFold(test[0], "CMD-SHELL"):
		return strings.Join(test[1:], " "), false
	default:
		return strings.Join(test, " "), false
	}
}

// validRestartPolicies is the set of restart policies accepted by docker/podman.
var validRestartPolicies = map[string]struct{}{
	"no":             {},
	"always":         {},
	"on-failure":     {},
	"unless-stopped": {},
}

// ValidateRun checks the run restart/healthcheck settings up front so a
// misconfiguration surfaces as a friendly Atmos error instead of an opaque
// docker/podman failure at create time. It is a no-op when `run` is unset.
func (s *ContainerSpec) ValidateRun() error {
	defer perf.Track(nil, "container.ContainerSpec.ValidateRun")()

	if s.Run == nil {
		return nil
	}
	if r := s.Run.Restart; r != nil && r.Policy != "" {
		if _, ok := validRestartPolicies[r.Policy]; !ok {
			return fmt.Errorf("%w: %q (want one of: no, always, on-failure, unless-stopped)",
				errUtils.ErrInvalidContainerRestartPolicy, r.Policy)
		}
		if r.MaxRetries < 0 {
			return fmt.Errorf("%w: max_retries must not be negative", errUtils.ErrInvalidContainerRestartPolicy)
		}
	}
	return s.validateHealthCheck()
}

// validateHealthCheck validates the healthcheck durations and retry count.
func (s *ContainerSpec) validateHealthCheck() error {
	hc := s.Run.HealthCheck
	if hc == nil {
		return nil
	}
	if hc.Retries < 0 {
		return fmt.Errorf("%w: retries must not be negative", errUtils.ErrInvalidContainerHealthCheck)
	}
	durations := map[string]string{
		"interval":       hc.Interval,
		"timeout":        hc.Timeout,
		"start_period":   hc.StartPeriod,
		"start_interval": hc.StartInterval,
	}
	for field, value := range durations {
		if value == "" {
			continue
		}
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%w: %s %q is not a valid duration (e.g. 30s, 1m30s): %w",
				errUtils.ErrInvalidContainerHealthCheck, field, value, err)
		}
	}
	return nil
}

// Mounts maps the run mount specs onto runtime Mounts.
func (s *ContainerSpec) Mounts() []ctr.Mount {
	defer perf.Track(nil, "container.ContainerSpec.Mounts")()

	if s.Run == nil {
		return nil
	}
	mounts := make([]ctr.Mount, 0, len(s.Run.Mounts))
	for _, m := range s.Run.Mounts {
		mountType := m.Type
		if mountType == "" {
			mountType = defaultMountType
		}
		mounts = append(mounts, ctr.Mount{
			Type:     mountType,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	return mounts
}
