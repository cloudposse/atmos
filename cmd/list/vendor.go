package list

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

var vendorParser *flags.StandardParser

// VendorOptions contains parsed flags for the vendor command.
type VendorOptions struct {
	global.Flags
	Format    string
	Stack     string
	Delimiter string
}

// vendorCmd lists vendor configurations.
var vendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "List all vendor configurations",
	Long:  "List all vendor configurations in a tabular way, including component and vendor manifests.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Skip stack validation for vendor.
		if err := checkAtmosConfig(true); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := vendorParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &VendorOptions{
			Flags:     flags.ParseGlobalFlags(cmd, v),
			Format:    v.GetString("format"),
			Stack:     v.GetString("stack"),
			Delimiter: v.GetString("delimiter"),
		}

		output, err := listVendorWithOptions(opts)
		if err != nil {
			return err
		}

		// Obfuscate home directory paths before printing.
		obfuscatedOutput := obfuscateHomeDirInOutput(output)
		fmt.Println(obfuscatedOutput)
		return nil
	},
}

func init() {
	// Create parser with vendor-specific flags using functional options
	vendorParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringFlag("stack", "s", "", "Filter by stack name or pattern"),
		flags.WithStringFlag("delimiter", "d", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("delimiter", "ATMOS_LIST_DELIMITER"),
	)

	// Register flags
	vendorParser.RegisterFlags(vendorCmd)

	// Add stack completion
	addStackCompletion(vendorCmd)

	// Bind flags to Viper for environment variable support
	if err := vendorParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listVendorWithOptions(opts *VendorOptions) (string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return "", err
	}

	options := &l.FilterOptions{
		FormatStr:    opts.Format,
		StackPattern: opts.Stack,
		Delimiter:    opts.Delimiter,
	}

	return l.FilterAndListVendor(&atmosConfig, options)
}

// obfuscateHomeDirInOutput replaces occurrences of the home directory with "~" to prevent leaking user paths.
func obfuscateHomeDirInOutput(output string) string {
	homeDir, err := homedir.Dir()
	if err != nil || homeDir == "" {
		return output
	}

	// Replace home directory with tilde only at path boundaries.
	// This prevents replacing homeDir when it's a prefix of another directory name.
	// For example, if homeDir is "/home/user", we should not replace "/home/username".
	sep := string(os.PathSeparator)

	// First replace homeDir followed by separator (e.g., "/home/user/file" -> "~/file").
	result := strings.ReplaceAll(output, homeDir+sep, "~"+sep)

	// Then replace homeDir at the end of string or followed by non-path characters.
	// We need to handle cases like:
	// - homeDir alone (e.g., "/home/user" -> "~")
	// - homeDir followed by space, newline, or other delimiters (e.g., "/home/user\n" -> "~\n")
	// But NOT homeDir as a prefix (e.g., "/home/username" should remain unchanged).
	var builder strings.Builder
	builder.Grow(len(result))

	i := 0
	for i < len(result) {
		// Check if we have homeDir at current position.
		if strings.HasPrefix(result[i:], homeDir) {
			nextPos := i + len(homeDir)
			if shouldReplaceHomeDir(result, nextPos) {
				builder.WriteString("~")
				i = nextPos
			} else {
				// This is a prefix - keep original.
				builder.WriteString(homeDir)
				i = nextPos
			}
		} else {
			builder.WriteByte(result[i])
			i++
		}
	}

	return builder.String()
}

// shouldReplaceHomeDir checks if homeDir at the current position should be replaced with ~.
// Returns true if at end of string or followed by non-path characters.
// Returns false if followed by alphanumeric/dash/underscore (indicating a prefix).
func shouldReplaceHomeDir(result string, nextPos int) bool {
	// homeDir at end of string - replace it.
	if nextPos >= len(result) {
		return true
	}

	nextChar := result[nextPos]
	// Check if next character is alphanumeric/dash/underscore.
	// If so, this is likely a prefix of another name - don't replace.
	if isPathChar(nextChar) {
		return false
	}

	// This is a boundary - replace with ~.
	return true
}

// isPathChar returns true if the character is typically part of a path component name.
func isPathChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}
