package workdir

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// CleanWorkdir removes the working directory for a specific component.
func CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component string) error {
	defer perf.Track(atmosConfig, "workdir.CleanWorkdir")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	workdirPath := filepath.Join(basePath, WorkdirPath, "terraform", component)

	// Check if workdir exists.
	if _, err := os.Stat(workdirPath); os.IsNotExist(err) {
		_ = ui.Info(fmt.Sprintf("No workdir found for component '%s'", component))
		return nil
	}

	_ = ui.Info(fmt.Sprintf("Cleaning workdir for component '%s'", component))

	if err := os.RemoveAll(workdirPath); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to remove component workdir").
			WithContext("component", component).
			WithContext("path", workdirPath).
			Err()
	}

	_ = ui.Success(fmt.Sprintf("Cleaned workdir: %s", workdirPath))
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
		_ = ui.Info("No workdirs found to clean")
		return nil
	}

	_ = ui.Info("Cleaning all workdirs")

	if err := os.RemoveAll(workdirBase); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to remove all workdirs").
			WithContext("path", workdirBase).
			Err()
	}

	_ = ui.Success(fmt.Sprintf("Cleaned all workdirs: %s", workdirBase))
	return nil
}

// CleanSourceCache removes the XDG source cache.
func CleanSourceCache() error {
	defer perf.Track(nil, "workdir.CleanSourceCache")()

	cache := NewDefaultCache()

	_ = ui.Info("Cleaning source cache")

	if err := cache.Clear(); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to clear source cache").
			Err()
	}

	_ = ui.Success("Cleaned source cache")
	return nil
}

// CleanOptions configures what to clean.
type CleanOptions struct {
	// Component is the specific component to clean (empty for all).
	Component string

	// All cleans all workdirs in the project.
	All bool

	// Cache cleans the XDG source cache.
	Cache bool
}

// Clean performs cleanup based on the provided options.
func Clean(atmosConfig *schema.AtmosConfiguration, opts CleanOptions) error {
	defer perf.Track(atmosConfig, "workdir.Clean")()

	var errs []error

	// Clean cache if requested.
	if opts.Cache {
		if err := CleanSourceCache(); err != nil {
			errs = append(errs, err)
		}
	}

	// Clean workdirs.
	if opts.All {
		if err := CleanAllWorkdirs(atmosConfig); err != nil {
			errs = append(errs, err)
		}
	} else if opts.Component != "" {
		if err := CleanWorkdir(atmosConfig, opts.Component); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithExplanation(fmt.Sprintf("%d error(s) occurred during cleanup", len(errs))).
			Err()
	}

	return nil
}
