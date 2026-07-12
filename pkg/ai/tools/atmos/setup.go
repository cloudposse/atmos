package atmos

import (
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/lsp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RegisterTools registers Atmos tools to the registry. When atmosConfig.AI.Tools.Allowed is
// non-empty, only tools matching one of those name patterns are registered — every other tool
// simply doesn't exist for this session (it won't appear in tools/list or atmos ai chat's tool
// set). An empty/unset Allowed list registers every tool, unchanged from prior behavior.
func RegisterTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration, lspManager client.ManagerInterface) error {
	if err := registerCoreTools(registry, atmosConfig); err != nil {
		return err
	}

	// Register LSP validation tool if LSP manager is available and allowed.
	if lspManager != nil && isToolAllowed(atmosConfig, "validate_file_lsp") {
		if err := registry.Register(NewValidateFileLSPTool(atmosConfig, lspManager)); err != nil {
			return err
		}
	}

	return registerWriteAndExecutionTools(registry, atmosConfig)
}

// isToolAllowed returns true if toolName should be registered: either no allow-list is
// configured (everything registers), or toolName matches one of its patterns.
func isToolAllowed(atmosConfig *schema.AtmosConfiguration, toolName string) bool {
	allowed := atmosConfig.AI.Tools.Allowed
	if len(allowed) == 0 {
		return true
	}
	for _, pattern := range allowed {
		if permission.MatchesPattern(toolName, pattern) {
			return true
		}
	}
	return false
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
		NewTerraformComponentHCLGetTool(atmosConfig),
		NewListFindingsTool(atmosConfig),
		NewDescribeFindingTool(atmosConfig),
		NewAnalyzeFindingTool(atmosConfig),
		NewComplianceReportTool(atmosConfig),
		NewConfigGetTool(atmosConfig),
		NewConfigListTool(atmosConfig),
		NewStackConfigGetTool(atmosConfig),
		NewStackConfigListTool(atmosConfig),
		NewVendorConfigGetTool(atmosConfig),
		NewVendorConfigListTool(atmosConfig),
		NewListCommandsTool(atmosConfig),
		NewCommandHelpTool(atmosConfig),
		NewDescribeDependentsTool(atmosConfig),
		NewDescribeWorkflowsTool(atmosConfig),
		NewListWorkflowsTool(atmosConfig),
		NewListComponentsTool(atmosConfig),
		NewListValuesTool(atmosConfig),
		NewAuthWhoamiTool(atmosConfig),
		NewAuthListTool(atmosConfig),
		NewToolchainListTool(atmosConfig),
		NewSecretListTool(atmosConfig),
		NewValidateComponentTool(atmosConfig),
	}
	return registerAll(registry, atmosConfig, coreTools)
}

// registerWriteAndExecutionTools registers write, execution, and web search tools.
func registerWriteAndExecutionTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration) error {
	writeTools := []tools.Tool{
		NewWriteComponentFileTool(atmosConfig),
		NewTerraformComponentHCLEditTool(atmosConfig),
		NewWriteStackFileTool(atmosConfig),
		NewEditFileTool(atmosConfig),
		NewExecuteAtmosCommandTool(atmosConfig),
		NewExecuteBashCommandTool(atmosConfig),
		NewWebSearchTool(atmosConfig),
		NewConfigSetTool(atmosConfig),
		NewConfigDeleteTool(atmosConfig),
		NewConfigFormatTool(atmosConfig),
		NewStackConfigSetTool(atmosConfig),
		NewStackConfigDeleteTool(atmosConfig),
		NewStackConfigFormatTool(atmosConfig),
		NewVendorConfigSetTool(atmosConfig),
		NewVendorConfigDeleteTool(atmosConfig),
		NewVendorConfigFormatTool(atmosConfig),
	}
	return registerAll(registry, atmosConfig, writeTools)
}

// registerAll registers every tool in toolList that passes isToolAllowed to the registry.
func registerAll(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration, toolList []tools.Tool) error {
	for _, t := range toolList {
		if !isToolAllowed(atmosConfig, t.Name()) {
			continue
		}
		if err := registry.Register(t); err != nil {
			return err
		}
	}
	return nil
}
