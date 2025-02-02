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
	"gopkg.in/yaml.v3"
)

const (
	DefaultCSVDelimiter = ","
	DefaultTSVDelimiter = "\t"
)

// VendorInfo represents a vendor configuration entry
type VendorInfo struct {
	Component string `json:"component"`
	Type      string `json:"type"`
	Manifest  string `json:"manifest"`
	Folder    string `json:"folder"`
	Version   string `json:"version"`
}

// processVendorFile processes a vendor configuration file and returns vendor information
func processVendorFile(filePath string, atmosConfig schema.AtmosConfiguration) ([]VendorInfo, error) {
	var vendors []VendorInfo

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading vendor file %s: %w", filePath, err)
	}

	var vendorConfig schema.AtmosVendorConfig
	if err := yaml.Unmarshal(data, &vendorConfig); err != nil {
		return nil, fmt.Errorf("error parsing vendor file %s: %w", filePath, err)
	}

	// Process vendor configuration
	for _, source := range vendorConfig.Spec.Sources {
		vendorType := "Vendor Manifest"
		manifest := filepath.Clean(filePath)
		folder := filepath.Join(atmosConfig.BasePath, source.Targets[0])

		vendors = append(vendors, VendorInfo{
			Component: source.Component,
			Type:      vendorType,
			Manifest:  manifest,
			Folder:    folder,
			Version:   source.Version,
		})
	}

	return vendors, nil
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
	header := []string{"Component", "Type", "Manifest", "Folder", "Version"}
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
	var allVendors []VendorInfo
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
				row[i] = vendor.Type
			case "Manifest":
				row[i] = vendor.Manifest
			case "Folder":
				row[i] = vendor.Folder
			case "Version":
				row[i] = vendor.Version
			}
		}
		rows = append(rows, row)
	}

	// Sort rows for consistent output
	sort.Slice(rows, func(i, j int) bool {
		return strings.Join(rows[i], delimiter) < strings.Join(rows[j], delimiter)
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
					if row == 0 {
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
