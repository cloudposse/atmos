package container

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func verifyRuntimeRetry(t *testing.T, stage string, arrange func(*MockRuntime, error), invoke func(Runtime) (any, error), want any) {
	t.Helper()
	runtime := NewMockRuntime(gomock.NewController(t))
	failure := errors.New("transient runtime failure")
	arrange(runtime, failure)
	calls := 0
	wrapped := WithRetry(runtime, func(ctx context.Context, actualStage string, call func() error) error {
		calls++
		require.Equal(t, t.Context(), ctx)
		require.Equal(t, stage, actualStage)
		require.ErrorIs(t, call(), failure)
		return call()
	})
	result, err := invoke(wrapped)
	require.NoError(t, err)
	require.Equal(t, want, result)
	require.Equal(t, 1, calls, "one runtime invocation owns one retry budget")
}

func TestRetryRuntimeBuild(t *testing.T) {
	verifyRuntimeRetry(t, "Build", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Build(t.Context(), &BuildConfig{Context: "source"}).Return(failure),
			runtime.EXPECT().Build(t.Context(), &BuildConfig{Context: "source"}).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Build(t.Context(), &BuildConfig{Context: "source"})
	}, nil)
}

func TestRetryRuntimeCreate(t *testing.T) {
	want := "created-id"
	verifyRuntimeRetry(t, "Create", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Create(t.Context(), &CreateConfig{Name: "app", Image: "app:latest"}).Return("", failure),
			runtime.EXPECT().Create(t.Context(), &CreateConfig{Name: "app", Image: "app:latest"}).Return(want, nil),
		)
	}, func(runtime Runtime) (any, error) {
		return runtime.Create(t.Context(), &CreateConfig{Name: "app", Image: "app:latest"})
	}, want)
}

func TestRetryRuntimeStart(t *testing.T) {
	verifyRuntimeRetry(t, "Start", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Start(t.Context(), "id").Return(failure),
			runtime.EXPECT().Start(t.Context(), "id").Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Start(t.Context(), "id")
	}, nil)
}

func TestRetryRuntimeStop(t *testing.T) {
	verifyRuntimeRetry(t, "Stop", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Stop(t.Context(), "id", time.Second).Return(failure),
			runtime.EXPECT().Stop(t.Context(), "id", time.Second).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Stop(t.Context(), "id", time.Second)
	}, nil)
}

func TestRetryRuntimeRemove(t *testing.T) {
	verifyRuntimeRetry(t, "Remove", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Remove(t.Context(), "id", true).Return(failure),
			runtime.EXPECT().Remove(t.Context(), "id", true).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Remove(t.Context(), "id", true)
	}, nil)
}

func TestRetryRuntimeInspect(t *testing.T) {
	want := &Info{ID: "id", Status: "running"}
	verifyRuntimeRetry(t, "Inspect", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Inspect(t.Context(), "id").Return(nil, failure),
			runtime.EXPECT().Inspect(t.Context(), "id").Return(want, nil),
		)
	}, func(runtime Runtime) (any, error) {
		return runtime.Inspect(t.Context(), "id")
	}, want)
}

func TestRetryRuntimeList(t *testing.T) {
	want := []Info{{ID: "first"}, {ID: "last"}}
	verifyRuntimeRetry(t, "List", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().List(t.Context(), map[string]string{"label": "app"}).Return(nil, failure),
			runtime.EXPECT().List(t.Context(), map[string]string{"label": "app"}).Return(want, nil),
		)
	}, func(runtime Runtime) (any, error) {
		return runtime.List(t.Context(), map[string]string{"label": "app"})
	}, want)
}

func TestRetryRuntimeExec(t *testing.T) {
	verifyRuntimeRetry(t, "Exec", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Exec(t.Context(), "id", []string{"echo", "hello"}, &ExecOptions{}).Return(failure),
			runtime.EXPECT().Exec(t.Context(), "id", []string{"echo", "hello"}, &ExecOptions{}).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Exec(t.Context(), "id", []string{"echo", "hello"}, &ExecOptions{})
	}, nil)
}

func TestRetryRuntimeShell(t *testing.T) {
	verifyRuntimeRetry(t, "Shell", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Shell(t.Context(), "id", &ShellOptions{}).Return(failure),
			runtime.EXPECT().Shell(t.Context(), "id", &ShellOptions{}).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Shell(t.Context(), "id", &ShellOptions{})
	}, nil)
}

func TestRetryRuntimeAttach(t *testing.T) {
	verifyRuntimeRetry(t, "Attach", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Attach(t.Context(), "id", &AttachOptions{}).Return(failure),
			runtime.EXPECT().Attach(t.Context(), "id", &AttachOptions{}).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Attach(t.Context(), "id", &AttachOptions{})
	}, nil)
}

func TestRetryRuntimePull(t *testing.T) {
	verifyRuntimeRetry(t, "Pull", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Pull(t.Context(), "app:latest").Return(failure),
			runtime.EXPECT().Pull(t.Context(), "app:latest").Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Pull(t.Context(), "app:latest")
	}, nil)
}

func TestRetryRuntimeTag(t *testing.T) {
	verifyRuntimeRetry(t, "Tag", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Tag(t.Context(), "source", "target").Return(failure),
			runtime.EXPECT().Tag(t.Context(), "source", "target").Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Tag(t.Context(), "source", "target")
	}, nil)
}

