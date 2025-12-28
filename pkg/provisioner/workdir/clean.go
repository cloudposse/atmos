package workdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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
		_ = ui.Info(fmt.Sprintf("No workdir found for component '%s' in stack '%s'", component, stack))
		return nil
	}

	_ = ui.Info(fmt.Sprintf("Cleaning workdir for component '%s' in stack '%s'", component, stack))

	if err := os.RemoveAll(workdirPath); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("failed to remove component workdir").
			WithContext("component", component).
			WithContext("stack", stack).
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

// CleanOptions configures what to clean.
type CleanOptions struct {
	// Component is the specific component to clean (empty for all).
	Component string

	// Stack is the stack name (required when Component is specified).
	Stack string

	// All cleans all workdirs in the project.
	All bool
}

// Clean performs cleanup based on the provided options.
func Clean(atmosConfig *schema.AtmosConfiguration, opts CleanOptions) error {
	defer perf.Track(atmosConfig, "workdir.Clean")()

	var errs []error

	// Clean workdirs.
	switch {
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
