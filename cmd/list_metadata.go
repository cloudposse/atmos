package cmd

import (
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	u "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	utils "github.com/cloudposse/atmos/pkg/utils"
)

// listMetadataCmd lists metadata across stacks.
var listMetadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "List metadata across stacks",
	Long:  "List metadata information across all stacks",
	Example: "atmos list metadata\n" +
		"atmos list metadata --query .component\n" +
		"atmos list metadata --format json\n" +
		"atmos list metadata --stack '*-{dev,staging}-*'\n" +
		"atmos list metadata --stack 'prod-*'",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listMetadata(cmd)
		if err != nil {
			log.Error("failed to list metadata", "error", err)
			return
		}

		utils.PrintMessage(output)
	},
}

func init() {
	fl.AddCommonListFlags(listMetadataCmd)

	// Add template and function processing flags
	listMetadataCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	listMetadataCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	AddStackCompletion(listMetadataCmd)

	listCmd.AddCommand(listMetadataCmd)
}

func listMetadata(cmd *cobra.Command) (string, error) {
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", &errors.QueryError{
			Query: "common flags",
			Cause: err,
		}
	}

	// Get template and function processing flags
	processTemplates := true
	if cmd.Flags().Lookup("process-templates") != nil {
		processTemplates, err = cmd.Flags().GetBool("process-templates")
		if err != nil {
			log.Warn("failed to get process-templates flag, using default true", "error", err)
		}
	}

	processYamlFunctions := true
	if cmd.Flags().Lookup("process-functions") != nil {
		processYamlFunctions, err = cmd.Flags().GetBool("process-functions")
		if err != nil {
			log.Warn("failed to get process-functions flag, using default true", "error", err)
		}
	}

	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", &errors.InitConfigError{Cause: err}
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, processTemplates, processYamlFunctions, false, nil)
	if err != nil {
		return "", &errors.DescribeStacksError{Cause: err}
	}

	// Log the metadata query
	log.Info("Filtering metadata",
		"query", commonFlags.Query,
		"maxColumns", commonFlags.MaxColumns,
		"format", commonFlags.Format,
		"stackPattern", commonFlags.Stack,
		"processTemplates", processTemplates,
		"processYamlFunctions", processYamlFunctions)

	// Use .metadata as the default query if none provided
	if commonFlags.Query == "" {
		commonFlags.Query = ".metadata"
	}

	output, err := l.FilterAndListValues(stacksMap, &l.FilterOptions{
		Component:       "",
		Query:           commonFlags.Query,
		IncludeAbstract: false,
		MaxColumns:      commonFlags.MaxColumns,
		FormatStr:       commonFlags.Format,
		Delimiter:       commonFlags.Delimiter,
		StackPattern:    commonFlags.Stack,
	})
	if err != nil {
		if u.IsNoValuesFoundError(err) {
			return "", &errors.NoMetadataFoundError{Query: commonFlags.Query}
		}
		return "", &errors.MetadataFilteringError{Cause: err}
	}

	return output, nil
}
