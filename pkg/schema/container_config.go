package schema

// ContainerConfig is the top-level `container:` namespace for global container
// behavior. It is the forward-compatible home for settings shared by the
// `type: container` workflow steps and the upcoming `container` component kind /
// `atmos container` command.
type ContainerConfig struct {
	Runtime ContainerRuntimeConfig `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
}

// Composition declares a named multi-service system. Components join a
// composition via their first-class `composition:` field. The `Services` list is
// a closed contract for membership (declaring membership in an unlisted service
// is an error) but open for fulfillment (a declared service with no component in
// a given stack is allowed).
type Composition struct {
	// Description is a human-readable summary of the composition.
	Description string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	// Services is the closed set of service names that may claim membership.
	Services []string `yaml:"services,omitempty" json:"services,omitempty" mapstructure:"services"`
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
	// Host grants the container access to the host container runtime (Docker-out-of-Docker):
	// Atmos mounts the runtime socket, runs the container as root, relabels it for SELinux,
	// and sets DOCKER_HOST so the container can launch and manage sibling containers. Opt-in
	// and effectively host-root; independent of `privileged` (kernel caps). On rootless
	// podman the socket is unreachable in-container — use Docker or `podman machine set --rootful`.
	Host bool `yaml:"host,omitempty" json:"host,omitempty" mapstructure:"host"`
}
