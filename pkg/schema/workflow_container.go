package schema

import (
	"encoding/json"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ErrInvalidWorkflowContainer is returned when a workflow `container` value cannot be decoded.
var ErrInvalidWorkflowContainer = errors.New("invalid workflow container configuration")

// ErrInvalidContainerDriver is returned when a container build step's `driver` value cannot be decoded.
var ErrInvalidContainerDriver = errors.New("invalid container build driver configuration")

// ContainerMount represents a volume mount for container steps.
type ContainerMount struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"` // bind, volume, tmpfs.
	Source   string `yaml:"source,omitempty" json:"source,omitempty" mapstructure:"source"`
	Target   string `yaml:"target,omitempty" json:"target,omitempty" mapstructure:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty" json:"read_only,omitempty" mapstructure:"read_only"`
}

// ContainerPort represents a port mapping for container steps.
type ContainerPort struct {
	// Host is a port number or an IP:port string (IPv6 addresses require brackets).
	Host      any    `yaml:"host,omitempty" json:"host,omitempty" mapstructure:"host" jsonschema:"oneof_type=integer;string;null"`
	Container int    `yaml:"container,omitempty" json:"container,omitempty" mapstructure:"container"`
	Protocol  string `yaml:"protocol,omitempty" json:"protocol,omitempty" mapstructure:"protocol"`
}

// ContainerBuildBakeStep configures a Docker Buildx Bake build action.
type ContainerBuildBakeStep struct {
	File    string            `yaml:"file,omitempty" json:"file,omitempty" mapstructure:"file"`
	Files   []string          `yaml:"files,omitempty" json:"files,omitempty" mapstructure:"files"`
	Target  string            `yaml:"target,omitempty" json:"target,omitempty" mapstructure:"target"`
	Targets []string          `yaml:"targets,omitempty" json:"targets,omitempty" mapstructure:"targets"`
	Set     []string          `yaml:"set,omitempty" json:"set,omitempty" mapstructure:"set"`
	Vars    map[string]string `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars"`
	Load    bool              `yaml:"load,omitempty" json:"load,omitempty" mapstructure:"load"`
	Push    bool              `yaml:"push,omitempty" json:"push,omitempty" mapstructure:"push"`
	Print   bool              `yaml:"print,omitempty" json:"print,omitempty" mapstructure:"print"`
}

// ContainerBuildStep configures a container image build action.
type ContainerBuildStep struct {
	Provider         string                  `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool                    `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Engine           string                  `yaml:"engine,omitempty" json:"engine,omitempty" mapstructure:"engine"`
	Context          string                  `yaml:"context,omitempty" json:"context,omitempty" mapstructure:"context"`
	Dockerfile       string                  `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty" mapstructure:"dockerfile"`
	Tags             []string                `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"`
	BuildArgs        map[string]string       `yaml:"build_args,omitempty" json:"build_args,omitempty" mapstructure:"build_args"`
	Target           string                  `yaml:"target,omitempty" json:"target,omitempty" mapstructure:"target"`
	NoCache          bool                    `yaml:"no_cache,omitempty" json:"no_cache,omitempty" mapstructure:"no_cache"`
	Pull             bool                    `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`
	Load             bool                    `yaml:"load,omitempty" json:"load,omitempty" mapstructure:"load"`
	Bake             *ContainerBuildBakeStep `yaml:"bake,omitempty" json:"bake,omitempty" mapstructure:"bake"`
	Driver           *ContainerDriverConfig  `yaml:"driver,omitempty" json:"driver,omitempty" mapstructure:"driver"`
	Cache            *ContainerCacheConfig   `yaml:"cache,omitempty" json:"cache,omitempty" mapstructure:"cache"`
}

// ContainerDriverConfig configures the Buildx builder instance used for build/bake.
// A bare string value sets Provider with no Opts (see UnmarshalYAML), e.g. `driver: docker-container`.
type ContainerDriverConfig struct {
	Name     string            `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Provider string            `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	Opts     map[string]string `yaml:"opts,omitempty" json:"opts,omitempty" mapstructure:"opts"`
}

// UnmarshalYAML supports both object syntax and a bare provider string, e.g. `driver: docker-container`.
func (d *ContainerDriverConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var provider string
		if err := value.Decode(&provider); err != nil {
			return fmt.Errorf("%w: driver must be a mapping or string: %w", ErrInvalidContainerDriver, err)
		}
		d.Provider = provider
		return nil
	case yaml.MappingNode:
		type containerDriverConfig ContainerDriverConfig
		var decoded containerDriverConfig
		if err := decodeYAMLKnownFields(value, &decoded); err != nil {
			return fmt.Errorf("%w: driver must be a mapping or string: %w", ErrInvalidContainerDriver, err)
		}
		*d = ContainerDriverConfig(decoded)
		return nil
	default:
		return fmt.Errorf("%w: driver must be a mapping or string, got YAML node kind %d", ErrInvalidContainerDriver, value.Kind)
	}
}

