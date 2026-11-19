package docs

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	pkgdocs "github.com/cloudposse/atmos/pkg/docs"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// skipHooksFlag is the flag (and Viper key suffix) that skips lifecycle hooks for one run.
const (
	skipHooksFlag        = "skip-hooks"
	skipHooksViperPrefix = "docs_generate_flags"
)

// docsGenerateParser registers and binds the docs generate flags.
var docsGenerateParser = flags.NewStandardParser(
	// Bare --skip-hooks skips all hooks; --skip-hooks=a,b skips the named hooks.
	flags.WithStringFlag(skipHooksFlag, "", "", "Skip lifecycle hooks for this invocation. Use --skip-hooks (no value) to skip all, or --skip-hooks=name1,name2 to skip specific hooks by name"),
	flags.WithNoOptDefVal(skipHooksFlag, "*"),
	flags.WithEnvVars(skipHooksFlag, "ATMOS_SKIP_HOOKS"),
	// The prefix avoids colliding with the docs.generate section of atmos.yaml in Viper.
	flags.WithViperPrefix(skipHooksViperPrefix),
)

// applySkipHooksEnv resolves --skip-hooks precedence (flag, then ATMOS_SKIP_HOOKS) and records the
// result on the flag, which is where the hook engine reads it.
func applySkipHooksEnv(cmd *cobra.Command) error {
	v := viper.GetViper()
	if err := docsGenerateParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	flag := cmd.Flags().Lookup(skipHooksFlag)
	if flag == nil || flag.Changed {
		return nil
	}
	if value := v.GetString(skipHooksViperPrefix + "." + skipHooksFlag); value != "" {
		return cmd.Flags().Set(skipHooksFlag, value)
	}
	return nil
}

// docsGenerateCmd is the subcommand under docs that groups generation operations.
var docsGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate documentation artifacts",
	Long: `Generate documentation by merging YAML data sources and applying templates.
Supports native terraform-docs injection.`,
	Example: `Generate the README.md in the current directory:
  atmos docs generate readme`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"readme"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// cobra.ExactArgs(1) already validates argument count.
		if err := applySkipHooksEnv(cmd); err != nil {
			return err
		}
		return pkgdocs.ExecuteGenerate(cmd, args)
	},
}

func init() {
	docsGenerateParser.RegisterFlags(docsGenerateCmd)
	if err := docsGenerateParser.BindToViper(viper.GetViper()); err != nil {
		log.Debug("Failed to bind docs generate flags to Viper", "error", err)
	}
	docsCmd.AddCommand(docsGenerateCmd)
	internal.RegisterCommandMetadata(
		docsGenerateCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/docs/generate"),
	)
}
