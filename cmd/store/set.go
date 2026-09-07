package store

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var setParser *flags.StandardParser

var setCmd = &cobra.Command{
	Use:   "set STORE KEY [VALUE]",
	Short: "Write a value to a store (create or update).",
	Long: "Write a value to a configured store backend. Provide the value inline, via --stdin, or " +
		"interactively when running in a terminal.",
	Args: cobra.RangeArgs(2, 3),
	RunE: runStoreSet,
}

func init() {
	setParser = flags.NewStandardParser(
		flags.WithBoolFlag("stdin", "", false, "Read the value from standard input"),
	)
	setParser.RegisterFlags(setCmd)
}

func runStoreSet(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "store.runStoreSet")()

	scope, err := parseStoreScope(cmd)
	if err != nil {
		return err
	}

	storeName, key := args[0], args[1]
	inlineValue, hasInline := "", false
	if len(args) == 3 {
		inlineValue, hasInline = args[2], true
	}

	useStdin, _ := cmd.Flags().GetBool("stdin")
	value, err := resolveSetValue(inlineValue, hasInline, useStdin)
	if err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	if err := svc.Set(storeName, scope.Stack, scope.Component, key, value); err != nil {
		return err
	}

	ui.Successf("Set `%s` in store `%s`", key, storeName)
	return nil
}

// resolveSetValue determines the value from the inline VALUE argument, stdin, or a prompt.
func resolveSetValue(inlineValue string, hasInline, useStdin bool) (string, error) {
	if useStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read value from stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	if hasInline {
		return inlineValue, nil
	}
	return promptForValueFn()
}
