package scaffold

import (
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	cfg "github.com/cloudposse/atmos/pkg/project/config"
)

// ProductionTemplateExecutor implements TemplateExecutor using the real template engine.
type ProductionTemplateExecutor struct{}

// NewProductionTemplateExecutor creates a new production template executor.
func NewProductionTemplateExecutor(_ *setup.GeneratorContext) TemplateExecutor {
	return &ProductionTemplateExecutor{}
}

// Generate executes the template generation.
//
//nolint:gocritic // hugeParam: interface signature requires value type for compatibility
func (e *ProductionTemplateExecutor) Generate(
	config templates.Configuration,
	targetDir string,
	force bool,
	values map[string]interface{},
) error {
	// Create template processor.
	processor := engine.NewProcessor()

	// Process all files.
	for _, file := range config.Files {
		// Skip scaffold.yaml file itself.
		if file.Path == cfg.ScaffoldConfigFileName {
			continue
		}

		// Convert templates.File to engine.File for processing.
		engineFile := engine.File{
			Path:        file.Path,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Permissions: file.Permissions,
		}

		// Process the file.
		if err := processor.ProcessFile(engineFile, targetDir, force, false, config, values); err != nil {
			return err
		}
	}

	return nil
}

// ValidateFiles validates template files.
func (e *ProductionTemplateExecutor) ValidateFiles(files []templates.File) error {
	// TODO: Implement validation logic if needed.
	return nil
}
