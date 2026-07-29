package container

import (
	"context"
	"encoding/json"
	"io"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	cachepkg "github.com/cloudposse/atmos/pkg/cache"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

const (
	runtimeCacheSubpath = "container-runtime"
	runtimeCacheVersion = 1
)

type runtimeCacheRecord struct {
	Version int      `json:"version"`
	Runtime ctr.Type `json:"runtime"`
}

// runtimeResolution keeps the metadata needed to recover from a stale cached
// runtime. Cache failures are deliberately non-fatal: runtime discovery still
// works when the local cache cannot be read or written.
type runtimeResolution struct {
	runtime    ctr.Runtime
	cached     bool
	cache      *cachepkg.FileCache
	cacheKey   string
	preference string
	autoStart  bool
}

func (r runtimeResolution) invalidate() {
	if r.cached && r.cache != nil {
		_ = r.cache.Delete(r.cacheKey)
	}
}

// resolveRuntimeForContainerCommand uses a durable cache only for truly
// automatic selection. Explicit configuration must still validate the runtime
// the user asked for on every invocation.
func resolveRuntimeForContainerCommand(ctx context.Context, preference string, autoStart bool) (runtimeResolution, error) {
	return resolveRuntime(ctx, preference, autoStart, false)
}

// rediscoverRuntime bypasses a stale entry after a read-only list operation
// fails. The normal detector can then recover a stopped Podman machine.
func rediscoverRuntime(ctx context.Context, previous runtimeResolution) (runtimeResolution, error) {
	previous.invalidate()
	return resolveRuntime(ctx, previous.preference, previous.autoStart, true)
}

func resolveRuntime(ctx context.Context, preference string, autoStart, skipCache bool) (runtimeResolution, error) {
	resolution := runtimeResolution{preference: preference, autoStart: autoStart}
	if !usesAutomaticRuntime(preference) {
		runtime, err := detectRuntime(ctx, preference, autoStart)
		resolution.runtime = runtime
		return resolution, err
	}

	cacheKey := runtimeSelectionCacheKey(preference)
	cache, cacheErr := cachepkg.NewFileCache(runtimeCacheSubpath)
	if cacheErr == nil {
		resolution.cache = cache
		resolution.cacheKey = cacheKey
		if !skipCache {
			if runtime, ok := loadCachedRuntime(cache, cacheKey); ok {
				resolution.runtime = invalidatingRuntime{Runtime: runtime, invalidateCache: resolution.invalidate}
				resolution.cached = true
				return resolution, nil
			}
		}
	}

	runtime, err := detectRuntimeWithProgress(ctx, preference, autoStart)
	if err != nil {
		return resolution, err
	}
	resolution.runtime = runtime
	if cache != nil {
		storeCachedRuntime(cache, cacheKey, runtime)
	}
	return resolution, nil
}

func usesAutomaticRuntime(preference string) bool {
	configured := strings.TrimSpace(preference)
	envRuntime := strings.TrimSpace(runtimeEnv("ATMOS_CONTAINER_RUNTIME"))
	return (configured == "" || configured == string(ctr.TypeAuto)) &&
		(envRuntime == "" || envRuntime == string(ctr.TypeAuto))
}

func runtimeSelectionCacheKey(preference string) string {
	parts := []string{
		"container-runtime-selection", strconv.Itoa(runtimeCacheVersion), runtime.GOOS, runtime.GOARCH,
		strings.TrimSpace(preference), strings.TrimSpace(runtimeEnv("ATMOS_CONTAINER_RUNTIME")),
		runtimeEnv("PATH"), runtimeEnv("DOCKER_HOST"), runtimeEnv("DOCKER_CONTEXT"),
		runtimeEnv("DOCKER_TLS_VERIFY"), runtimeEnv("DOCKER_CERT_PATH"), runtimeEnv("PODMAN_HOST"),
		runtimeEnv("CONTAINER_HOST"), runtimeEnv("XDG_RUNTIME_DIR"),
	}
	return strings.Join(parts, "\x00")
}

func runtimeEnv(name string) string {
	_ = viper.BindEnv(name, name)
	return viper.GetString(name)
}

func loadCachedRuntime(cache *cachepkg.FileCache, key string) (ctr.Runtime, bool) {
	content, found, err := cache.Get(key)
	if err != nil || !found {
		return nil, false
	}

	var record runtimeCacheRecord
	if err := json.Unmarshal(content, &record); err != nil || record.Version != runtimeCacheVersion {
		_ = cache.Delete(key)
		return nil, false
	}
	switch record.Runtime {
	case ctr.TypeDocker:
		return ctr.NewDockerRuntime(), true
	case ctr.TypePodman:
		return ctr.NewPodmanRuntime(), true
	default:
		_ = cache.Delete(key)
		return nil, false
	}
}

func storeCachedRuntime(cache *cachepkg.FileCache, key string, runtime ctr.Runtime) {
	typeName := ctr.GetRuntimeType(runtime)
	if typeName != ctr.TypeDocker && typeName != ctr.TypePodman {
		return
	}
	content, err := json.Marshal(runtimeCacheRecord{Version: runtimeCacheVersion, Runtime: typeName})
	if err == nil {
		_ = cache.Set(key, content)
	}
}

func detectRuntimeWithProgress(ctx context.Context, preference string, autoStart bool) (ctr.Runtime, error) {
	if !terminal.New().IsTTY(terminal.Stdout) {
		return detectRuntime(ctx, preference, autoStart)
	}

	var runtime ctr.Runtime
	err := spinner.ExecWithSpinner("Detecting container runtime", "Container runtime detected", func() error {
		var err error
		runtime, err = detectRuntime(ctx, preference, autoStart)
		return err
	})
	return runtime, err
}

// invalidatingRuntime makes stale cache entries self-healing for every
// container command. Mutating operations are not retried automatically; their
// error is returned unchanged, while the next invocation performs discovery.
type invalidatingRuntime struct {
	ctr.Runtime
	invalidateCache func()
}

func (r invalidatingRuntime) invalidate(err error) error {
	if err != nil && r.invalidateCache != nil {
		r.invalidateCache()
	}
	return err
}

func (r invalidatingRuntime) Build(ctx context.Context, config *ctr.BuildConfig) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Build")()

	return r.invalidate(r.Runtime.Build(ctx, config))
}

