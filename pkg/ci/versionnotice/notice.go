// Package versionnotice prepares optional stale-version annotations without delaying CI commands.
package versionnotice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/installer"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	sentinel         = "ATMOS_CI_VERSION_NOTICE_SHOWN"
	defaultTimeout   = 1000
	defaultThreshold = 10
)

type releaseClient interface {
	CheckBudget(context.Context) error
	Latest(context.Context, string, string) (string, error)
}

type dependencies struct {
	isCI   func() bool
	client func(context.Context) (releaseClient, bool)
	load   func(context.Context) (cfg.CacheConfig, error)
	update func(context.Context, func(*cfg.CacheConfig)) error
	hint   func(context.Context, string, string) string
}

func defaultDependencies() dependencies {
	return dependencies{
		isCI:   telemetry.IsCI,
		client: func(ctx context.Context) (releaseClient, bool) { return github.NewReleaseClient(ctx) },
		load:   cfg.LoadCacheContext,
		update: cfg.UpdateCacheContext,
		hint:   upgradeHint,
	}
}

// Start begins one optional background check. The returned function must run when
// the command finishes, before its output streams are torn down. It never waits
// for the worker and is safe to call more than once. Configuration is copied so
// subsequent command setup cannot race with the worker.
func Start(config *schema.AtmosConfiguration) func() {
	defer perf.Track(config, "versionnotice.Start")()

	return start(config, version.Version, defaultDependencies())
}

type result struct {
	annotation provider.Annotation
	marker     string
}

func start(config *schema.AtmosConfiguration, current string, deps dependencies) func() {
	if config == nil || !deps.isCI() || !config.Version.Check.Enabled || !config.Version.Check.CI.Enabled {
		return func() {}
	}
	settings, timeout := normalizeSettings(config, current)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ready := make(chan result, 1)
	go func() {
		notice, ok := prepare(ctx, settings, deps)
		if ok && ctx.Err() == nil {
			ready <- notice
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			// The worker never emits. Only a result already available at command exit
			// can be shown, even if a canceled dependency finishes later.
			defer cancel()
			select {
			case notice := <-ready:
				emit(&notice)
			default:
			}
		})
	}
}

func normalizeSettings(config *schema.AtmosConfiguration, current string) (noticeSettings, time.Duration) {
	timeout := config.Version.Check.Timeout
	if timeout <= 0 {
		log.Debug("Invalid CI version check timeout; using default", "timeout", timeout)
		timeout = defaultTimeout
	}
	threshold := config.Version.Check.MinorThreshold
	if threshold <= 0 {
		log.Debug("Invalid CI version minor threshold; using default", "threshold", threshold)
		threshold = defaultThreshold
	}
	// Copy only the scalar settings used by the worker, not shared config maps.
	settings := noticeSettings{
		current: current, threshold: uint64(threshold), frequency: config.Version.Check.Frequency,
		nativeRoot: config.Toolchain.InstallPath, buildDate: version.BuildDate,
	}
	if settings.nativeRoot != "" && !filepath.IsAbs(settings.nativeRoot) {
		settings.nativeRoot = filepath.Join(config.BasePathAbsolute, settings.nativeRoot)
	}
	return settings, time.Duration(timeout) * time.Millisecond
}

type noticeSettings struct {
	current    string
	threshold  uint64
	frequency  string
	nativeRoot string
	buildDate  string
}

func prepare(ctx context.Context, settings noticeSettings, deps dependencies) (result, bool) {
	client, authenticated := deps.client(ctx)
	if !authenticated || ctx.Err() != nil {
		return result{}, false
	}
	marker := jobMarker()
	if alreadyShown(marker) {
		return result{}, false
	}
	if !checkDue(ctx, settings.frequency, deps.load) {
		return result{}, false
	}
	current := stableVersion(settings.current)
	if current == nil {
		return result{}, false
	}
	latestTag, latest := fetchLatest(ctx, client)
	if latest == nil {
		return result{}, false
	}
	return prepareResult(ctx, settings, deps, releaseUpdate{tag: latestTag, behind: minorDistance(current, latest), marker: marker})
}

