package container

import (
	"context"
	"io"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// OperationRetry retries one runtime call, never an entire multi-step lifecycle.
type OperationRetry func(context.Context, string, func() error) error

type retryRuntime struct {
	Runtime
	run OperationRetry
}

// WithRetry decorates runtime calls and preserves optional network capabilities.
func WithRetry(runtime Runtime, run OperationRetry) Runtime {
	defer perf.Track(nil, "container.WithRetry")()

	if run == nil {
		return runtime
	}
	base := &retryRuntime{Runtime: runtime, run: run}
	ensure, hasEnsure := runtime.(NetworkEnsurer)
	connect, hasConnect := runtime.(NetworkConnector)
	switch {
	case hasEnsure && hasConnect:
		return struct {
			*retryRuntime
			NetworkEnsurer
			NetworkConnector
		}{base, &retryNetworkEnsurer{ensure, run}, &retryNetworkConnector{connect, run}}
	case hasEnsure:
		return struct {
			*retryRuntime
			NetworkEnsurer
		}{base, &retryNetworkEnsurer{ensure, run}}
	case hasConnect:
		return struct {
			*retryRuntime
			NetworkConnector
		}{base, &retryNetworkConnector{connect, run}}
	default:
		return base
	}
}

// SetEnv forwards the optional runtime subprocess environment capability.
func (r *retryRuntime) SetEnv(env []string) {
	defer perf.Track(nil, "container.retryRuntime.SetEnv")()

	if setter, ok := r.Runtime.(EnvSetter); ok {
		setter.SetEnv(env)
	}
}

func (r *retryRuntime) Build(ctx context.Context, config *BuildConfig) error {
	defer perf.Track(nil, "container.retryRuntime.Build")()

	return r.run(ctx, "Build", func() error { return r.Runtime.Build(ctx, config) })
}

func (r *retryRuntime) Create(ctx context.Context, config *CreateConfig) (string, error) {
	defer perf.Track(nil, "container.retryRuntime.Create")()

	var result string
	err := r.run(ctx, "Create", func() error {
		var err error
		result, err = r.Runtime.Create(ctx, config)
		return err
	})
	return result, err
}

func (r *retryRuntime) Start(ctx context.Context, containerID string) error {
	defer perf.Track(nil, "container.retryRuntime.Start")()

	return r.run(ctx, "Start", func() error { return r.Runtime.Start(ctx, containerID) })
}

func (r *retryRuntime) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	defer perf.Track(nil, "container.retryRuntime.Stop")()

	return r.run(ctx, "Stop", func() error { return r.Runtime.Stop(ctx, containerID, timeout) })
}

func (r *retryRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	defer perf.Track(nil, "container.retryRuntime.Remove")()

	return r.run(ctx, "Remove", func() error { return r.Runtime.Remove(ctx, containerID, force) })
}

func (r *retryRuntime) Inspect(ctx context.Context, containerID string) (*Info, error) {
	defer perf.Track(nil, "container.retryRuntime.Inspect")()

	var result *Info
	err := r.run(ctx, "Inspect", func() error {
		var err error
		result, err = r.Runtime.Inspect(ctx, containerID)
		return err
	})
	return result, err
}

func (r *retryRuntime) List(ctx context.Context, filters map[string]string) ([]Info, error) {
	defer perf.Track(nil, "container.retryRuntime.List")()

	var result []Info
	err := r.run(ctx, "List", func() error {
		var err error
		result, err = r.Runtime.List(ctx, filters)
		return err
	})
	return result, err
}

func (r *retryRuntime) Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error {
	defer perf.Track(nil, "container.retryRuntime.Exec")()

	return r.run(ctx, "Exec", func() error { return r.Runtime.Exec(ctx, containerID, cmd, opts) })
}

func (r *retryRuntime) Shell(ctx context.Context, containerID string, opts *ShellOptions) error {
	defer perf.Track(nil, "container.retryRuntime.Shell")()

	return r.run(ctx, "Shell", func() error { return r.Runtime.Shell(ctx, containerID, opts) })
}

func (r *retryRuntime) Attach(ctx context.Context, containerID string, opts *AttachOptions) error {
	defer perf.Track(nil, "container.retryRuntime.Attach")()

	return r.run(ctx, "Attach", func() error { return r.Runtime.Attach(ctx, containerID, opts) })
}

func (r *retryRuntime) Pull(ctx context.Context, image string) error {
	defer perf.Track(nil, "container.retryRuntime.Pull")()

	return r.run(ctx, "Pull", func() error { return r.Runtime.Pull(ctx, image) })
}

func (r *retryRuntime) Tag(ctx context.Context, source, target string) error {
	defer perf.Track(nil, "container.retryRuntime.Tag")()

	return r.run(ctx, "Tag", func() error { return r.Runtime.Tag(ctx, source, target) })
}

func (r *retryRuntime) Push(ctx context.Context, image string) (*PushResult, error) {
	defer perf.Track(nil, "container.retryRuntime.Push")()

	var result *PushResult
	err := r.run(ctx, "Push", func() error {
		var err error
		result, err = r.Runtime.Push(ctx, image)
		return err
	})
	return result, err
}

func (r *retryRuntime) ImageInspect(ctx context.Context, image string) (*ImageInfo, error) {
	defer perf.Track(nil, "container.retryRuntime.ImageInspect")()

	var result *ImageInfo
	err := r.run(ctx, "ImageInspect", func() error {
		var err error
		result, err = r.Runtime.ImageInspect(ctx, image)
		return err
	})
	return result, err
}

//nolint:revive // Signature implements Runtime.Logs.
func (r *retryRuntime) Logs(ctx context.Context, containerID string, follow bool, tail string, stdout, stderr io.Writer) error {
	defer perf.Track(nil, "container.retryRuntime.Logs")()

	return r.run(ctx, "Logs", func() error { return r.Runtime.Logs(ctx, containerID, follow, tail, stdout, stderr) })
}

func (r *retryRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	defer perf.Track(nil, "container.retryRuntime.Info")()

	var result *RuntimeInfo
	err := r.run(ctx, "Info", func() error {
		var err error
		result, err = r.Runtime.Info(ctx)
		return err
	})
	return result, err
}

type retryNetworkEnsurer struct {
	NetworkEnsurer
	run OperationRetry
}

func (r *retryNetworkEnsurer) EnsureNetwork(ctx context.Context, name string) error {
	defer perf.Track(nil, "container.retryNetworkEnsurer.EnsureNetwork")()

	return r.run(ctx, "EnsureNetwork", func() error { return r.NetworkEnsurer.EnsureNetwork(ctx, name) })
}

type retryNetworkConnector struct {
	NetworkConnector
	run OperationRetry
}

func (r *retryNetworkConnector) ConnectNetwork(ctx context.Context, network, containerID string, aliases []string) error {
	defer perf.Track(nil, "container.retryNetworkConnector.ConnectNetwork")()

	return r.run(ctx, "ConnectNetwork", func() error { return r.NetworkConnector.ConnectNetwork(ctx, network, containerID, aliases) })
}
