package workdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/duration"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Time conversion constants for duration formatting.
const (
	hoursPerDay    = 24
	minutesPerHour = 60
)

// CleanWorkdir removes the working directory for a specific component in a stack.
// The workdir name follows the stack-component naming convention (e.g., "dev-vpc").
func CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
	defer perf.Track(atmosConfig, "workdir.CleanWorkdir")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	// Construct workdir name using stack-component naming convention.
	workdirName := fmt.Sprintf("%s-%s", stack, component)
	workdirPath := filepath.Join(basePath, WorkdirPath, "terraform", workdirName)

	// Check if workdir exists.
	if _, err := os.Stat(workdirPath); os.IsNotExist(err) {
		ui.Info(fmt.Sprintf("No workdir found for component '%s' in stack '%s'", component, stack))
		return nil
	}

	ui.Info(fmt.Sprintf("Cleaning workdir for component '%s' in stack '%s'", component, stack))

	if err := os.RemoveAll(workdirPath); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to remove component workdir").
			WithContext("component", component).
			WithContext("stack", stack).
			WithContext("path", workdirPath).
			Err()
	}

	ui.Success(fmt.Sprintf("Cleaned workdir: %s", workdirPath))
	return nil
}

// CleanAllWorkdirs removes all working directories in the project.
func CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "workdir.CleanAllWorkdirs")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	workdirBase := filepath.Join(basePath, WorkdirPath)

	// Check if workdir base exists.
	if _, err := os.Stat(workdirBase); os.IsNotExist(err) {
		ui.Info("No workdirs found to clean")
		return nil
	}

	ui.Info("Cleaning all workdirs")

	if err := os.RemoveAll(workdirBase); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to remove all workdirs").
			WithContext("path", workdirBase).
			Err()
	}

	ui.Success(fmt.Sprintf("Cleaned all workdirs: %s", workdirBase))
	return nil
}

// CleanOptions configures what to clean.
type CleanOptions struct {
	// Component is the specific component to clean (empty for all).
	Component string

	// Stack is the stack name (required when Component is specified).
	Stack string

	// All cleans all workdirs in the project.
	All bool

	// Expired cleans only workdirs whose LastAccessed is older than TTL.
	Expired bool

	// TTL is the time-to-live duration for expired cleanup (e.g., "7d", "24h", "weekly").
	// Required when Expired is true.
	TTL string

	// DryRun shows what would be cleaned without actually deleting.
	DryRun bool
}

// ExpiredWorkdirInfo contains information about an expired workdir.
type ExpiredWorkdirInfo struct {
	// Path is the absolute path to the workdir.
	Path string
	// Name is the workdir name (e.g., "dev-vpc").
	Name string
	// LastAccessed is when the workdir was last accessed.
	LastAccessed time.Time
	// Age is how long ago the workdir was last accessed.
	Age time.Duration
}

// Clean performs cleanup based on the provided options.
func Clean(atmosConfig *schema.AtmosConfiguration, opts CleanOptions) error {
	defer perf.Track(atmosConfig, "workdir.Clean")()

	var errs []error

	// Clean workdirs.
	switch {
	case opts.Expired:
		if opts.TTL == "" {
			return errUtils.Build(errUtils.ErrWorkdirClean).
				WithExplanation("TTL is required when using --expired").
				WithHint("Specify a TTL like --ttl=7d or --ttl=24h").
				Err()
		}
		if err := CleanExpiredWorkdirs(atmosConfig, opts.TTL, opts.DryRun); err != nil {
			errs = append(errs, err)
		}
	case opts.All:
		if err := CleanAllWorkdirs(atmosConfig); err != nil {
			errs = append(errs, err)
		}
	case opts.Component != "" && opts.Stack != "":
		if err := CleanWorkdir(atmosConfig, opts.Component, opts.Stack); err != nil {
			errs = append(errs, err)
		}
	default:
		log.Debug("Clean called without --all flag or component/stack, no action taken")
	}

	if len(errs) > 0 {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(errors.Join(errs...)).
			WithExplanation(fmt.Sprintf("%d error(s) occurred during cleanup", len(errs))).
			Err()
	}

	return nil
}

