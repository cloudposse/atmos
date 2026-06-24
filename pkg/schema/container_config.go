package schema

// ContainerConfig is the top-level `container:` namespace for global container
// behavior. It is the forward-compatible home for settings shared by the
// `type: container` workflow steps and the upcoming `container` component kind /
// `atmos container` command.
type ContainerConfig struct {
	Runtime ContainerRuntimeConfig `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
}

// ContainerRuntimeConfig configures the container runtime provider (docker/podman).
type ContainerRuntimeConfig struct {
	// Provider selects the container runtime: "docker" | "podman" | "" (auto-detect:
	// docker, then podman). A global default for the per-step `provider:` field.
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	// AutoStart lets Atmos auto-init/start the Podman machine when no running runtime
	// is found, instead of failing. A global default for the per-step
	// `runtime_auto_start:` field; also settable via ATMOS_CONTAINER_RUNTIME_AUTO_START.
	AutoStart bool `yaml:"auto_start,omitempty" json:"auto_start,omitempty" mapstructure:"auto_start"`
}
