package planfile

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listParser handles flag parsing with Viper precedence for the list command.
var listParser *flags.StandardParser

// ListOptions contains parsed flags for the list command.
type ListOptions struct {
	BaseOptions
	Format    string
	Component string
	All       bool
}

var listCmd = &cobra.Command{
	Use:   "list [component]",
	Short: "List Terraform plan files in storage",
	Long: `List Terraform plan files from the configured storage backend.

Optionally filter by component (positional arg) and/or stack (-s flag).
By default, only planfiles for the current SHA are shown. Use --all to show all SHAs.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

func init() {
	// Create parser with list-specific flags using functional options.
	listParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithStringFlag("format", "", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithBoolFlag("all", "", false, "Show planfiles for all SHAs (bypass SHA filter)"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
		flags.WithEnvVars("format", "ATMOS_PLANFILE_FORMAT"),
	)

	// Register flags with the command.
	listParser.RegisterFlags(listCmd)

	// Bind to Viper for environment variable support.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	PlanfileCmd.AddCommand(listCmd)
}

// parseListOptions parses command flags into ListOptions.
func parseListOptions(cmd *cobra.Command, v *viper.Viper, args []string) *ListOptions {
	component := ""
	if len(args) > 0 {
		component = args[0]
	}

	return &ListOptions{
		BaseOptions: parseBaseOptions(cmd, v),
		Format:      v.GetString("format"),
		Component:   component,
		All:         v.GetBool("all"),
	}
}

func runList(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runList")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Bind persistent parent flags too.
	if err := planfileParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseListOptions(cmd, v, args)

	// Build ConfigAndStacksInfo from global flags to honor config selection flags.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           opts.BasePath,
		AtmosConfigFilesFromArg: opts.Config,
		AtmosConfigDirsFromArg:  opts.ConfigPath,
		ProfilesFromArg:         opts.Profile,
	}

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Get the storage configuration.
	storeOpts, err := getStoreOptions(&atmosConfig, opts.Store)
	if err != nil {
		return err
	}

	// Extract owner/repo from store options if available (e.g., GitHub store).
	owner, repo := extractOwnerRepo(storeOpts)

	// Create the store.
	store, err := createStore(&atmosConfig, opts.Store)
	if err != nil {
		return err
	}

	// Resolve SHA context.
	resolved, err := resolveContext(opts.All)
	if err != nil {
		return err
	}

	// Build query from component, stack, and SHA.
	query := buildQuery(opts.Component, opts.Stack, resolved.SHA)


	// List planfiles.
	ctx := context.Background()
	files, err := store.List(ctx, query)
	if err != nil {
		return err
	}

	return renderPlanfileList(files, opts.Format, owner, repo)
}

// extractOwnerRepo extracts owner and repo from store options if available.
func extractOwnerRepo(opts planfile.StoreOptions) (string, string) {
	defer perf.Track(nil, "planfile.extractOwnerRepo")()

	owner, _ := opts.Options["owner"].(string)
	repo, _ := opts.Options["repo"].(string)
	return owner, repo
}

// renderPlanfileList formats and outputs the planfile list using pkg/list infrastructure.
// When owner or repo are non-empty, additional OWNER and REPO columns are included.
func renderPlanfileList(files []planfile.PlanfileInfo, outputFormat, owner, repo string) error {
	if len(files) == 0 {
		// No planfiles found - render empty result.
		return renderWithRenderer([]map[string]any{}, outputFormat, owner, repo)
	}

	// Convert PlanfileInfo to map[string]any for the renderer.
	data := make([]map[string]any, len(files))
	for i, f := range files {
		item := map[string]any{
			"size":          f.Size,
			"last_modified": f.LastModified.Format("2006-01-02 15:04"),
		}
		if f.Metadata != nil {
			item["stack"] = f.Metadata.Stack
			item["component"] = f.Metadata.Component
			item["sha"] = f.Metadata.SHA
		} else {
			item["stack"] = ""
			item["component"] = ""
			item["sha"] = ""
		}
		if owner != "" || repo != "" {
			item["owner"] = owner
			item["repo"] = repo
		}
		data[i] = item
	}

	return renderWithRenderer(data, outputFormat, owner, repo)
}

// renderWithRenderer uses pkg/list renderer for consistent output formatting.
func renderWithRenderer(data []map[string]any, outputFormat, owner, repo string) error {
	// Define columns for planfile listing.
	columns := []column.Config{
		{Name: "STACK", Value: "{{ .stack }}"},
		{Name: "COMPONENT", Value: "{{ .component }}"},
		{Name: "SHA", Value: "{{ .sha }}"},
		{Name: "SIZE", Value: "{{ .size }}"},
		{Name: "MODIFIED", Value: "{{ .last_modified }}"},
	}

	// Add OWNER and REPO columns when they are available from context.
	if owner != "" || repo != "" {
		columns = append(columns,
			column.Config{Name: "OWNER", Value: "{{ .owner }}"},
			column.Config{Name: "REPO", Value: "{{ .repo }}"},
		)
	}

	// Create column selector with template functions.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("failed to create column selector: %w", err)
	}

	// Map format string to format.Format type.
	var outputFmt format.Format
	switch outputFormat {
	case "json":
		outputFmt = format.FormatJSON
	case "yaml":
		outputFmt = format.FormatYAML
	case "csv":
		outputFmt = format.FormatCSV
	case "tsv":
		outputFmt = format.FormatTSV
	default:
		outputFmt = format.FormatTable
	}

	// Create renderer (no filters, no sorters, using format and default delimiter).
	r := renderer.New(nil, selector, nil, outputFmt, "")

	return r.Render(data)
}
