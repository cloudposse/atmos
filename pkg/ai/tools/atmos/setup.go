package atmos

import (
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/lsp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RegisterTools registers all Atmos tools to the registry.
func RegisterTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration, lspManager client.ManagerInterface) error {
	if err := registerCoreTools(registry, atmosConfig); err != nil {
		return err
	}

	// Register LSP validation tool if LSP manager is available.
	if lspManager != nil {
		if err := registry.Register(NewValidateFileLSPTool(atmosConfig, lspManager)); err != nil {
			return err
		}
	}

	return registerWriteAndExecutionTools(registry, atmosConfig)
}

// registerCoreTools registers read-only, introspection, and search tools.
func registerCoreTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration) error {
	coreTools := []tools.Tool{
		NewReadComponentFileTool(atmosConfig),
		NewReadStackFileTool(atmosConfig),
		NewDescribeComponentTool(atmosConfig),
		NewListStacksTool(atmosConfig),
		NewDescribeAffectedTool(atmosConfig),
		NewValidateStacksTool(atmosConfig),
		NewReadFileTool(atmosConfig),
		NewSearchFilesTool(atmosConfig),
		NewListComponentFilesTool(atmosConfig),
		NewGetTemplateContextTool(atmosConfig),
	}
	return registerAll(registry, coreTools)
}

// registerWriteAndExecutionTools registers write, execution, and web search tools.
func registerWriteAndExecutionTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration) error {
	writeTools := []tools.Tool{
		NewWriteComponentFileTool(atmosConfig),
		NewWriteStackFileTool(atmosConfig),
		NewEditFileTool(atmosConfig),
		NewExecuteAtmosCommandTool(atmosConfig),
		NewExecuteBashCommandTool(atmosConfig),
		NewWebSearchTool(atmosConfig),
	}
	return registerAll(registry, writeTools)
}

// registerAll registers a slice of tools to the registry.
func registerAll(registry *tools.Registry, toolList []tools.Tool) error {
	for _, t := range toolList {
		if err := registry.Register(t); err != nil {
			return err
		}
	}
	return nil
}
