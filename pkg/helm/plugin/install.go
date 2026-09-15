package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/hashicorp/go-getter"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	diffRepository      = "https://github.com/databus23/helm-diff"
	installAttempts     = 3
	installInitialDelay = 15 * time.Second
	installMaxDelay     = 30 * time.Second
)

var transientInstallStatus = regexp.MustCompile(`(?i)(?:http[^\n]*?|(?:status|response)(?: code)?[: ]*|error[: ]+|returned[: ]+)(?:429|500|502|503|504)\b`)

func defaultInstallRetryConfig() schema.RetryConfig {
	attempts := installAttempts
	delay := installInitialDelay
	maxDelay := installMaxDelay
	return schema.RetryConfig{
		MaxAttempts: &attempts, InitialDelay: &delay, MaxDelay: &maxDelay,
		BackoffStrategy: schema.BackoffExponential,
	}
}

// install stages pinned Helm Diff archives, retrying transient failures. Other
// plugins retain Helm's final-directory install semantics: their hooks may embed
// absolute paths that would break if a staged installation were relocated.
func (i *Installer) install(ctx context.Context, spec Spec, replaceName string) error {
	defer perf.Track(nil, "plugin.Installer.install")()

	if installURL(spec, runtime.GOOS, runtime.GOARCH) == spec.URL {
		return i.installWithHelm(ctx, spec, replaceName)
	}

	return retry.WithPredicate(ctx, &i.retryConfig, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return i.installAttempt(ctx, spec, replaceName)
	}, isTransientInstallError)
}

func (i *Installer) installAttempt(ctx context.Context, spec Spec, replaceName string) error {
	stage, err := os.MkdirTemp(filepath.Dir(i.dir), ".helm-plugin-install-")
	if err != nil {
		return fmt.Errorf("%w: create staging directory: %w", errUtils.ErrHelmPluginInstall, err)
	}
	defer os.RemoveAll(stage)

	if err := i.fetchArchive(ctx, installURL(spec, runtime.GOOS, runtime.GOARCH), stage); err != nil {
		return fmt.Errorf("%w: download Helm Diff: %w", errUtils.ErrHelmPluginInstall, err)
	}
	pluginDir, err := validateInstall(stage, spec)
	if err != nil {
		return err
	}
	if isPinnedDiff(spec) {
		if err := i.verifyDiff(ctx, spec, stage); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return i.publishInstall(pluginDir, replaceName)
}

func (i *Installer) installWithHelm(ctx context.Context, spec Spec, replaceName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if replaceName != "" {
		if err := i.uninstall(ctx, replaceName); err != nil {
			return err
		}
	}
	if err := i.runInstall(ctx, spec, i.dir); err != nil {
		return err
	}
	if isPinnedDiff(spec) {
		return i.verifyDiff(ctx, spec, i.dir)
	}
	return nil
}

func (i *Installer) runInstall(ctx context.Context, spec Spec, stage string) error {
	args := []string{"plugin", "install", spec.URL}
	if !spec.IsLatest() {
		args = append(args, "--version", spec.Version)
	}
	stdout, stderr, err := i.runner.Run(ctx, i.helmBin, args, []string{"HELM_PLUGINS=" + stage})
	if err != nil {
		return errUtils.Build(errUtils.ErrHelmPluginInstall).
			WithCause(fmt.Errorf("%w: %s", err, strings.TrimSpace(stdout+"\n"+stderr))).
			WithExplanationf("Failed to install helm plugin %q from %s", spec.Name, spec.URL).
			WithExplanationf("helm reported: %s", strings.TrimSpace(stdout+"\n"+stderr)).Err()
	}
	return nil
}

// Release archives are complete Helm plugins. Extract directly into staging:
// Helm 3 may mistake GitHub's octet-stream response for a VCS repository, and
// the repository install hook on Windows may silently download latest.
func downloadDiffRelease(ctx context.Context, source, stage string) error {
	client := &getter.Client{
		Ctx: ctx, Src: "https::" + source, Dst: stage, Mode: getter.ClientModeDir,
		DisableSymlinks: true,
		Getters: map[string]getter.Getter{
			"https": &getter.HttpGetter{Client: httpClient.NewGitHubAuthenticatedHTTPClient(github.GetGitHubToken())},
		},
	}
	return client.Get()
}

// validateInstall locates the plugin Helm registered and checks its metadata.
func validateInstall(stage string, spec Spec) (string, error) {
	paths, err := filepath.Glob(filepath.Join(stage, "*", "plugin.yaml"))
	if err != nil {
		return "", fmt.Errorf("%w: locate plugin metadata: %w", errUtils.ErrHelmPluginInstall, err)
	}
	if len(paths) != 1 {
		return "", fmt.Errorf("%w: expected one installed plugin, found %d", errUtils.ErrHelmPluginInstall, len(paths))
	}
	metadata, err := readPluginMetadata(paths[0])
	if err != nil {
		return "", fmt.Errorf("%w: read plugin metadata: %w", errUtils.ErrHelmPluginInstall, err)
	}
	if metadata.Name == "" || metadata.Version == "" {
		return "", fmt.Errorf("%w: missing plugin name or version", errUtils.ErrHelmPluginInstall)
	}
	if !slices.Contains(spec.candidateNames(), metadata.Name) {
		return "", fmt.Errorf("%w: expected plugin %s, got %s", errUtils.ErrHelmPluginInstall, spec.Name, metadata.Name)
	}
	if _, versionErr := semver.StrictNewVersion(strings.TrimPrefix(spec.Version, "v")); versionErr == nil && !versionsEqual(metadata.Version, spec.Version) {
		return "", fmt.Errorf("%w: expected version %s, got %s", errUtils.ErrHelmPluginInstall, spec.Version, metadata.Version)
	}
	return filepath.Dir(paths[0]), nil
}

type pluginMetadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

func readPluginMetadata(path string) (pluginMetadata, error) {
	var metadata pluginMetadata
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, err
	}
	err = yaml.Unmarshal(data, &metadata)
	return metadata, err
}

