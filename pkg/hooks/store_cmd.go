package hooks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

// assert that Command implements Command interface.
var _ Command = &StoreCommand{}

type StoreCommand struct {
	Name        string
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
}

func NewStoreCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*StoreCommand, error) {
	return &StoreCommand{
		Name:        "store",
		atmosConfig: atmosConfig,
		info:        info,
	}, nil
}

func (c *StoreCommand) GetName() string {
	return c.Name
}

func (c *StoreCommand) processStoreCommand(hook *Hook) error {
	if len(hook.Outputs) == 0 {
		log.Info("skipping hook. no outputs configured.", "hook", hook.Name, "outputs", hook.Outputs)
		return nil
	}

	log.Debug("executing 'after-terraform-apply' hook", "hook", hook.Name, "command", hook.Command)

	for key, value := range hook.Outputs {
		outputKey, outputValue := c.getOutputValue(value)

		err := c.storeOutput(hook, key, outputKey, outputValue)
		if err != nil {
			return err
		}
	}

	return nil
}

// getOutputValue gets an output from terraform.
func (c *StoreCommand) getOutputValue(value string) (string, any) {
	outputKey := strings.TrimPrefix(value, ".")

	var outputValue any

	if strings.Index(value, ".") == 0 {
		outputValue = e.GetTerraformOutput(c.atmosConfig, c.info.Stack, c.info.ComponentFromArg, outputKey, true)
	} else {
		outputValue = value
	}

	return outputKey, outputValue
}

// storeOutput puts the value of the output in the store.
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
func (c *StoreCommand) RunE(hook *Hook, event HookEvent, cmd *cobra.Command, args []string) error {
	return c.processStoreCommand(hook)
}
