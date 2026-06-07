package secret

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var pushParser *flags.StandardParser

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Upload secret values from a local file (all keys must be declared).",
	Long:  "Upload secret values from a local .env or JSON file. Every key in the file MUST be declared; push fails on the first undeclared key. Use `import` to warn and skip undeclared keys instead.",
	Args:  cobra.NoArgs,
	RunE:  runSecretPush,
}

func init() {
	pushParser = flags.NewStandardParser(
		flags.WithStringFlag("input", "", ".env", "Input file to read secret values from (use - for stdin)"),
		flags.WithStringFlag("format", "", "env", "Input format: env or json"),
	)
	pushParser.RegisterFlags(pushCmd)
}

func runSecretPush(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretPush")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}
	input, _ := cmd.Flags().GetString("input")
	format, _ := cmd.Flags().GetString("format")

	values, err := parseSecretsFile(input, format)
	if err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	// Fail fast on any undeclared key before writing anything.
	for _, key := range sortedKeys(values) {
		if !svc.IsDeclared(key) {
			return errUtils.Build(errUtils.ErrValidationFailed).
				WithExplanation(fmt.Sprintf("key %q is not declared as a secret", key)).
				WithHint("Declare it under the component's secrets.vars, or use `atmos secret import` to skip undeclared keys").
				Err()
		}
	}

	count := 0
	for _, key := range sortedKeys(values) {
		if err := svc.Set(key, values[key]); err != nil {
			return err
		}
		count++
	}

	ui.Successf("Pushed %d secret(s) for component `%s` in stack `%s`", count, scope.Component, scope.Stack)
	return nil
}
