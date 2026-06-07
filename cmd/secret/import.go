package secret

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var importParser *flags.StandardParser

var importCmd = &cobra.Command{
	Use:   "import FILE",
	Short: "Import secret values from a file, skipping undeclared keys.",
	Long: "Import secret values from a .env or JSON file. Each key is written to the backend named " +
		"in its declaration (the `store:` or `sops:` of the matching secrets.vars entry) — there is no " +
		"single destination to choose, and keys backed by different stores/providers are routed " +
		"automatically. Declared keys are set; undeclared keys are warned about and skipped (unlike " +
		"`push`, which fails). Use - to read from stdin.",
	Args: cobra.ExactArgs(1),
	RunE: runSecretImport,
}

func init() {
	importParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "", "env", "Input format: env or json"),
		flags.WithBoolFlag("dry-run", "", false, "Preview what would be imported without writing"),
	)
	importParser.RegisterFlags(importCmd)
}

func runSecretImport(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretImport")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	values, err := parseSecretsFile(args[0], format)
	if err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	imported, skipped := 0, 0
	for _, key := range sortedKeys(values) {
		if !svc.IsDeclared(key) {
			ui.Warningf("Skipping undeclared key `%s`", key)
			skipped++
			continue
		}
		if dryRun {
			ui.Infof("Would import `%s`", key)
			imported++
			continue
		}
		if err := svc.Set(key, values[key]); err != nil {
			return err
		}
		imported++
	}

	if dryRun {
		ui.Infof("Dry run: %d would be imported, %d skipped (undeclared)", imported, skipped)
		return nil
	}
	ui.Successf("Imported %d secret(s), skipped %d undeclared", imported, skipped)
	return nil
}
