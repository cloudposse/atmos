package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	installAttempts     = 3
	installInitialDelay = 15 * time.Second
	installMaxDelay     = 30 * time.Second
	installReceiptName  = ".atmos-install.yaml"
	installReceiptPerm  = 0o600
)

var (
	transientInstallStatus = regexp.MustCompile(`(?i)(?:http[^\n]*?|(?:status|response)(?: code)?[: ]*|error[: ]+|returned[: ]+)(?:429|500|502|503|504)\b`)
	errInstallCleanup      = errors.New("failed to clean partial plugin installation")
)

func defaultInstallRetryConfig() schema.RetryConfig {
	attempts, delay, maxDelay := installAttempts, installInitialDelay, installMaxDelay
	return schema.RetryConfig{MaxAttempts: &attempts, InitialDelay: &delay, MaxDelay: &maxDelay, BackoffStrategy: schema.BackoffExponential}
}

// install preserves Helm's install/uninstall hooks and their final-directory
// paths. Failed attempts remove only newly created entries before retrying.
func (i *Installer) install(ctx context.Context, spec Spec, replaceName string) error {
	defer perf.Track(nil, "plugin.Installer.install")()
	if err := ctx.Err(); err != nil {
		return err
	}
	backup, err := i.backupPlugin(replaceName)
	if err != nil {
		return err
	}
	if replaceName != "" {
		if err := i.uninstall(ctx, replaceName); err != nil {
			return i.finishInstall(backup, err)
		}
	}
	err = retry.WithPredicate(ctx, &i.retryConfig, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return i.installAttempt(ctx, spec, replaceName)
	}, isTransientInstallError)
	return i.finishInstall(backup, err)
}

func (i *Installer) installAttempt(ctx context.Context, spec Spec, expectedName string) error {
	before, err := directoryEntries(i.dir)
	if err != nil {
		return fmt.Errorf("%w: inspect plugin directory: %w", errUtils.ErrHelmPluginInstall, err)
	}
	err = i.runInstall(ctx, spec)
	if err == nil {
		var pluginDir string
		pluginDir, err = validateInstall(i.dir, spec, before, expectedName)
		if err == nil {
			err = writeInstallReceipt(pluginDir, spec)
		}
	}
	if err == nil {
		return nil
	}
	if cleanupErr := removeNewEntries(i.dir, before); cleanupErr != nil {
		return errors.Join(err, errInstallCleanup, cleanupErr)
	}
	return err
}

func (i *Installer) runInstall(ctx context.Context, spec Spec) error {
	args := []string{"plugin", "install", spec.URL}
	if !spec.IsLatest() {
		args = append(args, "--version", spec.Version)
	}
	stdout, stderr, err := i.runner.Run(ctx, i.helmBin, args, i.env())
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return errUtils.Build(errUtils.ErrHelmPluginInstall).
			WithCause(fmt.Errorf("%w: %s", err, strings.TrimSpace(stdout+"\n"+stderr))).
			WithExplanationf("Failed to install helm plugin %q from %s", spec.Name, spec.URL).Err()
	}
	return nil
}

func directoryEntries(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(entries))
	for _, entry := range entries {
		result[entry.Name()] = true
	}
	return result, nil
}

func removeNewEntries(dir string, before map[string]bool) error {
	after, err := directoryEntries(dir)
	if err != nil {
		return err
	}
	for name := range after {
		if !before[name] {
			if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateInstall validates the plugin Helm just registered, without assuming
// any plugin-specific executable or version command.
func validateInstall(dir string, spec Spec, before map[string]bool, expectedName string) (string, error) {
	after, err := directoryEntries(dir)
	if err != nil {
		return "", fmt.Errorf("%w: inspect installed plugin: %w", errUtils.ErrHelmPluginInstall, err)
	}
	var installed string
	for name := range after {
		if before[name] {
			continue
		}
		path := filepath.Join(dir, name)
		metadata, err := readPluginMetadata(filepath.Join(path, "plugin.yaml"))
		if err != nil {
			return "", fmt.Errorf("%w: read plugin metadata: %w", errUtils.ErrHelmPluginInstall, err)
		}
		if err := validatePluginMetadata(metadata, spec, expectedName); err != nil {
			return "", err
		}
		if installed != "" {
			return "", fmt.Errorf("%w: expected one installed plugin", errUtils.ErrHelmPluginInstall)
		}
		installed = path
	}
	if installed == "" {
		return "", fmt.Errorf("%w: no plugin installed", errUtils.ErrHelmPluginInstall)
	}
	return installed, nil
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

func validatePluginMetadata(metadata pluginMetadata, spec Spec, expectedName string) error {
	knownURL, knownAlias := catalogURL(spec.Name)
	invalidAlias := knownAlias && knownURL == spec.URL && !slices.Contains(spec.candidateNames(), metadata.Name)
	if metadata.Name == "" || invalidAlias || (expectedName != "" && metadata.Name != expectedName) {
		return fmt.Errorf("%w: expected plugin %s, got %s", errUtils.ErrHelmPluginInstall, spec.Name, metadata.Name)
	}
	if _, err := semver.StrictNewVersion(strings.TrimPrefix(spec.Version, "v")); err == nil && !versionsEqual(metadata.Version, spec.Version) {
		return fmt.Errorf("%w: expected version %s, got %s", errUtils.ErrHelmPluginInstall, spec.Version, metadata.Version)
	}
	return nil
}

type installReceipt struct {
	Source  string `yaml:"source"`
	Version string `yaml:"version"`
}

func writeInstallReceipt(dir string, spec Spec) error {
	// A receipt is written only after Helm and all its installation hooks succeed.
	data, err := yaml.Marshal(installReceipt{Source: spec.URL, Version: spec.Version})
	if err != nil {
		return fmt.Errorf("%w: encode install receipt: %w", errUtils.ErrHelmPluginInstall, err)
	}
	if err := os.WriteFile(filepath.Join(dir, installReceiptName), data, installReceiptPerm); err != nil {
		return fmt.Errorf("%w: record successful installation: %w", errUtils.ErrHelmPluginInstall, err)
	}
	return nil
}

func (i *Installer) installReceipt(name string) (installReceipt, bool) {
	var receipt installReceipt
	dir, err := findPluginDirectory(i.dir, name)
	if err != nil || dir == "" {
		return receipt, false
	}
	data, err := os.ReadFile(filepath.Join(dir, installReceiptName))
	if err != nil {
		return receipt, false
	}
	err = yaml.Unmarshal(data, &receipt)
	return receipt, err == nil
}

func (i *Installer) hasInstallReceipt(name string, spec Spec) bool {
	receipt, ok := i.installReceipt(name)
	return ok && receipt.Source == spec.URL && receipt.Version == spec.Version
}

func isTransientInstallError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errInstallCleanup) {
		return false
	}
	message := strings.ToLower(err.Error())
	if transientInstallStatus.MatchString(message) {
		return true
	}
	for _, marker := range []string{"could not resolve host", "no such host", "temporary failure in name resolution", "connection reset", "connection refused", "connection timed out", "i/o timeout", "tls handshake timeout", "unexpected eof"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
