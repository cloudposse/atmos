package exec

import (
	"strings"

	"github.com/spf13/viper"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// ParseTFCliArgsVars reads the TF_CLI_ARGS environment variable, detects all -var arguments,
// and returns them as a map of key-value pairs.
// It handles both formats: -var key=value and -var "key=value".
func ParseTFCliArgsVars() map[string]string {
	result := make(map[string]string)

	// Get TF_CLI_ARGS environment variable using viper.
	if err := viper.BindEnv("TF_CLI_ARGS"); err != nil {
		log.Debug("Failed to bind TF_CLI_ARGS environment variable", "error", err)
		return result
	}
	tfCliArgs := viper.GetString("TF_CLI_ARGS")
	if tfCliArgs == "" {
		return result
	}

	// Split the arguments by spaces, but handle quoted strings properly.
	args := parseCommandArgs(tfCliArgs)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Look for -var arguments.
		if arg == "-var" {
			i = extractVarFromNextArg(args, i, result)
		} else if strings.HasPrefix(arg, "-var=") {
			extractVarFromEqualFormat(arg, result)
		}
	}

	return result
}

// extractVarFromNextArg extracts a variable from the next argument in the format "-var key=value".
func extractVarFromNextArg(args []string, i int, result map[string]string) int {
	// Next argument should be the key=value pair.
	if i+1 < len(args) {
		i++ // Move to the next argument.
		keyValue := args[i]
		if key, value, found := strings.Cut(keyValue, "="); found {
			result[key] = value
		}
	}
	return i
}

// extractVarFromEqualFormat extracts a variable from the format "-var=key=value".
func extractVarFromEqualFormat(arg string, result map[string]string) {
	// Handle -var=key=value format.
	varArg := strings.TrimPrefix(arg, "-var=")
	if key, value, found := strings.Cut(varArg, "="); found {
		result[key] = value
	}
}

// parseCommandArgs splits a command line string into arguments, properly handling quoted strings.
// This is a simplified parser that handles basic quoting scenarios.
func parseCommandArgs(cmdLine string) []string {
	if cmdLine == "" {
		return []string{}
	}

	parser := &argParser{
		args:    make([]string, 0),
		current: strings.Builder{},
	}

	for i, char := range cmdLine {
		parser.processChar(char)

		// Handle the last argument.
		if i == len(cmdLine)-1 {
			parser.finalize()
		}
	}

	return parser.args
}

// argParser handles the parsing state for command line arguments.
type argParser struct {
	args      []string
	current   strings.Builder
	inQuotes  bool
	quoteChar rune
}

// processChar processes a single character in the command line.
func (p *argParser) processChar(char rune) {
	switch {
	case isQuoteChar(char):
		p.handleQuote(char)
	case char == ' ' && !p.inQuotes:
		p.handleSpace()
	default:
		p.current.WriteRune(char)
	}
}

// isQuoteChar checks if the character is a quote character.
func isQuoteChar(char rune) bool {
	return char == '"' || char == '\''
}

// handleQuote processes quote characters.
func (p *argParser) handleQuote(char rune) {
	switch {
	case !p.inQuotes:
		p.inQuotes = true
		p.quoteChar = char
	case char == p.quoteChar:
		p.inQuotes = false
		p.quoteChar = 0
	default:
		p.current.WriteRune(char)
	}
}

// handleSpace processes space characters.
func (p *argParser) handleSpace() {
	if p.current.Len() > 0 {
		p.args = append(p.args, p.current.String())
		p.current.Reset()
	}
}

// finalize completes the parsing and adds any remaining argument.
func (p *argParser) finalize() {
	if p.current.Len() > 0 {
		p.args = append(p.args, p.current.String())
	}
}
