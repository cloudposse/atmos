package cache

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/utils"
)

var cacheListParser *flags.StandardParser

// tableHeaders defines the columns shown in table/csv/tsv output.
var tableHeaders = []string{"Key", "Size", "Created"}

// cacheListCmd lists cache entries.
var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List CI cache entries",
	Long: `List CI cache entries, optionally filtered by key prefix.

Uses the CI provider's cache API. Newest entries are listed first.`,
	Args: cobra.NoArgs,
	RunE: runCacheList,
}

func runCacheList(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := cacheListParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	prefix := v.GetString(fieldKey)
	formatStr := v.GetString("format")
	delimiter := v.GetString("delimiter")
	maxColumns := v.GetInt("max-columns")

	if formatStr == "" {
		formatStr = string(format.FormatTable)
	}
	if err := format.ValidateFormat(formatStr); err != nil {
		return err
	}

	manager, _, err := cacheSetup(cmd, cacheOverrides{})
	if err != nil {
		return err
	}

	entries, err := manager.List(cmd.Context(), prefix)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		ui.Info("No cache entries found")
		return nil
	}

	output, err := formatCacheOutput(entries, formatStr, delimiter, maxColumns)
	if err != nil {
		return err
	}
	return data.Write(output)
}

// formatCacheOutput renders cache entries according to the requested format.
func formatCacheOutput(entries []cachepkg.Entry, formatStr, delimiter string, maxColumns int) (string, error) {
	isTTY := term.IsTTYSupportForStdout()

	switch format.Format(formatStr) {
	case format.FormatJSON, format.FormatYAML:
		// For structured formats include all fields (Key, Size, Created, ID).
		rows := buildCacheFullRows(entries)
		dataMap := buildCacheDataMap(rows)
		f, err := format.NewFormatter(format.Format(formatStr))
		if err != nil {
			return "", err
		}
		opts := format.FormatOptions{
			Format:     format.Format(formatStr),
			TTY:        isTTY,
			MaxColumns: maxColumns,
		}
		return f.Format(dataMap, opts)

	case format.FormatCSV:
		del := delimiter
		if del == "" {
			del = format.DefaultCSVDelimiter
		}
		return buildCacheCSVTSV(entries, del), nil

	case format.FormatTSV:
		del := delimiter
		if del == "" {
			del = format.DefaultTSVDelimiter
		}
		return buildCacheCSVTSV(entries, del), nil

	default:
		// table format: TTY → styled, non-TTY → plain TSV.
		rows := buildCacheTableRows(entries)
		if isTTY {
			return format.CreateStyledTable(tableHeaders, rows), nil
		}
		// Non-TTY: plain tab-separated output, no header.
		del := delimiter
		if del == "" {
			del = format.DefaultTSVDelimiter
		}
		return buildCacheCSVTSV(entries, del), nil
	}
}

// buildCacheTableRows converts entries to string rows for the Key/Size/Created table.
func buildCacheTableRows(entries []cachepkg.Entry) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{
			e.Key,
			fmt.Sprintf("%d", e.Size),
			formatCreatedAt(e.CreatedAt),
		})
	}
	return rows
}

// buildCacheFullRows converts entries to maps that include all four fields.
func buildCacheFullRows(entries []cachepkg.Entry) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, map[string]interface{}{
			"key":     e.Key,
			"size":    e.Size,
			"created": formatCreatedAt(e.CreatedAt),
			"id":      e.ID,
		})
	}
	return rows
}

// buildCacheDataMap converts the full-row slice to a map keyed by entry key for use
// with the format.Formatter interface (which expects map[string]interface{}).
func buildCacheDataMap(rows []map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(rows))
	for i, row := range rows {
		key, _ := row["key"].(string)
		if key == "" {
			key = fmt.Sprintf("entry_%d", i)
		}
		m[key] = row
	}
	return m
}

// buildCacheCSVTSV builds a delimited string (CSV or TSV) from entries.
func buildCacheCSVTSV(entries []cachepkg.Entry, delimiter string) string {
	var b strings.Builder
	lineEnding := utils.GetLineEnding()
	// Header row.
	b.WriteString(strings.Join(tableHeaders, delimiter) + lineEnding)
	// Data rows.
	for _, e := range entries {
		b.WriteString(e.Key + delimiter +
			fmt.Sprintf("%d", e.Size) + delimiter +
			formatCreatedAt(e.CreatedAt) + lineEnding)
	}
	return b.String()
}

// formatCreatedAt formats a CreatedAt timestamp for display.
func formatCreatedAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func init() {
	cacheListParser = flags.NewStandardParser(
		flags.WithStringFlag(fieldKey, "k", "", "Filter entries by key prefix"),
		flags.WithStringFlag("format", "", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringFlag("delimiter", "", "", "Delimiter for csv/tsv output"),
		flags.WithIntFlag("max-columns", "", 0, "Maximum number of columns to display in table format"),
		flags.WithEnvVars(fieldKey, "ATMOS_CI_CACHE_KEY"),
		flags.WithEnvVars("format", "ATMOS_CI_CACHE_FORMAT"),
		flags.WithEnvVars("delimiter", "ATMOS_DELIMITER"),
		flags.WithEnvVars("max-columns", "ATMOS_MAX_COLUMNS"),
	)
	cacheListParser.RegisterFlags(cacheListCmd)
	if err := cacheListParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
