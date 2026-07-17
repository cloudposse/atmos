package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui"
)

type validationTaskStatus string

const (
	validationTaskPassed  validationTaskStatus = "passed"
	validationTaskFailed  validationTaskStatus = "failed"
	validationTaskSkipped validationTaskStatus = "skipped"
)

type validationTask struct {
	name       string
	applicable func() (bool, error)
	run        func() error
}

type validationTaskResult struct {
	name   string
	status validationTaskStatus
}

// runValidateAll runs every project-wide validation target and reports all outcomes.
func runValidateAll(cmd *cobra.Command) error {
	format, err := validationFormat(cmd)
	if err != nil {
		return err
	}
	if format == validateFormatRich {
		aggregateValidationFormat = validateFormatRich
		defer func() { aggregateValidationFormat = "" }()
		for _, child := range []*cobra.Command{ValidateSchemaCmd, ValidateStacksCmd, editorConfigCmd, validateCICmd} {
			flags := child.Flags()
			if flags.Lookup("format") == nil {
				flags = child.PersistentFlags()
			}
			if flag := flags.Lookup("format"); flag != nil {
				if err := flags.Set("format", validateFormatRich); err != nil {
					return err
				}
			}
		}
	}
	results, err := runValidationTasks([]validationTask{
		{
			name: "schema",
			applicable: func() (bool, error) {
				return true, nil
			},
			run: func() error {
				return runValidateSchema(ValidateSchemaCmd, nil)
			},
		},
		{
			name: "stacks",
			applicable: func() (bool, error) {
				return true, nil
			},
			run: func() error {
				return runValidateStacks(ValidateStacksCmd, nil)
			},
		},
		{
			name:       "editorconfig",
			applicable: editorConfigValidationApplicable,
			run: func() error {
				return runEditorConfig(editorConfigCmd)
			},
		},
		{
			name:       "ci",
			applicable: githubActionsValidationApplicable,
			run: func() error {
				return validateCICmd.RunE(validateCICmd, nil)
			},
		},
	})

	ui.Writeln(formatValidationSummary(results))
	return err
}

// runValidationTasks executes applicable tasks in declaration order and does not stop at failures.
func runValidationTasks(tasks []validationTask) ([]validationTaskResult, error) {
	results := make([]validationTaskResult, 0, len(tasks))
	failures := make([]error, 0)

	for _, task := range tasks {
		applicable, err := task.applicable()
		if err != nil {
			results = append(results, validationTaskResult{name: task.name, status: validationTaskFailed})
			failures = append(failures, validationTaskError(task.name, err))
			continue
		}
		if !applicable {
			results = append(results, validationTaskResult{name: task.name, status: validationTaskSkipped})
			continue
		}

		if err := task.run(); err != nil {
			results = append(results, validationTaskResult{name: task.name, status: validationTaskFailed})
			failures = append(failures, validationTaskError(task.name, err))
			continue
		}

		results = append(results, validationTaskResult{name: task.name, status: validationTaskPassed})
	}

	if len(failures) == 0 {
		return results, nil
	}

	return results, errUtils.WithExitCode(errors.Join(append([]error{errUtils.ErrValidationFailed}, failures...)...), 1)
}

func validationTaskError(name string, err error) error {
	return fmt.Errorf("%w: %s: %w", errUtils.ErrValidationFailed, name, err)
}

func formatValidationSummary(results []validationTaskResult) string {
	var builder strings.Builder
	builder.WriteString("Validation summary:\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf("  %s: %s\n", result.name, result.status))
	}
	return strings.TrimSuffix(builder.String(), "\n")
}

func editorConfigValidationApplicable() (bool, error) {
	paths := defaultConfigFileNames
	if len(atmosConfig.Validate.EditorConfig.ConfigFilePaths) > 0 {
		paths = atmosConfig.Validate.EditorConfig.ConfigFilePaths
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return true, nil
			}
			continue
		}
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("%w: inspect EditorConfig file %q: %w", errUtils.ErrValidationFailed, path, err)
		}
	}

	return false, nil
}

func githubActionsValidationApplicable() (bool, error) {
	entries, err := os.ReadDir(filepath.Join(".github", "workflows"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: inspect GitHub Actions workflows: %w", errUtils.ErrValidationFailed, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yaml" || extension == ".yml" {
			return true, nil
		}
	}

	return false, nil
}