// ContainerCacheConfig configures Buildx cache import/export sources for a build.
// Each entry is a raw Buildx cache attribute set (e.g. type, ref, mode, image-manifest, oci-mediatypes).
type ContainerCacheConfig struct {
	From []map[string]string `yaml:"from,omitempty" json:"from,omitempty" mapstructure:"from"`
	To   []map[string]string `yaml:"to,omitempty" json:"to,omitempty" mapstructure:"to"`
}

// ContainerPushStep configures a container image push action.
type ContainerPushStep struct {
	Provider         string   `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool     `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Image            string   `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
	Tags             []string `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"`
}

// ContainerInspectStep configures a container image inspect action that renders
// curated image metadata.
type ContainerInspectStep struct {
	Provider         string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool   `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Image            string `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
}

// ContainerRestart configures the runtime restart policy for a long-lived
// container component. It maps to the docker/podman `--restart` flag
// (`<policy>[:<max_retries>]`). MaxRetries is only meaningful for `on-failure`.
type ContainerRestart struct {
	Policy     string `yaml:"policy,omitempty" json:"policy,omitempty" mapstructure:"policy"`                // no, always, on-failure, unless-stopped.
	MaxRetries int    `yaml:"max_retries,omitempty" json:"max_retries,omitempty" mapstructure:"max_retries"` // on-failure only.
}

// ContainerHealthCheck configures a container health check, mirroring the Docker
// Compose `healthcheck` shape. It maps to the docker/podman `--health-*` flags
// (or `--no-healthcheck` when disabled). Test may be a bare string or a list whose
// first element is `NONE`, `CMD`, or `CMD-SHELL`; a bare string / unprefixed list
// is treated as `CMD-SHELL`. Duration fields accept Go duration strings (e.g. `30s`).
type ContainerHealthCheck struct {
	Test          []string `yaml:"test,omitempty" json:"test,omitempty" mapstructure:"test"`                               // string or [NONE|CMD|CMD-SHELL, ...].
	Interval      string   `yaml:"interval,omitempty" json:"interval,omitempty" mapstructure:"interval"`                   // e.g. 30s.
	Timeout       string   `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`                      // e.g. 10s.
	Retries       int      `yaml:"retries,omitempty" json:"retries,omitempty" mapstructure:"retries"`                      // consecutive failures before unhealthy.
	StartPeriod   string   `yaml:"start_period,omitempty" json:"start_period,omitempty" mapstructure:"start_period"`       // grace period before failures count.
	StartInterval string   `yaml:"start_interval,omitempty" json:"start_interval,omitempty" mapstructure:"start_interval"` // probe interval during the start period.
	Disable       bool     `yaml:"disable,omitempty" json:"disable,omitempty" mapstructure:"disable"`                      // disable any image healthcheck.
}

// ContainerRunStep configures a one-shot container run action.
type ContainerRunStep struct {
	Image            string `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
	Command          string `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	Shell            string `yaml:"shell,omitempty" json:"shell,omitempty" mapstructure:"shell"`
	Provider         string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool   `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	// Runtime is the nested per-instance runtime block (e.g. `runtime.host: true` for
	// Docker-out-of-Docker). Mirrors the global `container.runtime` config.
	Runtime           *ContainerRuntimeConfig `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
	Pull              string                  `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`
	Workspace         string                  `yaml:"workspace,omitempty" json:"workspace,omitempty" mapstructure:"workspace"`
	WorkspaceReadOnly bool                    `yaml:"workspace_read_only,omitempty" json:"workspace_read_only,omitempty" mapstructure:"workspace_read_only"`
	Cleanup           string                  `yaml:"cleanup,omitempty" json:"cleanup,omitempty" mapstructure:"cleanup"`
	User              string                  `yaml:"user,omitempty" json:"user,omitempty" mapstructure:"user"`
	RunArgs           []string                `yaml:"run_args,omitempty" json:"run_args,omitempty" mapstructure:"run_args"`
	Mounts            []ContainerMount        `yaml:"mounts,omitempty" json:"mounts,omitempty" mapstructure:"mounts"`
	Ports             []ContainerPort         `yaml:"ports,omitempty" json:"ports,omitempty" mapstructure:"ports"`
	Env               map[string]string       `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
	Restart           *ContainerRestart       `yaml:"restart,omitempty" json:"restart,omitempty" mapstructure:"restart"`
	HealthCheck       *ContainerHealthCheck   `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty" mapstructure:"healthcheck"`
}

