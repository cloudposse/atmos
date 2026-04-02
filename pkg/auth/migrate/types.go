// Package migrate provides a framework for auth migration operations.
package migrate

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
)

// StepStatus represents the current state of a migration step.
type StepStatus int

const (
	// StepNeeded indicates the migration step needs to be applied.
	StepNeeded StepStatus = iota
	// StepComplete indicates the migration step is already done.
	StepComplete
	// StepNotApplicable indicates the step does not apply to this project.
	StepNotApplicable
)

// String returns a human-readable label for the step status.
func (s StepStatus) String() string {
	switch s {
	case StepNeeded:
		return "needed"
	case StepComplete:
		return "already complete"
	case StepNotApplicable:
		return "not applicable"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Change represents a single file change proposed by a migration step.
type Change struct {
	// FilePath is the path to the file being modified or created.
	FilePath string
	// Description is a human-readable summary of the change.
	Description string
	// Detail is an optional diff or preview of the change content.
	Detail string
}

// StepResult holds the detection and planning output for one step.
type StepResult struct {
	// Step is the migration step that produced this result.
	Step MigrationStep
	// Status is the detected status of the step.
	Status StepStatus
	// Changes are the planned changes (empty if Status != StepNeeded).
	Changes []Change
	// Error is any error encountered during detection or planning.
	Error error
}

// MigrationStep defines a single discrete migration operation.
type MigrationStep interface {
	// Name returns a short identifier for the step (e.g., "configure-provider").
	Name() string
	// Description returns a human-readable description shown to the user.
	Description() string
	// Detect checks if this step needs to run, is already complete, or doesn't apply.
	Detect(ctx context.Context) (StepStatus, error)
	// Plan returns the list of changes this step would make. Only called if Detect returns StepNeeded.
	Plan(ctx context.Context) ([]Change, error)
	// Apply executes the migration step, writing files in place.
	Apply(ctx context.Context) error
}

// MigrationContext holds shared data for all migration steps.
type MigrationContext struct {
	// AtmosConfig is the loaded atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// AccountMap maps account names to account IDs.
	AccountMap map[string]string
	// SSOConfig holds discovered SSO provider and group assignment data.
	SSOConfig *SSOConfig
	// StacksBasePath is the resolved stacks directory.
	StacksBasePath string
	// ProfilesPath is where to write profile directories.
	ProfilesPath string
	// ExistingAuth is the current auth config (may be nil).
	ExistingAuth *schema.AuthConfig
	// AtmosConfigPath is the path to atmos.yaml.
	AtmosConfigPath string
}

// SSOConfig holds SSO provider details and group-to-account permission assignments.
type SSOConfig struct {
	// StartURL is the AWS SSO start URL.
	StartURL string
	// Region is the AWS SSO region.
	Region string
	// ProviderName is the name for the SSO provider in atmos config.
	ProviderName string
	// AccountAssignments maps group -> permission-set -> []account-names.
	AccountAssignments map[string]map[string][]string
}

// FileSystem abstracts file operations for testability.
type FileSystem interface {
	// ReadFile reads the contents of a file.
	ReadFile(path string) ([]byte, error)
	// WriteFile writes content to a file, creating parent directories as needed.
	WriteFile(path string, data []byte, perm os.FileMode) error
	// Exists returns true if the path exists.
	Exists(path string) bool
	// Glob returns file paths matching a pattern.
	Glob(pattern string) ([]string, error)
	// Remove removes a file or empty directory.
	Remove(path string) error
}

// Prompter abstracts interactive user prompts for testability.
type Prompter interface {
	// Confirm shows a yes/no prompt and returns the user's choice.
	Confirm(title string) (bool, error)
	// Select shows a list of options and returns the selected value.
	Select(title string, options []string) (string, error)
	// Input shows a text input prompt and returns the entered value.
	Input(title, defaultValue string) (string, error)
	// SelectAction shows the "apply all / step by step / cancel" prompt.
	SelectAction() (Action, error)
}

// Action represents the user's choice for how to apply changes.
type Action int

const (
	// ActionApplyAll applies all needed steps without individual confirmation.
	ActionApplyAll Action = iota
	// ActionStepByStep walks through each step with yes/no confirmation.
	ActionStepByStep
	// ActionCancel aborts the migration.
	ActionCancel
)
