package hooks

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// init registers the built-in `store` kind with the kind registry. This is
// the same dispatch the pre-rename `command: store` YAML used, just routed
// through the kind registry instead of a hardcoded switch in RunAll.
func init() {
	if err := RegisterKind(&Kind{
		Name:   "store",
		Engine: storeEngine{},
	}); err != nil {
		panic("failed to register built-in store kind: " + err.Error())
	}
}

// storeEngine adapts the existing StoreCommand to the Engine interface.
// It produces no structured Output (store hooks write directly to a
// configured store backend), so Run returns (nil, err).
type storeEngine struct{}

// Run satisfies Engine. It instantiates a StoreCommand bound to the active
// AtmosConfiguration/Info and delegates to the existing RunE logic.
func (storeEngine) Run(ctx *ExecContext) (*Output, error) {
	sc, err := NewStoreCommand(ctx.AtmosConfig, ctx.Info)
	if err != nil {
		return nil, err
	}
	return nil, sc.RunE(ctx.Hook, ctx.Event, ctx.Cmd, ctx.Args)
}

// TerraformOutputGetter retrieves terraform outputs.
// This enables dependency injection for testing.
// Returns:
//   - value: The output value (may be nil if the output exists but has a null value)
//   - exists: Whether the output key exists in the terraform outputs
//   - error: Any error that occurred during retrieval (SDK errors, network issues, etc.)
type TerraformOutputGetter func(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error)

// CustomOutputGetter retrieves an output for a custom-component-type apply.
// Custom components don't have terraform state; their step list publishes
// values to a file pointed at by ATMOS_OUTPUTS. This function reads that file
// (path on the ConfigAndStacksInfo) and looks up the requested key.
//
// Returns the same triple as TerraformOutputGetter so the StoreCommand
// dispatch is symmetric.
type CustomOutputGetter func(
	info *schema.ConfigAndStacksInfo,
	outputKey string,
) (any, bool, error)

// Assert that StoreCommand implements Command interface.
var _ Command = &StoreCommand{}

type StoreCommand struct {
	Name               string
	atmosConfig        *schema.AtmosConfiguration
	info               *schema.ConfigAndStacksInfo
	terraformOutputter TerraformOutputGetter
	customOutputter    CustomOutputGetter
}

func NewStoreCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*StoreCommand, error) {
	return &StoreCommand{
		Name:        "store",
		atmosConfig: atmosConfig,
		info:        info,
		// terraformOutputter is re-selected per-event in RunE: after-events skip
		// init (workdir already initialized by apply/plan); before-events run init
		// normally. customOutputter reads ATMOS_OUTPUTS for custom component types.
		terraformOutputter: tfoutput.GetOutput,
		customOutputter:    defaultCustomOutputter,
	}, nil
}

func (c *StoreCommand) GetName() string {
	return c.Name
}

func (c *StoreCommand) processStoreCommand(hook *Hook, event HookEvent) error {
	if len(hook.Outputs) == 0 {
		log.Info("Skipping hook. No outputs configured", "hook", hook.Name, "outputs", hook.Outputs)
		return nil
	}

	log.Debug("Executing store hook", "hook", hook.Name, "kind", hook.Kind, "component_type", c.info.ComponentType)
	for key, value := range hook.Outputs {
		outputKey, outputValue, err := c.getOutputValue(hook.Name, event, value)
		if err != nil {
			return err
		}

		err = c.storeOutput(hook, key, outputKey, outputValue)
		if err != nil {
			return err
		}
	}
	return nil
}

// getOutputValue resolves a hook output reference. hookName and event are
// included in error messages so the user can identify which hook triggered the
// failure.
//
// If `value` begins with a dot, it's an "output reference" — look it up in
// terraform state (for terraform components) or in the ATMOS_OUTPUTS file
// (for any other component type). Otherwise it's a literal value, stored as-is.
func (c *StoreCommand) getOutputValue(hookName string, event HookEvent, value string) (string, any, error) {
	if !strings.HasPrefix(value, ".") {
		return value, value, nil
	}

	outputKey := strings.TrimPrefix(value, ".")

	// Dispatch on component type. Terraform keeps the existing state-reading
	// path; everything else reads the outputs file written by the custom
	// command step(s).
	if c.isTerraformComponent() {
		outputValue, exists, err := c.terraformOutputter(
			c.atmosConfig,
			c.info.Stack,
			c.info.ComponentFromArg,
			outputKey,
			true,
			c.info.AuthContext,
			c.info.AuthManager,
		)
		if err != nil {
			return "", nil, fmt.Errorf("%w: hook %q (event %q) failed to get terraform output %q for component %q in stack %q: %w",
				errUtils.ErrTerraformOutputFailed, hookName, event, outputKey, c.info.ComponentFromArg, c.info.Stack, err)
		}
		if !exists {
			return "", nil, fmt.Errorf("%w: hook %q (event %q) could not find terraform output %q for component %q in stack %q",
				errUtils.ErrTerraformOutputNotFound, hookName, event, outputKey, c.info.ComponentFromArg, c.info.Stack)
		}
		// outputValue may legitimately be nil here (null output) — that's allowed.
		return outputKey, outputValue, nil
	}

	// Custom component path.
	outputValue, exists, err := c.customOutputter(c.info, outputKey)
	if err != nil {
		return "", nil, err
	}
	if !exists {
		return "", nil, fmt.Errorf("%w: %s (component %q, stack %q)",
			errUtils.ErrCustomOutputMissing, outputKey, c.info.ComponentFromArg, c.info.Stack)
	}
	return outputKey, outputValue, nil
}

// isTerraformComponent returns true when the active component is a terraform
// component. The empty string is treated as terraform for back-compat with
// older callers that never set ComponentType.
func (c *StoreCommand) isTerraformComponent() bool {
	t := c.info.ComponentType
	return t == "" || t == "terraform"
}

// defaultCustomOutputter reads ATMOS_OUTPUTS for the active info and looks up
// the requested key.
func defaultCustomOutputter(info *schema.ConfigAndStacksInfo, outputKey string) (any, bool, error) {
	outputs, err := ReadOutputsFile(info.OutputsFilePath)
	if err != nil {
		return nil, false, err
	}
	val, ok := outputs[outputKey]
	return val, ok, nil
}

// storeOutput puts the value of the output in the store
func (c *StoreCommand) storeOutput(hook *Hook, key string, outputKey string, outputValue any) error {
	log.Debug("checking if the store exists", "store", hook.Name)
	store := c.atmosConfig.Stores[hook.Name]

	if store == nil {
		return fmt.Errorf("store %q not found in configuration", hook.Name)
	}

	log.Debug("storing output", "outputKey", outputKey, "store", hook.Name, "key", key, "value", outputValue)

	return store.Set(c.info.Stack, c.info.ComponentFromArg, key, outputValue)
}

// RunE is the entrypoint for the store command.
// It selects the appropriate terraform output getter based on the event: after-events
// (e.g. after-terraform-apply) skip terraform init because the workdir is already
// initialized; before-events run init normally since the workdir may not exist yet.
func (c *StoreCommand) RunE(hook *Hook, event HookEvent, cmd *cobra.Command, args []string) error {
	// Re-select the terraform getter per-event: after-events skip init (the
	// workdir is already initialized by apply/plan), before-events run init.
	// This only affects the terraform path; custom component types resolve
	// outputs from the ATMOS_OUTPUTS file via customOutputter.
	if event.IsPostExecution() {
		c.terraformOutputter = tfoutput.GetOutputSkipInit
	} else {
		c.terraformOutputter = tfoutput.GetOutput
	}
	return c.processStoreCommand(hook, event)
}