func TestRetryRuntimePush(t *testing.T) {
	want := &PushResult{}
	verifyRuntimeRetry(t, "Push", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Push(t.Context(), "app:latest").Return(nil, failure),
			runtime.EXPECT().Push(t.Context(), "app:latest").Return(want, nil),
		)
	}, func(runtime Runtime) (any, error) {
		return runtime.Push(t.Context(), "app:latest")
	}, want)
}

func TestRetryRuntimeImageInspect(t *testing.T) {
	want := &ImageInfo{}
	verifyRuntimeRetry(t, "ImageInspect", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().ImageInspect(t.Context(), "app:latest").Return(nil, failure),
			runtime.EXPECT().ImageInspect(t.Context(), "app:latest").Return(want, nil),
		)
	}, func(runtime Runtime) (any, error) {
		return runtime.ImageInspect(t.Context(), "app:latest")
	}, want)
}

func TestRetryRuntimeLogs(t *testing.T) {
	verifyRuntimeRetry(t, "Logs", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Logs(t.Context(), "id", true, "20", io.Discard, io.Discard).Return(failure),
			runtime.EXPECT().Logs(t.Context(), "id", true, "20", io.Discard, io.Discard).Return(nil),
		)
	}, func(runtime Runtime) (any, error) {
		return nil, runtime.Logs(t.Context(), "id", true, "20", io.Discard, io.Discard)
	}, nil)
}

func TestRetryRuntimeInfo(t *testing.T) {
	want := &RuntimeInfo{}
	verifyRuntimeRetry(t, "Info", func(runtime *MockRuntime, failure error) {
		gomock.InOrder(
			runtime.EXPECT().Info(t.Context()).Return(nil, failure),
			runtime.EXPECT().Info(t.Context()).Return(want, nil),
		)
	}, func(runtime Runtime) (any, error) {
		return runtime.Info(t.Context())
	}, want)
}

func TestRetryRuntimeDisabledAndRejected(t *testing.T) {
	runtime := NewMockRuntime(gomock.NewController(t))
	require.Same(t, runtime, WithRetry(runtime, nil))
	failure := errors.New("policy denied")
	wrapped := WithRetry(runtime, func(context.Context, string, func() error) error { return failure })
	require.ErrorIs(t, wrapped.Start(t.Context(), "id"), failure)
}

func TestRetryRuntimeEnvironment(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime, setter := NewMockRuntime(ctrl), NewMockEnvSetter(ctrl)
	env := []string{"CREDENTIAL=masked"}
	setter.EXPECT().SetEnv(env)
	run := func(_ context.Context, _ string, call func() error) error { return call() }
	wrapped := WithRetry(struct {
		Runtime
		EnvSetter
	}{runtime, setter}, run)
	wrapped.(EnvSetter).SetEnv(env)
	// Runtimes without an environment setter remain usable.
	WithRetry(runtime, run).(EnvSetter).SetEnv(env)
}

type retryNetworkProbe struct {
	ensureCalls, connectCalls int
}

func (p *retryNetworkProbe) EnsureNetwork(ctx context.Context, name string) error {
	p.ensureCalls++
	if name != "shared" {
		return errors.New("unexpected network")
	}
	return ctx.Err()
}

func (p *retryNetworkProbe) ConnectNetwork(ctx context.Context, network, id string, aliases []string) error {
	p.connectCalls++
	if network != "shared" || id != "id" || len(aliases) != 1 || aliases[0] != "app" {
		return errors.New("unexpected connection")
	}
	return ctx.Err()
}

func TestRetryRuntimeOptionalNetworks(t *testing.T) {
	for _, capabilities := range []string{"none", "ensure", "connect", "both"} {
		t.Run(capabilities, func(t *testing.T) {
			runtime := NewMockRuntime(gomock.NewController(t))
			probe := &retryNetworkProbe{}
			var base Runtime = runtime
			switch capabilities {
			case "ensure":
				base = struct {
					Runtime
					NetworkEnsurer
				}{runtime, probe}
			case "connect":
				base = struct {
					Runtime
					NetworkConnector
				}{runtime, probe}
			case "both":
				base = struct {
					Runtime
					NetworkEnsurer
					NetworkConnector
				}{runtime, probe, probe}
			}
			var stages []string
			wrapped := WithRetry(base, func(ctx context.Context, stage string, call func() error) error {
				require.Equal(t, t.Context(), ctx)
				stages = append(stages, stage)
				return call()
			})
			ensure, hasEnsure := wrapped.(NetworkEnsurer)
			connect, hasConnect := wrapped.(NetworkConnector)
			require.Equal(t, capabilities == "ensure" || capabilities == "both", hasEnsure)
			require.Equal(t, capabilities == "connect" || capabilities == "both", hasConnect)
			var expected []string
			if hasEnsure {
				require.NoError(t, ensure.EnsureNetwork(t.Context(), "shared"))
				expected = append(expected, "EnsureNetwork")
				require.Equal(t, 1, probe.ensureCalls)
			}
			if hasConnect {
				require.NoError(t, connect.ConnectNetwork(t.Context(), "shared", "id", []string{"app"}))
				expected = append(expected, "ConnectNetwork")
				require.Equal(t, 1, probe.connectCalls)
			}
			require.Equal(t, expected, stages)
		})
	}
}