func (r invalidatingRuntime) Create(ctx context.Context, config *ctr.CreateConfig) (string, error) {
	defer perf.Track(nil, "container.invalidatingRuntime.Create")()

	v, err := r.Runtime.Create(ctx, config)
	return v, r.invalidate(err)
}

func (r invalidatingRuntime) Start(ctx context.Context, id string) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Start")()

	return r.invalidate(r.Runtime.Start(ctx, id))
}

func (r invalidatingRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Stop")()

	return r.invalidate(r.Runtime.Stop(ctx, id, timeout))
}

func (r invalidatingRuntime) Remove(ctx context.Context, id string, force bool) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Remove")()

	return r.invalidate(r.Runtime.Remove(ctx, id, force))
}

func (r invalidatingRuntime) Inspect(ctx context.Context, id string) (*ctr.Info, error) {
	defer perf.Track(nil, "container.invalidatingRuntime.Inspect")()

	v, err := r.Runtime.Inspect(ctx, id)
	return v, r.invalidate(err)
}

func (r invalidatingRuntime) List(ctx context.Context, filters map[string]string) ([]ctr.Info, error) {
	defer perf.Track(nil, "container.invalidatingRuntime.List")()

	v, err := r.Runtime.List(ctx, filters)
	return v, r.invalidate(err)
}

func (r invalidatingRuntime) Exec(ctx context.Context, id string, cmd []string, opts *ctr.ExecOptions) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Exec")()

	return r.invalidate(r.Runtime.Exec(ctx, id, cmd, opts))
}

func (r invalidatingRuntime) Shell(ctx context.Context, id string, opts *ctr.ShellOptions) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Shell")()

	return r.invalidate(r.Runtime.Shell(ctx, id, opts))
}

func (r invalidatingRuntime) Attach(ctx context.Context, id string, opts *ctr.AttachOptions) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Attach")()

	return r.invalidate(r.Runtime.Attach(ctx, id, opts))
}

func (r invalidatingRuntime) Pull(ctx context.Context, image string) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Pull")()

	return r.invalidate(r.Runtime.Pull(ctx, image))
}

func (r invalidatingRuntime) Tag(ctx context.Context, source, target string) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Tag")()

	return r.invalidate(r.Runtime.Tag(ctx, source, target))
}

func (r invalidatingRuntime) Push(ctx context.Context, image string) (*ctr.PushResult, error) {
	defer perf.Track(nil, "container.invalidatingRuntime.Push")()

	v, err := r.Runtime.Push(ctx, image)
	return v, r.invalidate(err)
}

func (r invalidatingRuntime) ImageInspect(ctx context.Context, image string) (*ctr.ImageInfo, error) {
	defer perf.Track(nil, "container.invalidatingRuntime.ImageInspect")()

	v, err := r.Runtime.ImageInspect(ctx, image)
	return v, r.invalidate(err)
}

//nolint:revive // argument-limit: matches the container Runtime interface.
func (r invalidatingRuntime) Logs(ctx context.Context, id string, follow bool, tail string, stdout, stderr io.Writer) error {
	defer perf.Track(nil, "container.invalidatingRuntime.Logs")()

	return r.invalidate(r.Runtime.Logs(ctx, id, follow, tail, stdout, stderr))
}

func (r invalidatingRuntime) Info(ctx context.Context) (*ctr.RuntimeInfo, error) {
	defer perf.Track(nil, "container.invalidatingRuntime.Info")()

	v, err := r.Runtime.Info(ctx)
	return v, r.invalidate(err)
}

func (r invalidatingRuntime) SetEnv(env []string) {
	defer perf.Track(nil, "container.invalidatingRuntime.SetEnv")()

	if setter, ok := r.Runtime.(ctr.EnvSetter); ok {
		setter.SetEnv(env)
	}
}
