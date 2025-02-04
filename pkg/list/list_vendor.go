package list

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultCSVDelimiter = ","
	DefaultTSVDelimiter = "\t"
)

// processVendorFile processes a vendor configuration file and returns vendor information
func processVendorFile(filePath string, atmosConfig schema.AtmosConfiguration) ([]schema.AtmosVendorSource, error) {
	// Use the existing vendoring logic to process the file and its imports
	vendorConfig, _, _, err := exec.ReadAndProcessVendorConfigFile(atmosConfig, filePath, false)
	if err != nil {
		return nil, fmt.Errorf("error processing vendor file %s: %w", filePath, err)
	}

	// Process all sources from the main config and its imports
	mergedSources, _, err := exec.ProcessVendorImports(atmosConfig, filePath, vendorConfig.Spec.Imports, vendorConfig.Spec.Sources, []string{})
	if err != nil {
		return nil, fmt.Errorf("error processing vendor imports: %w", err)
	}

	// Get vendor base path
	var vendorBasePath string
	if utils.IsPathAbsolute(atmosConfig.Vendor.BasePath) {
		vendorBasePath = atmosConfig.Vendor.BasePath
	} else {
		vendorBasePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Vendor.BasePath)
	}

	// Get relative path from vendor base path's parent directory
	relPath, err := filepath.Rel(filepath.Dir(vendorBasePath), filePath)
	if err != nil {
		return nil, fmt.Errorf("error getting relative path for vendor file: %w", err)
	}

	// Process templates in sources and targets
	for i := range mergedSources {
		// Set the File field with the relative path, preserving the vendor directory
		mergedSources[i].File = filepath.ToSlash(relPath)

		// Process templates in the target path
		if len(mergedSources[i].Targets) > 0 {
			processedTarget, err := exec.ProcessTmpl(
				"target",
				mergedSources[i].Targets[0],
				map[string]string{
					"Component": mergedSources[i].Component,
					"Version":   mergedSources[i].Version,
				},
				false,
			)
			if err != nil {
				return nil, fmt.Errorf("error processing target template: %w", err)
			}
			mergedSources[i].Targets[0] = processedTarget
		}

		// Process templates in the source URI
		processedSource, err := exec.ProcessTmpl(
			"source",
			mergedSources[i].Source,
			map[string]string{
				"Component": mergedSources[i].Component,
				"Version":   mergedSources[i].Version,
			},
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("error processing source template: %w", err)
		}
		mergedSources[i].Source = processedSource
	}

	return mergedSources, nil
}

// FilterAndListVendors lists vendor configurations based on the provided configuration
func FilterAndListVendors(listConfig schema.ListConfig, format string, delimiter string) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
	}

	// Set default delimiters based on format
	if format == FormatCSV && delimiter == DefaultTSVDelimiter {
		delimiter = DefaultCSVDelimiter
	}

	if format == "" && listConfig.Format != "" {
		if err := ValidateFormat(listConfig.Format); err != nil {
			return "", err
		}
		format = listConfig.Format
	}

	// Initialize Atmos config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", fmt.Errorf("error initializing CLI config: %w", err)
	}

	// Define default columns if not specified in config
	header := []string{"Component", "Type", "Manifest", "Folder", "Version", "Source"}
	if len(listConfig.Columns) > 0 {
		header = make([]string, len(listConfig.Columns))
		for i, col := range listConfig.Columns {
			header[i] = col.Name
		}
	}

	// Get vendor path
	var vendorPath string
	if utils.IsPathAbsolute(atmosConfig.Vendor.BasePath) {
		vendorPath = atmosConfig.Vendor.BasePath
	} else {
		vendorPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Vendor.BasePath)
	}

	// Check if vendor path exists
	fileInfo, err := os.Stat(vendorPath)
	if err != nil {
		return "", fmt.Errorf("the vendor path '%s' does not exist. Review 'vendor.base_path' in 'atmos.yaml'", vendorPath)
	}

	var files []string
	if fileInfo.IsDir() {
		// If it's a directory, get all YAML files
		files, err = utils.GetAllYamlFilesInDir(vendorPath)
		if err != nil {
			return "", fmt.Errorf("error reading the directory '%s' defined in 'vendor.base_path' in 'atmos.yaml': %v",
				atmosConfig.Vendor.BasePath, err)
		}
		// Convert relative paths to absolute paths
		for i, f := range files {
			files[i] = filepath.Join(vendorPath, f)
		}
	} else {
		// If it's a file, just use that file
		files = []string{vendorPath}
	}

	// Process all vendor files
	var allVendors []schema.AtmosVendorSource
	for _, f := range files {
		vendors, err := processVendorFile(f, atmosConfig)
		if err != nil {
			return "", err
		}
		allVendors = append(allVendors, vendors...)
	}

	// Convert vendor info to rows based on header columns
	var rows [][]string
	for _, vendor := range allVendors {
		row := make([]string, len(header))
		for i, col := range header {
			switch col {
			case "Component":
				row[i] = vendor.Component
			case "Type":
				row[i] = "Vendor Manifest"
			case "Manifest":
				row[i] = vendor.File
			case "Folder":
				if len(vendor.Targets) > 0 {
					row[i] = filepath.Join(atmosConfig.BasePath, vendor.Targets[0])
				}
			case "Version":
				row[i] = vendor.Version
			case "Source":
				row[i] = vendor.Source
			}
		}
		rows = append(rows, row)
	}

	// Sort rows for consistent output
	sort.Slice(rows, func(i, j int) bool {
		// Compare each column in order until we find a difference
		for col := 0; col < len(rows[i]); col++ {
			if rows[i][col] != rows[j][col] {
				return rows[i][col] < rows[j][col]
			}
		}
		return false // rows are identical
	})

	if len(rows) == 0 {
		return "No vendor configurations found", nil
	}

	// Handle different output formats
	switch format {
	case FormatJSON:
		jsonBytes, err := json.MarshalIndent(allVendors, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error formatting JSON output: %w", err)
		}
		return string(jsonBytes), nil

	case FormatCSV, FormatTSV:
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil

	default:
		// If format is empty or "table", use table format
		if format == "" && exec.CheckTTYSupport() {
			// Create a styled table for TTY
			t := table.New().
				Border(lipgloss.ThickBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
				StyleFunc(func(row, col int) lipgloss.Style {
					style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
					if row == -1 {
						return style.Inherit(theme.Styles.CommandName).Align(lipgloss.Center)
					}
					return style.Inherit(theme.Styles.Description)
				}).
				Headers(header...).
				Rows(rows...)

			return t.String() + utils.GetLineEnding(), nil
		}

		// Default to simple tabular format for non-TTY or when format is explicitly "table"
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil
	}
}
