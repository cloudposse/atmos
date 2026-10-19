package ci

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cipkg "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/policy"
)

var ciContextParser = flags.NewStandardParser(
	flags.WithBoolFlag("paths", "", false, "List observed field paths and types instead of snapshot values"),
	flags.WithStringFlag("query", "q", "", "YQ expression to filter the JSON output"),
	flags.WithEnvVars("query", "ATMOS_QUERY"),
)

const (
	contextMaxNestingDepth = 64
	contextMaxFields       = 10_000
)

var (
	errContextTooDeep       = errors.New("CI context exceeds the field explorer's maximum nesting depth of 64")
	errContextTooManyFields = errors.New("CI context exceeds the field explorer's maximum field count of 10000")
)

func newContextCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "context",
		Short: "Inspect the CI snapshot available to policies",
		Long: `Print the provider-native CI snapshot as JSON under the "ci" key.

Uses the same collector as policy evaluation, without fetching API facts or
running component operations or lifecycle hooks. Normal command admission applies.
Outside CI, prints {"ci": null}. Unavailable datasets retain their status and reason.

Use --paths to list observed field paths and JSON types without field values.
Paths work in CEL; prefix them with "ctx." for Starlark. This is an inspection
of the current event, not an exhaustive provider schema.

Use --query (-q) to filter the snapshot or --paths output with a YQ expression.
Results remain JSON; a query selecting multiple results emits one JSON value per result.

Native payloads may contain private information. Review output before sharing it.`,
		Args: cobra.NoArgs,
		RunE: runCIContext,
	}
	ciContextParser.RegisterFlags(command)
	return command
}

func runCIContext(cmd *cobra.Command, _ []string) error {
	v := viper.New()
	if err := ciContextParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	paths, query := v.GetBool("paths"), v.GetString("query")
	snapshot, err := cipkg.CollectPolicySnapshot(cmd.Context())
	if err != nil {
		return err
	}
	if !paths {
		return writeCIContext(map[string]any{"ci": snapshot}, query)
	}
	fields, err := ciContextPaths(snapshot)
	if err != nil {
		return err
	}
	return writeCIContext(fields, query)
}

type ciContextField struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

func ciContextPaths(snapshot *policy.CISnapshot) ([]ciContextField, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	fields := []ciContextField{}
	if err := walkCIContext(value, "ci", 0, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func walkCIContext(value any, path string, depth int, fields *[]ciContextField) error {
	// Bound output growth for deeply nested or broad provider-supplied JSON. Never
	// silently truncate and imply that the field inventory is complete.
	if depth > contextMaxNestingDepth {
		return errContextTooDeep
	}
	if len(*fields) >= contextMaxFields {
		return errContextTooManyFields
	}
	*fields = append(*fields, ciContextField{Path: path, Type: ciContextType(value)})
	switch node := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			// JSON quoting handles punctuation, quotes, and control characters.
			quoted, _ := json.Marshal(key)
			if err := walkCIContext(node[key], path+"["+string(quoted)+"]", depth+1, fields); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range node {
			if err := walkCIContext(item, fmt.Sprintf("%s[%d]", path, index), depth+1, fields); err != nil {
				return err
			}
		}
	}
	return nil
}

func ciContextType(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	default:
		return "null"
	}
}