// CleanExpiredWorkdirs removes workdirs whose LastAccessed is older than the specified TTL.
// If dryRun is true, it only reports what would be cleaned without actually deleting.
func CleanExpiredWorkdirs(atmosConfig *schema.AtmosConfiguration, ttl string, dryRun bool) error {
	defer perf.Track(atmosConfig, "workdir.CleanExpiredWorkdirs")()

	// Parse TTL duration.
	ttlDuration, err := duration.ParseDuration(ttl)
	if err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("invalid TTL format").
			WithContext("ttl", ttl).
			WithHint("Use formats like '7d', '24h', '30m', or keywords like 'daily', 'weekly'").
			Err()
	}

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	// Find all expired workdirs.
	expiredWorkdirs, err := findExpiredWorkdirs(basePath, ttlDuration)
	if err != nil {
		return err
	}

	if len(expiredWorkdirs) == 0 {
		ui.Info(fmt.Sprintf("No expired workdirs found (TTL: %s)", ttl))
		return nil
	}

	if dryRun {
		ui.Info(fmt.Sprintf("Dry run: would clean %d expired workdir(s) (TTL: %s):", len(expiredWorkdirs), ttl))
		for _, w := range expiredWorkdirs {
			ui.Info(fmt.Sprintf("  - %s (last accessed %s ago)", w.Name, formatDuration(w.Age)))
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Cleaning %d expired workdir(s) (TTL: %s)...", len(expiredWorkdirs), ttl))

	var errs []error
	cleaned := 0
	for _, w := range expiredWorkdirs {
		if err := os.RemoveAll(w.Path); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", w.Path, err))
			continue
		}
		ui.Success(fmt.Sprintf("Removed %s (last accessed %s ago)", w.Name, formatDuration(w.Age)))
		cleaned++
	}

	if len(errs) > 0 {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(errors.Join(errs...)).
			WithExplanation(fmt.Sprintf("cleaned %d workdirs but %d failed", cleaned, len(errs))).
			Err()
	}

	ui.Success(fmt.Sprintf("Cleaned %d expired workdir(s)", cleaned))
	return nil
}

// findExpiredWorkdirs finds all workdirs that haven't been accessed within the TTL.
func findExpiredWorkdirs(basePath string, ttl time.Duration) ([]ExpiredWorkdirInfo, error) {
	defer perf.Track(nil, "workdir.findExpiredWorkdirs")()

	cutoff := time.Now().Add(-ttl)
	workdirBase := filepath.Join(basePath, WorkdirPath, "terraform")

	if _, err := os.Stat(workdirBase); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(workdirBase)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to read workdir directory").
			WithContext("path", workdirBase).
			Err()
	}

	var expired []ExpiredWorkdirInfo
	for _, entry := range entries {
		if info := checkWorkdirExpiry(workdirBase, entry, cutoff); info != nil {
			expired = append(expired, *info)
		}
	}

	return expired, nil
}

// checkWorkdirExpiry checks if a single workdir entry is expired.
// Returns nil if the entry is not expired or not a valid workdir.
func checkWorkdirExpiry(workdirBase string, entry os.DirEntry, cutoff time.Time) *ExpiredWorkdirInfo {
	if !entry.IsDir() {
		return nil
	}

	workdirPath := filepath.Join(workdirBase, entry.Name())
	lastAccessed := getLastAccessedTime(workdirPath, entry)

	if lastAccessed.IsZero() || !lastAccessed.Before(cutoff) {
		return nil
	}

	return &ExpiredWorkdirInfo{
		Path:         workdirPath,
		Name:         entry.Name(),
		LastAccessed: lastAccessed,
		Age:          time.Since(lastAccessed),
	}
}

// getLastAccessedTime determines the last accessed time for a workdir.
// Uses metadata if available, otherwise falls back to file system modification time.
func getLastAccessedTime(workdirPath string, entry os.DirEntry) time.Time {
	metadata, err := ReadMetadata(workdirPath)
	if err != nil || metadata == nil {
		return getModTimeFromEntry(entry)
	}

	// Use LastAccessed from metadata, fall back to UpdatedAt, then CreatedAt.
	if !metadata.LastAccessed.IsZero() {
		return metadata.LastAccessed
	}
	if !metadata.UpdatedAt.IsZero() {
		return metadata.UpdatedAt
	}
	return metadata.CreatedAt
}

// getModTimeFromEntry gets the modification time from a directory entry.
func getModTimeFromEntry(entry os.DirEntry) time.Time {
	info, err := entry.Info()
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / hoursPerDay)
	hours := int(d.Hours()) % hoursPerDay
	minutes := int(d.Minutes()) % minutesPerHour

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "< 1m"
}