// WorkflowContainer configures a workflow-level container-backed sandbox or a
// step-level container override. A YAML value of `false` disables inheritance.
type WorkflowContainer struct {
	Enabled           *bool             `yaml:"-" json:"-" mapstructure:"-"`
	Image             string            `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
	Shell             string            `yaml:"shell,omitempty" json:"shell,omitempty" mapstructure:"shell"`
	Provider          string            `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart  bool              `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Pull              string            `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`
	Workspace         string            `yaml:"workspace,omitempty" json:"workspace,omitempty" mapstructure:"workspace"`
	WorkspaceReadOnly bool              `yaml:"workspace_read_only,omitempty" json:"workspace_read_only,omitempty" mapstructure:"workspace_read_only"`
	Cleanup           string            `yaml:"cleanup,omitempty" json:"cleanup,omitempty" mapstructure:"cleanup"`
	User              string            `yaml:"user,omitempty" json:"user,omitempty" mapstructure:"user"`
	RunArgs           []string          `yaml:"run_args,omitempty" json:"run_args,omitempty" mapstructure:"run_args"`
	Mounts            []ContainerMount  `yaml:"mounts,omitempty" json:"mounts,omitempty" mapstructure:"mounts"`
	Ports             []ContainerPort   `yaml:"ports,omitempty" json:"ports,omitempty" mapstructure:"ports"`
	Env               map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
}

// UnmarshalYAML supports both object syntax and `container: false`.
func (c *WorkflowContainer) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var enabled bool
		if err := value.Decode(&enabled); err != nil {
			return fmt.Errorf("%w: container must be a mapping or boolean: %w", ErrInvalidWorkflowContainer, err)
		}
		c.Enabled = &enabled
		return nil
	case yaml.MappingNode:
		type workflowContainer WorkflowContainer
		var decoded workflowContainer
		if err := decodeYAMLKnownFields(value, &decoded); err != nil {
			return fmt.Errorf("%w: container must be a mapping or boolean: %w", ErrInvalidWorkflowContainer, err)
		}
		*c = WorkflowContainer(decoded)
		return nil
	default:
		return fmt.Errorf("%w: container must be a mapping or boolean, got YAML node kind %d", ErrInvalidWorkflowContainer, value.Kind)
	}
}

// workflowContainerJSON mirrors WorkflowContainer's fields for JSON
// marshaling, adding an explicit "enabled" key. WorkflowContainer's own
// struct tag hides Enabled from JSON (`json:"-"`) because it's normally
// populated only by UnmarshalYAML's polymorphic bool-or-mapping decode, not
// by generic reflection-based decoding. That's fine for the YAML config-load
// path, but it means a generic JSON-based deep copy -- e.g.
// cmd/cmd_utils.go's cloneCommand, which round-trips a schema.Command
// (including any step's Container) through json.Marshal/json.Unmarshal to
// give each custom command's Cobra closure an independent copy -- silently
// dropped a step's `container: false` opt-out: Enabled came back nil, which
// IsEnabled() treats as enabled, inverting the opt-out. MarshalJSON/
// UnmarshalJSON below make WorkflowContainer round-trip through JSON
// losslessly, the same way UnmarshalYAML already does for YAML.
type workflowContainerJSON struct {
	Enabled           *bool             `json:"enabled,omitempty"`
	Image             string            `json:"image,omitempty"`
	Shell             string            `json:"shell,omitempty"`
	Provider          string            `json:"provider,omitempty"`
	RuntimeAutoStart  bool              `json:"runtime_auto_start,omitempty"`
	Pull              string            `json:"pull,omitempty"`
	Workspace         string            `json:"workspace,omitempty"`
	WorkspaceReadOnly bool              `json:"workspace_read_only,omitempty"`
	Cleanup           string            `json:"cleanup,omitempty"`
	User              string            `json:"user,omitempty"`
	RunArgs           []string          `json:"run_args,omitempty"`
	Mounts            []ContainerMount  `json:"mounts,omitempty"`
	Ports             []ContainerPort   `json:"ports,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
}

// MarshalJSON serializes Enabled alongside the rest of the fields; see
// workflowContainerJSON for why this is needed.
func (c *WorkflowContainer) MarshalJSON() ([]byte, error) {
	return json.Marshal(workflowContainerJSON(*c))
}

// UnmarshalJSON is the counterpart to MarshalJSON; see workflowContainerJSON
// for why this is needed.
func (c *WorkflowContainer) UnmarshalJSON(data []byte) error {
	var decoded workflowContainerJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWorkflowContainer, err)
	}
	*c = WorkflowContainer(decoded)
	return nil
}

// IsEnabled reports whether the container config should be applied.
func (c *WorkflowContainer) IsEnabled() bool {
	return c != nil && (c.Enabled == nil || *c.Enabled)
}