func (i *Installer) verifyDiff(ctx context.Context, spec Spec, dir string) error {
	stdout, stderr, err := i.runner.Run(ctx, i.helmBin, []string{"diff", "version"}, []string{"HELM_PLUGINS=" + dir})
	if err != nil {
		return fmt.Errorf("%w: verify Helm Diff: %s: %w", errUtils.ErrHelmPluginInstall, strings.TrimSpace(stderr), err)
	}
	if actual := strings.TrimSpace(stdout); !versionsEqual(actual, spec.Version) {
		return fmt.Errorf("%w: Helm Diff binary version mismatch: expected %s, got %s", errUtils.ErrHelmPluginInstall, spec.Version, actual)
	}
	return nil
}

func isPinnedDiff(spec Spec) bool {
	url := strings.TrimSuffix(strings.TrimRight(spec.URL, "/"), ".git")
	_, err := semver.StrictNewVersion(strings.TrimPrefix(spec.Version, "v"))
	return url == diffRepository && err == nil
}

// Helm Diff release archives include plugin.yaml and the executable. Installing
// the exact archive avoids install-binary.ps1's cwd-dependent git describe,
// which can silently select latest even when Helm checked out a pinned tag.
func installURL(spec Spec, goos, goarch string) string {
	if !isPinnedDiff(spec) || (goarch != "amd64" && goarch != "arm64") {
		return spec.URL
	}
	switch goos {
	case "darwin":
		goos = "macos"
	case "linux", "windows", "freebsd":
	default:
		return spec.URL
	}
	return fmt.Sprintf("%s/releases/download/v%s/helm-diff-%s-%s.tgz", diffRepository,
		strings.TrimPrefix(spec.Version, "v"), goos, goarch)
}

func isTransientInstallError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	if transientInstallStatus.MatchString(message) {
		return true
	}
	for _, marker := range []string{
		"could not resolve host", "no such host", "temporary failure in name resolution",
		"connection reset", "connection refused", "connection timed out", "i/o timeout",
		"tls handshake timeout", "unexpected eof",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
