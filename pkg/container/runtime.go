package container

//go:generate go run go.uber.org/mock/mockgen@latest -source=runtime.go -destination=mock_runtime_test.go -package=container

import (
	"context"
	"time"
)

// Runtime defines the interface for container runtime operations.
// This interface abstracts Docker and Podman operations for testability.
type Runtime interface {
	// Lifecycle operations
	Build(ctx context.Context, config *BuildConfig) error
	Create(ctx context.Context, config *CreateConfig) (string, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeout time.Duration) error
	Remove(ctx context.Context, containerID string, force bool) error

	// State inspection
	Inspect(ctx context.Context, containerID string) (*Info, error)
	List(ctx context.Context, filters map[string]string) ([]Info, error)

	// Execution
	Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error
	Attach(ctx context.Context, containerID string, opts *AttachOptions) error

	// Image operations
	Pull(ctx context.Context, image string) error

	// Logs
	Logs(ctx context.Context, containerID string, follow bool, tail string) error

	// Info
	Info(ctx context.Context) (*RuntimeInfo, error)
}

// BuildConfig represents container image build configuration.
type BuildConfig struct {
	Dockerfile string
	Context    string
	Args       map[string]string
	Tags       []string
}

// CreateConfig represents container creation configuration.
type CreateConfig struct {
	Name            string
	Image           string
	WorkspaceFolder string
	Mounts          []Mount
	Ports           []PortBinding
	Env             map[string]string
	User            string
	Labels          map[string]string

	// Runtime configuration
	RunArgs         []string
	OverrideCommand bool     // Whether to override default command with sleep infinity
	Init            bool     // Whether to use init process
	Privileged      bool     // Run in privileged mode
	CapAdd          []string // Linux capabilities to add
	SecurityOpt     []string // Security options
}

// Mount represents a volume mount.
type Mount struct {
	Type     string // bind, volume, tmpfs
	Source   string
	Target   string
	ReadOnly bool
}

// PortBinding represents a port mapping.
type PortBinding struct {
	ContainerPort int
	HostPort      int
	Protocol      string // tcp, udp
}

// Info represents container state information.
type Info struct {
	ID      string
	Name    string
	Image   string
	Status  string // running, stopped, exited, etc.
	Created time.Time
	Ports   []PortBinding
	Labels  map[string]string
}

// ExecOptions represents options for executing commands in containers.
type ExecOptions struct {
	User         string
	WorkingDir   string
	Env          []string
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
	Tty          bool
}

// AttachOptions represents options for attaching to containers.
type AttachOptions struct {
	Shell     string
	ShellArgs []string
	User      string
}

// RuntimeInfo represents container runtime information.
type RuntimeInfo struct {
	Type    string // docker, podman
	Version string
	Running bool
}

// Type represents the container runtime type.
type Type string

const (
	// TypeDocker represents Docker runtime.
	TypeDocker Type = "docker"

	// TypePodman represents Podman runtime.
	TypePodman Type = "podman"
)

func (t Type) String() string {
	return string(t)
}
