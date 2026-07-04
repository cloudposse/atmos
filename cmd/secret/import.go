package secret

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

var importParser *flags.StandardParser

// importFromFlags are the flags that select store-coordinate source mode (the positional
// argument is then a declared secret NAME instead of a FILE).
var importFromFlags = []string{"from-store", "from-stack", "from-component", "from-key"}

var importCmd = &cobra.Command{
	Use:   "import FILE|NAME",
	Short: "Import existing secret values, bringing them under management.",
	Long: "Import is the general way to bring existing secret values under management.\n\n" +
		"With a FILE (.env or JSON, or - for stdin), each key is written to the backend named in " +
		"its declaration (the `store:` or `sops:` of the matching secrets.vars entry) — keys backed " +
		"by different stores/providers are routed automatically. Declared keys are set; undeclared " +
		"keys are warned about and skipped (unlike `push`, which fails).\n\n" +
		"With a NAME and any --from-* flag, the value is copied from an existing store coordinate " +
		"into the declaration's computed coordinate (like `terraform import`, the source is never " +
		"modified or deleted). The --from-stack/--from-component values are raw path segments — " +
		"typically transcribed from a legacy `!store <store> <stack> <component> <key>` expression — " +
		"and need not name a real stack or component. --from-store defaults to the declaration's " +
		"own store, --from-key to the secret name.",
	Args: cobra.ExactArgs(1),
	RunE: runSecretImport,
}

func init() {
	importParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "", "env", "Input format: env or json (FILE mode only)"),
		flags.WithBoolFlag("dry-run", "", false, "Preview what would be imported without writing"),
		flags.WithStringFlag("from-store", "", "", "Source store to copy from (default: the declaration's own store)"),
		flags.WithStringFlag("from-stack", "", "", "Source stack path segment (raw; need not be a real stack)"),
		flags.WithStringFlag("from-component", "", "", "Source component path segment (raw; omit for stack-scoped source paths)"),
		flags.WithStringFlag("from-key", "", "", "Source key (default: the secret name)"),
	)
	importParser.RegisterFlags(importCmd)
}

func runSecretImport(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretImport")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if importFromStoreMode(cmd) {
		return runSecretImportFromStore(cmd, scope, args[0], dryRun)
	}
	return runSecretImportFile(cmd, scope, args[0], dryRun)
}

// importFromStoreMode reports whether any --from-* flag was given, selecting store-coordinate
// source mode (positional arg = NAME) over the default file mode (positional arg = FILE).
func importFromStoreMode(cmd *cobra.Command) bool {
	for _, name := range importFromFlags {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

// runSecretImportFromStore copies one declared secret's value from an existing store coordinate
// into the declaration's computed coordinate.
func runSecretImportFromStore(cmd *cobra.Command, scope secretScope, name string, dryRun bool) error {
	if cmd.Flags().Changed("format") {
		return errUtils.Build(errUtils.ErrMutuallyExclusiveFlags).
			WithExplanation("--format applies to FILE imports and cannot be combined with --from-* flags").
			WithHint("Drop --format when importing from a store coordinate; the value is copied as-is").
			Err()
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	src := secrets.ImportSource{
		Store:     flagValue(cmd, "from-store"),
		Stack:     flagValue(cmd, "from-stack"),
		Component: flagValue(cmd, "from-component"),
		Key:       flagValue(cmd, "from-key"),
	}
	if err := svc.ImportFromStore(name, src, dryRun); err != nil {
		return err
	}

	if dryRun {
		ui.Infof("Would import `%s` from the source coordinate (source verified readable)", name)
		return nil
	}
	ui.Successf("Imported `%s` from its source coordinate (the source value was left in place)", name)
	return nil
}

// flagValue returns a string flag's value, tolerating absent flags.
func flagValue(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// runSecretImportFile imports declared keys from a .env/JSON file, warning on undeclared keys.
func runSecretImportFile(cmd *cobra.Command, scope secretScope, file string, dryRun bool) error {
	format, _ := cmd.Flags().GetString("format")

	values, err := parseSecretsFile(file, format)
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
