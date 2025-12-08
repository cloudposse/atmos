package hooks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
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
		Name:         "store",
		atmosConfig:  atmosConfig,
		info:         info,
		outputGetter: e.GetTerraformOutput,
	}, nil
}

func (c *StoreCommand) GetName() string {
	return c.Name
}

func (c *StoreCommand) processStoreCommand(hook *Hook) error {
	if len(hook.Outputs) == 0 {
		log.Info("Skipping hook. No outputs configured", "hook", hook.Name, "outputs", hook.Outputs)
		return nil
	}

	log.Debug("Executing 'after-terraform-apply' hook", "hook", hook.Name, "command", hook.Command)
	for key, value := range hook.Outputs {
		outputKey, outputValue, err := c.getOutputValue(value)
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
func (c *StoreCommand) getOutputValue(value string) (string, any, error) {
	outputKey := strings.TrimPrefix(value, ".")
	var outputValue any

	if strings.Index(value, ".") == 0 {
		var exists bool
		var err error
		outputValue, exists, err = c.outputGetter(c.atmosConfig, c.info.Stack, c.info.ComponentFromArg, outputKey, true, c.info.AuthContext, c.info.AuthManager)
		// Handle errors from terraform output retrieval (SDK errors, network issues, etc.).
		if err != nil {
			return "", nil, fmt.Errorf("%w: failed to get terraform output for key %s: %w", errUtils.ErrNilTerraformOutput, outputKey, err)
		}

		// Handle missing outputs (key doesn't exist).
		// This is different from a legitimate null value.
		if !exists {
			return "", nil, fmt.Errorf("%w: terraform output key %s does not exist", errUtils.ErrNilTerraformOutput, outputKey)
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

// RunE is the entrypoint for the store command
func (c *StoreCommand) RunE(hook *Hook, event HookEvent, cmd *cobra.Command, args []string) error {
	return c.processStoreCommand(hook)
}
