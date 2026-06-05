package secret

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var setParser *flags.StandardParser

var setCmd = &cobra.Command{
	Use:     "set NAME[=VALUE]",
	Aliases: []string{"add"},
	Short:   "Set a declared secret's value (create or update).",
	Long: "Set a declared secret's value. Provide the value inline as NAME=VALUE, via --stdin, or " +
		"interactively when running in a terminal.",
	Args: cobra.ExactArgs(1),
	RunE: runSecretSet,
}

func init() {
	setParser = flags.NewStandardParser(
		flags.WithBoolFlag("stdin", "", false, "Read the secret value from standard input"),
		flags.WithBoolFlag("force", "f", false, "Overwrite an existing value without confirmation"),
	)
	setParser.RegisterFlags(setCmd)
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretSet")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}

	name, value, hasValue := strings.Cut(args[0], "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("secret NAME is required").Err()
	}

	useStdin, _ := cmd.Flags().GetBool("stdin")
	resolvedValue, err := resolveSetValue(value, hasValue, useStdin)
	if err != nil {
		return err
	}

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	if err := svc.Set(name, resolvedValue); err != nil {
		return err
	}

	ui.Successf("Set secret `%s` for component `%s` in stack `%s`", name, scope.Component, scope.Stack)
	return nil
}

// resolveSetValue determines the secret value from the inline value, stdin, or a prompt.
func resolveSetValue(inlineValue string, hasInline, useStdin bool) (string, error) {
	if useStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read secret from stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	if hasInline {
		return inlineValue, nil
	}
	return promptForSecretValue()
}
