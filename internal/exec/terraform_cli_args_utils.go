package exec

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filetype"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const tfCliArgsEnvVar = "TF_CLI_ARGS"

// parseArgs splits a command line string into arguments, handling quoted strings.
func parseArgs(input string) []string {
	if input == "" {
		return []string{}
	}

	var args []string
	var current strings.Builder
	inQuotes := false
	var quoteChar rune

	for _, char := range input {
		switch {
		case char == '"' || char == '\'':
			inQuotes, quoteChar = handleQuoteChar(char, inQuotes, quoteChar, &current)
		case char == ' ' && !inQuotes:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// handleQuoteChar processes quote characters and returns updated quote state.
func handleQuoteChar(char rune, inQuotes bool, quoteChar rune, current *strings.Builder) (bool, rune) {
	switch {
	case !inQuotes:
		return true, char
	case char == quoteChar:
		return false, 0
	default:
		current.WriteRune(char)
		return inQuotes, quoteChar
	}
}

// GetTerraformEnvCliArgs reads the TF_CLI_ARGS environment variable and returns a slice of arguments.
// Example: TF_CLI_ARGS="-var environment=prod -auto-approve -var region=us-east-1".
// Returns: ["-var", "environment=prod", "-auto-approve", "-var", "region=us-east-1"].
func GetTerraformEnvCliArgs() []string {
	defer perf.Track(nil, "exec.GetTerraformEnvCliArgs")()

	// Get TF_CLI_ARGS environment variable directly from os.Getenv for better cross-platform compatibility.
	// Using os.Getenv instead of viper to avoid potential viper binding issues on Windows.
	tfCliArgs := os.Getenv(tfCliArgsEnvVar) //nolint:forbidigo // This is not a CLI command, but an internal utility for parsing TF_CLI_ARGS.
	if tfCliArgs == "" {
		return []string{}
	}

	// Split the arguments by spaces, handling quoted strings.
	return parseArgs(tfCliArgs)
}

// GetTerraformEnvCliVars reads the TF_CLI_ARGS environment variable, parses all `-var` arguments,
// and returns them as a map of variables with proper type conversion.
// This function processes JSON values and returns them as parsed objects.
// It handles both formats: -var key=value and -var=key=value.
// Example: TF_CLI_ARGS='-var name=test -var=region=us-east-1 -var tags={"env":"prod","team":"devops"}'
// Returns: map[string]any{"name": "test", "region": "us-east-1", "tags": map[string]any{"env": "prod", "team": "devops"}}.
func GetTerraformEnvCliVars() (map[string]any, error) {
	defer perf.Track(nil, "exec.GetTerraformEnvCliVars")()

	args := GetTerraformEnvCliArgs()
	if len(args) == 0 {
		return map[string]any{}, nil
	}

	variables := make(map[string]any)
	for i := 0; i < len(args); i++ {
		arg := args[i]

		var kv string
		// Look for the `-var` arguments.
		switch {
		case arg == "-var" && i+1 < len(args):
			kv = args[i+1]
			i++
		case strings.HasPrefix(arg, "-var="):
			// Handle -var=key=value format.
			kv = strings.TrimPrefix(arg, "-var=")
		default:
			continue
		}

		// Process the key=value pair.
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed arguments.
		}

		varName := parts[0]
		part2 := parts[1]
		var varValue any

		// Check if the value is JSON and parse it accordingly.
		if filetype.IsJSON(part2) {
			v, err := u.ConvertFromJSON(part2)
			if err != nil {
				return nil, fmt.Errorf("%w: parsing TF_CLI_ARGS -var JSON for %q: %v", errUtils.ErrTerraformEnvCliVarJSON, varName, err)
			}
			varValue = v
		} else {
			varValue = strings.TrimSpace(part2)
		}

		variables[varName] = varValue
	}

	return variables, nil
}
