package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	cicmd "github.com/cloudposse/atmos/cmd/ci"
	errUtils "github.com/cloudposse/atmos/errors"
	ci "github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
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

var validationCISummaryWriter = ci.WriteStepSummary

// runValidateAll runs every project-wide validation target and reports all outcomes.
func runValidateAll(cmd *cobra.Command) error {
	affectedFiles, affected, err := validationAffectedFiles(cmd)
	if err != nil {
		return err
	}
	excludes, err := validationExcludePatterns(cmd)
	if err != nil {
		return err
	}
	schemaFiles, schemaValidateAll := affectedSchemaFiles(affectedFiles, "")
	editorConfigFiles, editorConfigValidateAll := affectedEditorConfigFiles(affectedFiles)
	workflowFiles := affectedWorkflowPaths(affectedFiles)
	workflowValidateAll := affectedWorkflowConfigChanged(affectedFiles)

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
				return !affected || schemaValidateAll || len(schemaFiles) > 0, nil
			},
			run: func() error {
				return runValidateSchemaForFiles(ValidateSchemaCmd, nil, affectedFiles, affected, excludes)
			},
		},
		{
			name: "stacks",
			applicable: func() (bool, error) {
				return !affected || affectedStacksApplicable(affectedFiles), nil
			},
			run: func() error {
				return runValidateStacksForFiles(ValidateStacksCmd, nil, affectedFiles, affected, excludes)
			},
		},
		{
			name: "editorconfig",
			applicable: func() (bool, error) {
				if affected && !editorConfigValidateAll && len(editorConfigFiles) == 0 {
					return false, nil
				}
				return editorConfigValidationApplicable()
			},
			run: func() error {
				if affected && !editorConfigValidateAll {
					return runEditorConfigForFiles(editorConfigCmd, editorConfigFiles, excludes)
				}
				return runEditorConfigForFiles(editorConfigCmd, nil, excludes)
			},
		},
		{
			name: "ci",
			applicable: func() (bool, error) {
				if affected && !workflowValidateAll && len(workflowFiles) == 0 {
					return false, nil
				}
				return githubActionsValidationApplicable()
			},
			run: func() error {
				if affected && !workflowValidateAll {
					return cicmd.RunValidateFilesExcluding(validateCICmd, workflowFiles, excludes)
				}
				return cicmd.RunValidateFilesExcluding(validateCICmd, nil, excludes)
			},
		},
	})

	summary := formatValidationSummary(results)
	ui.Writeln(summary)
	writeValidationCISummary(results)
	return err
}

func writeValidationCISummary(results []validationTaskResult) {
	if !ci.Enabled(&atmosConfig) {
		return
	}
	if err := validationCISummaryWriter(formatValidationSummaryMarkdown(results)); err != nil {
		log.Warn("Failed to write validation CI summary", "error", err)
	}
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

func formatValidationSummaryMarkdown(results []validationTaskResult) string {
	var builder strings.Builder
	builder.WriteString("## Atmos validation\n\n")
	builder.WriteString("| Validator | Result |\n| --- | --- |\n")
	for _, result := range results {
		icon := "✅"
		switch result.status {
		case validationTaskFailed:
			icon = "❌"
		case validationTaskSkipped:
			icon = "⏭️"
		}
		_, _ = fmt.Fprintf(&builder, "| %s | %s %s |\n", result.name, icon, result.status)
	}
	return builder.String()
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
	// Windows reports a child of a regular file as not found, whereas Unix
	// reports ENOTDIR. Validate the parent explicitly so a malformed `.github`
	// path is never mistaken for an absent optional workflows directory.
	if info, err := os.Stat(".github"); err == nil && !info.IsDir() {
		return false, fmt.Errorf("%w: inspect GitHub Actions workflows: .github is not a directory", errUtils.ErrValidationFailed)
	}

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
