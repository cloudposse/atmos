package container

//go:generate go run go.uber.org/mock/mockgen@latest -source=runtime.go -destination=mock_runtime_test.go -package=container

import (
	"context"
	"io"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
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

	// Execution - IO streams configured via options structs
	Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error
	// Shell opens an interactive shell in a running container (a new shell
	// process via `exec`). Used to "shell into" a container.
	Shell(ctx context.Context, containerID string, opts *ShellOptions) error
	// Attach connects local stdin/stdout/stderr to a running container's main
	// process (PID 1) via `docker/podman attach`. Unlike Shell, it does not
	// start a new process — it attaches to the existing one (Docker/Compose
	// `attach` semantics).
	Attach(ctx context.Context, containerID string, opts *AttachOptions) error

	// Image operations
	Pull(ctx context.Context, image string) error
	Tag(ctx context.Context, source, target string) error
	Push(ctx context.Context, image string) (*PushResult, error)
	ImageInspect(ctx context.Context, image string) (*ImageInfo, error)

	// Logs - methods that produce user-facing output accept io.Writer
	Logs(ctx context.Context, containerID string, follow bool, tail string, stdout, stderr io.Writer) error

	// Info
	Info(ctx context.Context) (*RuntimeInfo, error)
}

// EnvSetter is implemented by runtimes whose CLI subprocesses can be launched
// with a specific environment. The container step uses this to forward the
// identity-resolved environment (e.g. the DOCKER_CONFIG materialized by the
// aws/ecr auth integration, or AWS_* credentials) so build/push/run can reach
// private registries. Runtimes that do not implement it inherit os.Environ().
type EnvSetter interface {
	SetEnv(env []string)
}

// BuildConfig represents container image build configuration.
type BuildConfig struct {
	Dockerfile string
	Context    string
	Engine     string
	Args       map[string]string
	Tags       []string
	Target     string
	NoCache    bool
	Pull       bool
	Bake       *BakeConfig
}

// BakeConfig represents Docker Buildx Bake configuration.
type BakeConfig struct {
	File    string
	Files   []string
	Target  string
	Targets []string
	Set     []string
	Vars    map[string]string
	Load    bool
	Push    bool
	Print   bool
}

// ImageInfo contains metadata about a local container image.
type ImageInfo struct {
	ID             string
	RepoTags       []string
	RepoDigests    []string
	Size           int64
	Created        string
	Architecture   string
	Os             string
	Author         string
	Labels         map[string]string
	Env            []string
	Cmd            []string
	Entrypoint     []string
	ExposedPorts   []string
	StopSignal     string
	StorageDriver  string
	LayerDigests   []string
	Layers         int
	RawInspectJSON string
}

// PushResult contains metadata returned by a container image push.
type PushResult struct {
	Image  string
	Digest string
	Output string
}

// CreateConfig represents container creation configuration.
type CreateConfig struct {
	Name            string
	Image           string
	Command         []string // Command/args to run; appended after the image. Ignored when OverrideCommand is set.
	WorkspaceFolder string
	Mounts          []Mount
	Ports           []PortBinding
	Env             map[string]string
	User            string
	Labels          map[string]string

	// Runtime configuration
	RunArgs         []string
	OverrideCommand bool           // Whether to override default command with sleep infinity
	Init            bool           // Whether to use init process
	Privileged      bool           // Run in privileged mode
	Host            bool           // Grant access to the host container runtime (Docker-out-of-Docker)
	CapAdd          []string       // Linux capabilities to add
	SecurityOpt     []string       // Security options
	Restart         *RestartPolicy // Restart policy (nil = runtime default)
	HealthCheck     *HealthCheck   // Health check (nil = inherit image healthcheck)
}

// RestartPolicy is the resolved restart policy for a long-lived container.
// It maps to the docker/podman `--restart` flag (`<policy>[:<max_retries>]`).
type RestartPolicy struct {
	Policy     string // no, always, on-failure, unless-stopped.
	MaxRetries int    // on-failure only.
}

// HealthCheck is the resolved health check for a container. Cmd is the shell
// command run by `--health-cmd`; when Disable is set the container is created
// with `--no-healthcheck` and the remaining fields are ignored. Duration fields
// are Go duration strings forwarded verbatim to the runtime.
type HealthCheck struct {
	Cmd           string // shell command for --health-cmd ("" when Disable).
	Interval      string
	Timeout       string
	Retries       int
	StartPeriod   string
	StartInterval string
	Disable       bool
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
	Health  string // healthy, unhealthy, starting, or "" when no healthcheck.
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

	// IO streams for input/output. If nil, defaults to iolib.Data/UI.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// ShellOptions represents options for opening an interactive shell in a
// container (via `exec`). Shell/ShellArgs select the shell program and its
// arguments; an empty Shell defaults to /bin/bash.
type ShellOptions struct {
	Shell     string
	ShellArgs []string
	User      string

	// IO streams for input/output. If nil, defaults to os.Stdin and iolib.Data/UI.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// AttachOptions represents options for attaching to a container's main process
// (PID 1) via `docker/podman attach`.
type AttachOptions struct {
	// NoStdin attaches output only, leaving the container's stdin unconnected
	// (maps to `--no-stdin`).
	NoStdin bool
	// DetachKeys overrides the key sequence that detaches without stopping the
	// container (maps to `--detach-keys`; runtime default is ctrl-p,ctrl-q).
	DetachKeys string

	// IO streams for input/output. If nil, defaults to os.Stdin and iolib.Data/UI.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
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
	defer perf.Track(nil, "container.Type.String")()

	return string(t)
}
