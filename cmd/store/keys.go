package store

import (
	"encoding/json"
	"sort"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var keysParser *flags.StandardParser

var keysCmd = &cobra.Command{
	Use:   "keys STORE",
	Short: "List the keys stored under a stack/component scope in a store.",
	Long: "List the keys in a configured store backend. Scope the listing with --stack and " +
		"--component the same way `atmos store set` writes a key (omit both to list a " +
		"store-global scope). Only backends that support key enumeration can run this command; " +
		"see the Listable column in `atmos store list`.",
	Args: cobra.ExactArgs(1),
	RunE: runStoreKeys,
}

func init() {
	keysParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "", "text", "Output format: text, json"),
		flags.WithValidValues("format", "text", "json"),
	)
	keysParser.RegisterFlags(keysCmd)
}

func runStoreKeys(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "store.runStoreKeys")()

	scope, err := parseStoreScope(cmd)
	if err != nil {
		return err
	}
	storeName := args[0]

	format, _ := cmd.Flags().GetString("format")

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	keys, err := svc.Keys(storeName, scope.Stack, scope.Component)
	if err != nil {
		return err
	}
	sort.Strings(keys)

	return writeStoreKeys(keys, format)
}

// writeStoreKeys renders the key list as JSON or as one key per line (text, the default).
func writeStoreKeys(keys []string, format string) error {
	if format == "json" {
		b, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		return data.Writeln(string(b))
	}

	if len(keys) == 0 {
		ui.Info("No keys found.")
		return nil
	}
	for _, k := range keys {
		if err := data.Writeln(k); err != nil {
			return err
		}
	}
	return nil
}
