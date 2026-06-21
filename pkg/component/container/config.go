package container

import (
	"fmt"
	"strings"

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

// CommandArgs splits the run command string into argv. An empty command yields
// nil so the image's default ENTRYPOINT/CMD is used.
func (s *ContainerSpec) CommandArgs() []string {
	defer perf.Track(nil, "container.ContainerSpec.CommandArgs")()

	if s.Run == nil || strings.TrimSpace(s.Run.Command) == "" {
		return nil
	}
	return strings.Fields(s.Run.Command)
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
