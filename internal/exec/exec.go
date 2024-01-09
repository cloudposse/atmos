package exec

import (
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
)

func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	commans := []string{
		"terraform plan",
		"terraform apply",
		"terraform init",
		"terraform clean",
		"terraform workspace",
		"terraform generate varfile",
		"terraform generate backend",
		"helmfile diff",
		"helmfile apply",
		"helmfile generate varfile",
		"describe component",
		"describe dependents",
		"validate component",
	}

	stacks := []string{
		"Ramen",
		"Tomato Soup",
		"Hamburgers",
		"Cheeseburgers",
		"Currywurst",
		"Okonomiyaki",
		"Pasta",
		"Fillet Mignon",
		"Caviar",
		"Just Wine",
	}

	componens := []string{
		"Ramen",
		"Tomato Soup",
		"Hamburgers",
		"Cheeseburgers",
		"Currywurst",
		"Okonomiyaki",
		"Pasta",
		"Fillet Mignon",
		"Caviar",
		"Just Wine",
	}

	_, component, stack, err := tui.Execute(commans, componens, stacks)
	if err != nil {
		return err
	}

	c, err := ExecuteDescribeComponent(component, stack)
	if err != nil {
		return err
	}

	err = u.PrintAsYAML(c)
	if err != nil {
		return err
	}

	return nil
}
