package atmos

import (
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/lsp"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RegisterTools registers all Atmos tools to the registry.
func RegisterTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration, lspManager lsp.ManagerInterface) error {
	// Register component file tools.
	if err := registry.Register(NewReadComponentFileTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewWriteComponentFileTool(atmosConfig)); err != nil {
		return err
	}

	// Register stack file tools.
	if err := registry.Register(NewReadStackFileTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewWriteStackFileTool(atmosConfig)); err != nil {
		return err
	}

	// Register component and stack tools.
	if err := registry.Register(NewDescribeComponentTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewListStacksTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewDescribeAffectedTool(atmosConfig)); err != nil {
		return err
	}

	// Register validation tools.
	if err := registry.Register(NewValidateStacksTool(atmosConfig)); err != nil {
		return err
	}

	// Register LSP validation tool if LSP manager is available.
	if lspManager != nil {
		if err := registry.Register(NewValidateFileLSPTool(atmosConfig, lspManager)); err != nil {
			return err
		}
	}

	// Register file operation tools.
	if err := registry.Register(NewReadFileTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewEditFileTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewSearchFilesTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewListComponentFilesTool(atmosConfig)); err != nil {
		return err
	}

	// Register command execution and template tools.
	if err := registry.Register(NewExecuteAtmosCommandTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewExecuteBashCommandTool(atmosConfig)); err != nil {
		return err
	}
	if err := registry.Register(NewGetTemplateContextTool(atmosConfig)); err != nil {
		return err
	}

	return nil
}
