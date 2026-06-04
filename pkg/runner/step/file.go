package step

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// FileHandler prompts for file selection.
type FileHandler struct {
	BaseHandler
}

func init() {
	Register(&FileHandler{
		BaseHandler: NewBaseHandler("file", CategoryInteractive, true),
	})
}

// Validate checks that the step has required fields.
func (h *FileHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.FileHandler.Validate")()

	return h.ValidateRequired(step, "prompt", step.Prompt)
}

// Execute prompts for file selection and returns the chosen path.
func (h *FileHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.FileHandler.Execute")()

	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	absPath, err := h.resolveStartPath(step, vars)
	if err != nil {
		return nil, err
	}

	files, err := h.collectFiles(step, absPath)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to scan directory: %w", step.Name, err)
	}

	if len(files) == 0 {
		return nil, errUtils.Build(errUtils.ErrStepNoFilesFound).
			WithContext("step", step.Name).
			WithContext("path", absPath).
			Err()
	}

	choice, err := h.promptForFile(prompt, files)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			return nil, err
		}
		return nil, fmt.Errorf("step '%s': file selection failed: %w", step.Name, err)
	}

	fullPath := filepath.Join(absPath, choice)
	return NewStepResult(fullPath), nil
}

// resolveStartPath resolves and validates the starting path for file scanning.
func (h *FileHandler) resolveStartPath(step *schema.WorkflowStep, vars *Variables) (string, error) {
	startPath := step.Path
	if startPath == "" {
		startPath = "."
	} else {
		var err error
		startPath, err = vars.Resolve(startPath)
		if err != nil {
			return "", fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
		}
	}

	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
	}
	return absPath, nil
}

// collectFiles walks the directory and collects matching files.
func (h *FileHandler) collectFiles(step *schema.WorkflowStep, absPath string) ([]string, error) {
	var files []string
	err := filepath.Walk(absPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // Skip files we can't access, continue walking.
		}
		if info.IsDir() {
			return nil
		}

		if !h.matchesExtensions(path, step.Extensions) {
			return nil
		}

		relPath, err := filepath.Rel(absPath, path)
		if err != nil {
			relPath = path
		}
		files = append(files, relPath)
		return nil
	})
	return files, err
}

// matchesExtensions checks if the file matches the allowed extensions.
func (h *FileHandler) matchesExtensions(path string, extensions []string) bool {
	if len(extensions) == 0 {
		return true
	}

	ext := filepath.Ext(path)
	for _, e := range extensions {
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

// promptForFile displays the file selection UI.
func (h *FileHandler) promptForFile(prompt string, files []string) (string, error) {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(prompt).
				Description("Press ctrl+c or esc to cancel. Type to filter.").
				Options(huh.NewOptions(files...)...).
				Filtering(true).
				Value(&choice),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", err
	}

	return choice, nil
}
