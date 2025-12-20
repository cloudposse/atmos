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
	return h.ValidateRequired(step, "prompt", step.Prompt)
}

// Execute prompts for file selection and returns the chosen path.
func (h *FileHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	if err := h.CheckTTY(step); err != nil {
		return nil, err
	}

	prompt, err := h.ResolvePrompt(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve starting path if present.
	startPath := step.Path
	if startPath == "" {
		startPath = "."
	} else {
		startPath, err = vars.Resolve(startPath)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
		}
	}

	// Convert to absolute path.
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
	}

	// Build list of files.
	var files []string
	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access.
		}
		if info.IsDir() {
			return nil // Skip directories.
		}

		// Filter by extensions if specified.
		if len(step.Extensions) > 0 {
			ext := filepath.Ext(path)
			matched := false
			for _, e := range step.Extensions {
				// Normalize extension (with or without dot).
				if !strings.HasPrefix(e, ".") {
					e = "." + e
				}
				if strings.EqualFold(ext, e) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Make path relative to starting directory for display.
		relPath, err := filepath.Rel(absPath, path)
		if err != nil {
			relPath = path
		}
		files = append(files, relPath)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to scan directory: %w", step.Name, err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("step '%s': no files found matching criteria in %s", step.Name, absPath)
	}

	// Create custom keymap that adds ESC to quit keys.
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
			return nil, errUtils.ErrUserAborted
		}
		return nil, fmt.Errorf("step '%s': file selection failed: %w", step.Name, err)
	}

	// Return full path.
	fullPath := filepath.Join(absPath, choice)
	return NewStepResult(fullPath), nil
}