func checkDue(ctx context.Context, frequency string, load func(context.Context) (cfg.CacheConfig, error)) bool {
	cache, err := load(ctx)
	if err != nil {
		debugError(err)
		return false
	}
	if _, err := duration.Parse(frequency); err != nil {
		log.Debug("Invalid CI version check frequency; using daily", "frequency", frequency)
		frequency = "daily"
	}
	return ctx.Err() == nil && cfg.ShouldCheckForUpdates(cache.LastChecked, frequency)
}

func fetchLatest(ctx context.Context, client releaseClient) (string, *semver.Version) {
	if err := client.CheckBudget(ctx); err != nil {
		debugError(err)
		return "", nil
	}
	tag, err := client.Latest(ctx, "cloudposse", "atmos")
	if err != nil {
		debugError(err)
		return "", nil
	}
	if ctx.Err() != nil {
		return "", nil
	}
	return tag, stableVersion(tag)
}

type releaseUpdate struct {
	tag    string
	behind uint64
	marker string
}

func prepareResult(ctx context.Context, settings noticeSettings, deps dependencies, update releaseUpdate) (result, bool) {
	n := update.behind
	var notice result
	if n >= settings.threshold {
		latestText := strings.TrimPrefix(update.tag, "v")
		notice = result{marker: update.marker, annotation: provider.Annotation{
			Level:   provider.AnnotationWarning,
			Title:   annotationTitle(settings.current, n, settings.buildDate, time.Now()),
			Message: "Latest is " + latestText + ". " + deps.hint(ctx, settings.nativeRoot, latestText),
		}}
	}
	if ctx.Err() != nil {
		return result{}, false
	}
	if err := deps.update(ctx, func(cache *cfg.CacheConfig) { cache.LastChecked = time.Now().Unix() }); err != nil {
		debugError(err)
	}
	return notice, n >= settings.threshold && ctx.Err() == nil
}

func stableVersion(value string) *semver.Version {
	parsed, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v"))
	if err != nil || parsed.Prerelease() != "" || parsed.Metadata() != "" {
		return nil
	}
	return parsed
}

func minorDistance(current, latest *semver.Version) uint64 {
	if latest.Major() != current.Major() || latest.Minor() <= current.Minor() {
		return 0
	}
	return latest.Minor() - current.Minor()
}

func upgradeHint(ctx context.Context, root, latest string) string {
	return formatHint(installer.Detect(ctx, installer.WithNativeRoots(root)).UpgradeHint(latest))
}

func emit(notice *result) {
	if os.Getenv(sentinel) != "" {
		return
	}
	p := ci.ResolveProvider()
	annotator, ok := p.(provider.Annotator)
	if !ok {
		return
	}
	if !claimMarker(notice.marker) {
		return
	}
	if err := annotator.Annotate([]provider.Annotation{notice.annotation}); err != nil {
		debugError(err)
		return
	}
	if err := os.Setenv(sentinel, "1"); err != nil {
		debugError(err)
	}
	exportSentinel(p)
}

func exportSentinel(p ci.Provider) {
	// Generic WriteEnv can print a shell export; only use file-backed exporters
	// here so an optional notice cannot introduce extra output.
	if exporter, ok := p.(provider.EnvExporter); ok &&
		(os.Getenv("GITHUB_ENV") != "" && p.Name() == "github-actions" || os.Getenv("ATMOS_CI_ENV") != "" && p.Name() == ci.GenericProviderName) {
		if err := exporter.WriteEnv(sentinel, "1"); err != nil {
			debugError(err)
		}
	}
}

func debugError(err error) {
	log.Debug("Skipping optional CI version notice operation", "error", err)
}

// formatHint preserves conditions on package-manager commands in plain annotations.
func formatHint(hint installer.Hint) string {
	parts := make([]string, 0, 3)
	if hint.Command != "" {
		command := "Run: " + hint.Command
		if hint.Condition != "" {
			command = hint.Condition + ": " + command
		}
		parts = append(parts, command)
	}
	if hint.Message != "" {
		parts = append(parts, hint.Message)
	}
	if hint.URL != "" {
		parts = append(parts, "More info: "+hint.URL)
	}
	return strings.Join(parts, " ")
}
