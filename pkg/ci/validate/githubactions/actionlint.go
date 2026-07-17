// Package githubactions implements GitHub Actions workflow validation.
package githubactions

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/rhysd/actionlint"

	civalidate "github.com/cloudposse/atmos/pkg/ci/validate"
	"github.com/cloudposse/atmos/pkg/validation"
)

// ValidatorName is the registry name for GitHub Actions workflow validation.
const ValidatorName = "github-actions"

// Validator validates GitHub Actions workflow files with actionlint.
type Validator struct{}

func init() {
	civalidate.Register(Validator{})
}

// Name returns this validator's registry name.
func (Validator) Name() string { return ValidatorName }

// Validate checks the requested workflow files or workflow path. When neither
// is given, it checks the default .github/workflows path under Root. ShellCheck
// and Pyflakes are disabled intentionally: they are external tools and would
// make this built-in command depend on the host environment.
func (Validator) Validate(_ context.Context, request civalidate.Request) (validation.Report, error) {
	var renderedDiagnostics bytes.Buffer
	linter, err := actionlint.NewLinter(&renderedDiagnostics, &actionlint.LinterOptions{
		Color:      actionlint.ColorOptionKindNever,
		Shellcheck: "",
		Pyflakes:   "",
		WorkingDir: request.Root,
	})
	if err != nil {
		return validation.Report{}, err
	}

	report := validation.Report{Target: request.Root}
	var errors []*actionlint.Error
	if request.WorkflowPath != "" {
		report.Target = request.WorkflowPath
		report.FilesChecked, err = workflowFileCount(request.WorkflowPath)
		if err != nil {
			return validation.Report{}, err
		}
		errors, err = linter.LintDir(request.WorkflowPath, nil)
	} else if len(request.Paths) == 0 {
		workflowPath := filepath.Join(request.Root, ".github", "workflows")
		report.Target = workflowPath
		report.FilesChecked, err = workflowFileCount(workflowPath)
		if err != nil {
			return validation.Report{}, err
		}
		errors, err = linter.LintDir(workflowPath, nil)
	} else {
		report.FilesChecked = len(request.Paths)
		errors, err = linter.LintFiles(request.Paths, nil)
	}
	if err != nil {
		return validation.Report{}, err
	}

	report.Diagnostics = make([]validation.Diagnostic, 0, len(errors))
	for _, lintErr := range errors {
		ruleID := lintErr.Kind
		if ruleID == "" {
			ruleID = "actionlint"
		}
		report.Diagnostics = append(report.Diagnostics, validation.Diagnostic{
			Source:   ValidatorName,
			RuleID:   ruleID,
			Severity: validation.SeverityError,
			Message:  lintErr.Message,
			File:     repositoryPath(lintErr.Filepath),
			Line:     lintErr.Line,
			Column:   lintErr.Column,
		})
	}
	report.RenderedDiagnostics = strings.TrimSpace(renderedDiagnostics.String())
	return report, nil
}

// repositoryPath converts an OS-native path into the slash-separated form used
// by GitHub Actions annotations and repository-relative diagnostics.
func repositoryPath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

func workflowFileCount(dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".yml" || extension == ".yaml" {
			count++
		}
		return nil
	})
	return count, err
}
