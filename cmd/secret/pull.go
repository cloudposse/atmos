package secret

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

var pullParser *flags.StandardParser

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Download declared secrets to a local file for development.",
	Long:  "Download the initialized declared secrets for a component in a stack to a local .env or JSON file. Secret values are masked in any UI output; they are written to the target file in cleartext for local development.",
	Args:  cobra.NoArgs,
	RunE:  runSecretPull,
}

func init() {
	pullParser = flags.NewStandardParser(
		flags.WithStringFlag("output", "o", "", "Output file (default: stdout)"),
		flags.WithStringFlag("format", "", "env", "Output format: env or json"),
	)
	pullParser.RegisterFlags(pullCmd)
}

func runSecretPull(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretPull")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	format, _ := cmd.Flags().GetString("format")

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	values := make(map[string]any)
	for _, decl := range svc.Declarations() {
		value, getErr := svc.Get(decl.Name, secrets.ResolveOptions{})
		if getErr != nil {
			ui.Warningf("Skipping `%s`: %v", decl.Name, getErr)
			continue
		}
		values[decl.Name] = value
	}

	rendered, err := renderPull(values, format)
	if err != nil {
		return err
	}

	if output == "" {
		return data.Writeln(rendered)
	}
	const filePerm = 0o600
	if err := os.WriteFile(output, []byte(rendered+"\n"), filePerm); err != nil {
		return fmt.Errorf("failed to write %q: %w", output, err)
	}
	ui.Successf("Wrote %d secret(s) to %s", len(values), output)
	return nil
}

// renderPull serializes pulled secrets in the requested format.
func renderPull(values map[string]any, format string) (string, error) {
	switch format {
	case "json":
		b, err := json.MarshalIndent(values, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "env", "":
		names := make([]string, 0, len(values))
		for k := range values {
			names = append(names, k)
		}
		sort.Strings(names) // Deterministic order.
		var b strings.Builder
		for _, k := range names {
			fmt.Fprintf(&b, "%s=%v\n", k, values[k])
		}
		return strings.TrimRight(b.String(), "\n"), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
}
