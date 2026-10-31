package policy

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/format"
	policypkg "github.com/cloudposse/atmos/pkg/policy"
)

var listParser = flags.NewStandardParser(
	flags.WithStringFlag("format", "f", "table", "Output format: table, json, yaml"),
	flags.WithValidValues("format", "table", "json", "yaml"),
)

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "list", Short: "List loaded policy definitions", Args: cobra.NoArgs, RunE: runList,
	}
	listParser.RegisterFlags(cmd)
	return cmd
}

func runList(cmd *cobra.Command, _ []string) error {
	v := viper.New()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	if err := listParser.ValidateFlagValues(cmd); err != nil {
		return err
	}
	bundle, err := loadBundle()
	if err != nil {
		return err
	}
	return writePolicyList(bundle.Rules, v.GetString("format"))
}

func writePolicyList(rules []policypkg.Rule, outputFormat string) error {
	if len(rules) == 0 {
		return errUtils.ErrNoPoliciesConfigured
	}
	switch outputFormat {
	case "json":
		return data.WriteJSON(rules)
	case "yaml":
		// Honor the policy JSON contract, including raw JSON parameters and resources.
		output, err := yaml.Marshal(rules)
		if err != nil {
			return err
		}
		return data.Write(string(output))
	}
	rows := make([][]string, 0, len(rules))
	for i := range rules {
		rule := &rules[i]
		action := "allow"
		switch {
		case rule.Effects.Deny:
			action = "deny"
		case len(rule.Effects.Approvals) > 0:
			action = "require_approval"
		case len(rule.Effects.Hooks) > 0:
			action = "hooks"
		}
		rows = append(rows, []string{rule.ID, string(rule.On), rule.Engine, action, rule.Description})
	}
	return data.Write(format.CreateStyledTable([]string{"Policy", "On", "Engine", "Action", "Description"}, rows))
}
