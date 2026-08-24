package atmos

import (
	"bytes"
	"context"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	hcl "github.com/cloudposse/atmos/pkg/hcl"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HCLEditOperation represents the type of Terraform HCL edit operation.
type HCLEditOperation string

const (
	// HCLOperationAttributeSet sets an existing attribute's value. Silently
	// does nothing if the attribute doesn't already exist (hcledit semantics).
	HCLOperationAttributeSet HCLEditOperation = "attribute_set"
	// HCLOperationAttributeAppend adds a new attribute. Errors if it already exists.
	HCLOperationAttributeAppend HCLEditOperation = "attribute_append"
	// HCLOperationAttributeRemove removes an attribute.
	HCLOperationAttributeRemove HCLEditOperation = "attribute_remove"
	// HCLOperationBlockNew creates a new empty block.
	HCLOperationBlockNew HCLEditOperation = "block_new"
	// HCLOperationBlockAppend appends a new child block inside every block matching parent's address.
	HCLOperationBlockAppend HCLEditOperation = "block_append"
	// HCLOperationBlockRemove removes every block matching an address.
	HCLOperationBlockRemove HCLEditOperation = "block_remove"
)

// TerraformComponentHCLEditTool structurally edits a Terraform component
// file using hcledit, preserving comments and formatting.
type TerraformComponentHCLEditTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewTerraformComponentHCLEditTool creates a new Terraform component HCL editor tool.
func NewTerraformComponentHCLEditTool(atmosConfig *schema.AtmosConfiguration) *TerraformComponentHCLEditTool {
	return &TerraformComponentHCLEditTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *TerraformComponentHCLEditTool) Name() string {
	return "atmos_terraform_component_hcl_edit"
}

// Description returns the tool description.
func (t *TerraformComponentHCLEditTool) Description() string {
	return "Structurally edit a Terraform component file using hcledit, preserving comments and formatting: " +
		"attribute_set, attribute_append, attribute_remove, block_new, block_append, block_remove. " +
		"attribute_set silently does nothing if the attribute does not already exist -- use attribute_append to create one. " +
		"block_append and block_remove apply to every block matching the given address, not just the first match. " +
		"Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *TerraformComponentHCLEditTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Relative path to the .tf file within the Terraform components directory (e.g. 'vpc/main.tf').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name: "operation",
			Description: "Edit operation: 'attribute_set', 'attribute_append', 'attribute_remove', " +
				"'block_new', 'block_append', 'block_remove'.",
			Type:     tools.ParamTypeString,
			Required: true,
		},
		{
			Name: paramAddress,
			Description: "hcledit address the operation targets. For attribute_set/attribute_append/attribute_remove: the " +
				"attribute address (e.g. 'resource.aws_instance.web.instance_type'). For block_new: the new block's address " +
				"including labels (e.g. 'resource.aws_instance.web'). For block_remove: the block address to remove -- if it " +
				"matches more than one block, ALL matching blocks are removed.",
			Type:     tools.ParamTypeString,
			Required: false,
		},
		{
			Name: "value",
			Description: "For attribute_set/attribute_append: the new value as a raw HCL expression. Quote string values " +
				`yourself, e.g. "t2.micro"; leave numbers/bools/lists unquoted, e.g. 3, true, ["a","b"].`,
			Type:     tools.ParamTypeString,
			Required: false,
		},
		{
			Name: "parent",
			Description: "For block_append: the parent block address to append into (e.g. 'resource.aws_instance.web'). " +
				"If it matches multiple blocks, the child is appended to ALL of them.",
			Type:     tools.ParamTypeString,
			Required: false,
		},
		{
			Name:        "child",
			Description: "For block_append: the new child block's type and labels, relative to parent (e.g. 'lifecycle').",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "newline",
			Description: "For attribute_append/block_new/block_append: insert a blank line before the new attribute/block.",
			Type:        tools.ParamTypeBool,
			Required:    false,
			Default:     true,
		},
	}
}

