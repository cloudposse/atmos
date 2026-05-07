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

// Assert that StoreCommand implements Command interface.
var _ Command = &StoreCommand{}

type StoreCommand struct {
	Name         string
	atmosConfig  *schema.AtmosConfiguration
	info         *schema.ConfigAndStacksInfo
	outputGetter TerraformOutputGetter
}

func NewStoreCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*StoreCommand, error) {
	return &StoreCommand{
		Name:        "store",
		atmosConfig: atmosConfig,
		info:        info,
		// outputGetter is resolved per-event in RunE: after-events skip init (workdir
		// already initialized by apply/plan); before-events run init normally.
		outputGetter: tfoutput.GetOutput,
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

	log.Debug("Executing store hook", "hook", hook.Name, "command", hook.Command)
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

// getOutputValue gets an output from terraform or returns a literal value.
// hookName and event are included in error messages so the user can identify
// which hook triggered the failure.
func (c *StoreCommand) getOutputValue(hookName string, event HookEvent, value string) (string, any, error) {
	outputKey := strings.TrimPrefix(value, ".")
	var outputValue any

	if strings.Index(value, ".") == 0 {
		var exists bool
		var err error
		outputValue, exists, err = c.outputGetter(c.atmosConfig, c.info.Stack, c.info.ComponentFromArg, outputKey, true, c.info.AuthContext, c.info.AuthManager)
		// Handle errors from terraform output retrieval (SDK errors, network issues, etc.).
		if err != nil {
			return "", nil, fmt.Errorf("%w: hook %q (event %q) failed to get terraform output %q for component %q in stack %q: %w",
				errUtils.ErrTerraformOutputFailed, hookName, event, outputKey, c.info.ComponentFromArg, c.info.Stack, err)
		}

		// Handle missing outputs (key doesn't exist).
		// This is different from a legitimate null value.
		if !exists {
			return "", nil, fmt.Errorf("%w: hook %q (event %q) could not find terraform output %q for component %q in stack %q",
				errUtils.ErrTerraformOutputNotFound, hookName, event, outputKey, c.info.ComponentFromArg, c.info.Stack)
		}

		// At this point, exists==true, but outputValue may be nil.
		// A nil value here is a legitimate Terraform output that is null, which is valid.
		// We allow it to be stored.
	} else {
		outputValue = value
	}
	return outputKey, outputValue, nil
}

// storeOutput puts the value of the output in the store
func (c *StoreCommand) storeOutput(hook *Hook, key string, outputKey string, outputValue any) error {
	log.Debug("checking if the store exists", "store", hook.Name)
	store := c.atmosConfig.Stores[hook.Name]

	if store == nil {
		return fmt.Errorf("store %q not found in configuration", hook.Name)
	}

	log.Debug("storing terraform output", "outputKey", outputKey, "store", hook.Name, "key", key, "value", outputValue)

	return store.Set(c.info.Stack, c.info.ComponentFromArg, key, outputValue)
}

// RunE is the entrypoint for the store command.
// It selects the appropriate terraform output getter based on the event: after-events
// (e.g. after-terraform-apply) skip terraform init because the workdir is already
// initialized; before-events run init normally since the workdir may not exist yet.
func (c *StoreCommand) RunE(hook *Hook, event HookEvent, cmd *cobra.Command, args []string) error {
	if event.IsPostExecution() {
		c.outputGetter = tfoutput.GetOutputSkipInit
	} else {
		c.outputGetter = tfoutput.GetOutput
	}
	return c.processStoreCommand(hook, event)
}