// Execute applies the requested HCL edit operation to the component file.
func (t *TerraformComponentHCLEditTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	filePath, err := extractRequiredStringParam(params, "file_path")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	operationStr, err := extractRequiredStringParam(params, "operation")
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	operation := HCLEditOperation(operationStr)

	cleanPath, err := resolveTerraformComponentFilePath(t.atmosConfig, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	before, err := readAndValidateFile(cleanPath, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	log.Debugf("Editing Terraform component HCL: %s (%s)", filePath, operation)

	if err := applyHCLOperation(cleanPath, operation, params); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	after, err := os.ReadFile(cleanPath)
	if err != nil {
		err = fmt.Errorf("%w: %w", errUtils.ErrAIFileNotFound, err)
		return &tools.Result{Success: false, Error: err}, err
	}

	changed := !bytes.Equal(before, after)
	output := fmt.Sprintf("Successfully applied %s to %s", operation, filePath)
	if !changed {
		output = fmt.Sprintf("No matching address found for %s in %s; nothing was changed", operation, filePath)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file_path": filePath,
			"operation": string(operation),
			"changed":   changed,
		},
	}, nil
}

// paramAddress is the hcledit address parameter name, shared by several
// operations below.
const paramAddress = "address"

// applyHCLOperation dispatches to the pkg/hcl *File function matching operation.
func applyHCLOperation(cleanPath string, operation HCLEditOperation, params map[string]interface{}) error {
	switch operation {
	case HCLOperationAttributeSet:
		return applyAttributeSet(cleanPath, params)
	case HCLOperationAttributeAppend:
		return applyAttributeAppend(cleanPath, params)
	case HCLOperationAttributeRemove:
		return applyAttributeRemove(cleanPath, params)
	case HCLOperationBlockNew:
		return applyBlockNew(cleanPath, params)
	case HCLOperationBlockAppend:
		return applyBlockAppend(cleanPath, params)
	case HCLOperationBlockRemove:
		return applyBlockRemove(cleanPath, params)
	default:
		return fmt.Errorf("%w: %s", errUtils.ErrAIUnknownOperation, string(operation))
	}
}

// extractNewline reads the optional newline parameter, defaulting to true.
func extractNewline(params map[string]interface{}) bool {
	newlineParam := true
	if v, ok := params["newline"].(bool); ok {
		newlineParam = v
	}
	return newlineParam
}

// extractAddressAndValue extracts the address and value parameters shared by
// attribute_set and attribute_append.
func extractAddressAndValue(params map[string]interface{}) (address, value string, err error) {
	address, err = extractRequiredStringParam(params, paramAddress)
	if err != nil {
		return "", "", err
	}
	value, err = extractRequiredStringParam(params, "value")
	if err != nil {
		return "", "", err
	}
	return address, value, nil
}

func applyAttributeSet(cleanPath string, params map[string]interface{}) error {
	address, value, err := extractAddressAndValue(params)
	if err != nil {
		return err
	}
	return hcl.SetAttributeFile(cleanPath, address, value)
}

func applyAttributeAppend(cleanPath string, params map[string]interface{}) error {
	address, value, err := extractAddressAndValue(params)
	if err != nil {
		return err
	}
	return hcl.AppendAttributeFile(cleanPath, address, value, extractNewline(params))
}

func applyAttributeRemove(cleanPath string, params map[string]interface{}) error {
	address, err := extractRequiredStringParam(params, paramAddress)
	if err != nil {
		return err
	}
	return hcl.RemoveAttributeFile(cleanPath, address)
}

func applyBlockNew(cleanPath string, params map[string]interface{}) error {
	address, err := extractRequiredStringParam(params, paramAddress)
	if err != nil {
		return err
	}
	return hcl.NewBlockFile(cleanPath, address, extractNewline(params))
}

func applyBlockAppend(cleanPath string, params map[string]interface{}) error {
	parent, err := extractRequiredStringParam(params, "parent")
	if err != nil {
		return err
	}
	child, err := extractRequiredStringParam(params, "child")
	if err != nil {
		return err
	}
	return hcl.AppendBlockFile(cleanPath, parent, child, extractNewline(params))
}

func applyBlockRemove(cleanPath string, params map[string]interface{}) error {
	address, err := extractRequiredStringParam(params, paramAddress)
	if err != nil {
		return err
	}
	return hcl.RemoveBlockFile(cleanPath, address)
}

// RequiresPermission returns true if this tool needs permission.
func (t *TerraformComponentHCLEditTool) RequiresPermission() bool {
	return true // File modification requires user confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *TerraformComponentHCLEditTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
