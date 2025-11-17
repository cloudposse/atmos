package column

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Config defines a column with name, Go template for value extraction, and optional width.
type Config struct {
	Name  string `yaml:"name" json:"name" mapstructure:"name"`    // Display header
	Value string `yaml:"value" json:"value" mapstructure:"value"` // Go template string
	Width int    `yaml:"width" json:"width" mapstructure:"width"` // Optional width override
}

// Selector manages column extraction with template evaluation.
// Templates are evaluated during Extract(), not at config load time.
type Selector struct {
	configs     []Config
	selected    []string           // Column names to display (nil = all)
	templateMap *template.Template // Pre-parsed templates with FuncMap
}

// TemplateContext provides data available to column templates during evaluation.
type TemplateContext struct {
	// Standard fields available in all templates
	AtmosComponent     string `json:"atmos_component"`
	AtmosStack         string `json:"atmos_stack"`
	AtmosComponentType string `json:"atmos_component_type"`

	// Component configuration
	Vars     map[string]any `json:"vars"`
	Settings map[string]any `json:"settings"`
	Metadata map[string]any `json:"metadata"`
	Env      map[string]any `json:"env"`

	// Flags
	Enabled  bool `json:"enabled"`
	Locked   bool `json:"locked"`
	Abstract bool `json:"abstract"`

	// Full raw data for advanced templates
	Raw map[string]any `json:"raw"`
}

// NewSelector creates a selector with Go template support.
// Templates are pre-parsed for performance but NOT evaluated until Extract().
// funcMap should include functions like atmos.Component, toString, get, etc.
func NewSelector(configs []Config, funcMap template.FuncMap) (*Selector, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("%w: no columns configured", errUtils.ErrInvalidConfig)
	}

	// Pre-parse all templates with function map
	tmplMap := template.New("columns").Funcs(funcMap)
	for _, cfg := range configs {
		if cfg.Name == "" {
			return nil, fmt.Errorf("%w: column name cannot be empty", errUtils.ErrInvalidConfig)
		}
		if cfg.Value == "" {
			return nil, fmt.Errorf("%w: column %q has empty value template", errUtils.ErrInvalidConfig, cfg.Name)
		}

		// Parse each column template
		_, err := tmplMap.New(cfg.Name).Parse(cfg.Value)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid template for column %q: %v", errUtils.ErrInvalidConfig, cfg.Name, err)
		}
	}

	return &Selector{
		configs:     configs,
		selected:    nil, // nil = all columns
		templateMap: tmplMap,
	}, nil
}

// Select restricts which columns to display.
// Pass nil or empty slice to display all columns.
func (s *Selector) Select(columnNames []string) error {
	if len(columnNames) == 0 {
		s.selected = nil
		return nil
	}

	// Validate all requested columns exist
	configMap := make(map[string]bool)
	for _, cfg := range s.configs {
		configMap[cfg.Name] = true
	}

	for _, name := range columnNames {
		if !configMap[name] {
			return fmt.Errorf("%w: column %q not found in configuration", errUtils.ErrInvalidConfig, name)
		}
	}

	s.selected = columnNames
	return nil
}

// Extract evaluates templates against data and returns table rows.
// ⚠️ CRITICAL: This is where Go template evaluation happens (NOT at config load).
// Each row is processed with its full data as template context.
func (s *Selector) Extract(data []map[string]any) (headers []string, rows [][]string, err error) {
	if len(data) == 0 {
		return s.Headers(), [][]string{}, nil
	}

	headers = s.Headers()

	// Get configs for selected columns (or all if no selection)
	selectedConfigs := s.getSelectedConfigs()

	// Process each data item
	for i, item := range data {
		row := make([]string, len(selectedConfigs))

		for j, cfg := range selectedConfigs {
			// Evaluate template for this column and data item
			value, evalErr := s.evaluateTemplate(cfg, item)
			if evalErr != nil {
				return nil, nil, fmt.Errorf("%w: row %d, column %q: %v", errUtils.ErrTemplateEvaluation, i, cfg.Name, evalErr)
			}
			row[j] = value
		}

		rows = append(rows, row)
	}

	return headers, rows, nil
}

// Headers returns the header row based on selected columns.
func (s *Selector) Headers() []string {
	selectedConfigs := s.getSelectedConfigs()
	headers := make([]string, len(selectedConfigs))
	for i, cfg := range selectedConfigs {
		headers[i] = cfg.Name
	}
	return headers
}

// getSelectedConfigs returns configs for selected columns (or all if no selection).
func (s *Selector) getSelectedConfigs() []Config {
	if len(s.selected) == 0 {
		return s.configs
	}

	// Build map for quick lookup
	selectedMap := make(map[string]bool)
	for _, name := range s.selected {
		selectedMap[name] = true
	}

	// Filter configs to selected columns in original order
	var configs []Config
	for _, cfg := range s.configs {
		if selectedMap[cfg.Name] {
			configs = append(configs, cfg)
		}
	}

	return configs
}

// evaluateTemplate evaluates a column template against data item.
func (s *Selector) evaluateTemplate(cfg Config, data map[string]any) (string, error) {
	// Get the pre-parsed template for this column
	tmpl := s.templateMap.Lookup(cfg.Name)
	if tmpl == nil {
		return "", fmt.Errorf("template %q not found", cfg.Name)
	}

	// Build template context from data
	context := buildTemplateContext(data)

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// buildTemplateContext creates template context from raw data.
// Maps common field names and makes full data available via .Raw.
func buildTemplateContext(data map[string]any) any {
	// Try to map to structured context for better template readability
	ctx := make(map[string]any)

	// Copy all data to .Raw for full access
	ctx["raw"] = data

	// Map common fields to top-level for convenience
	if v, ok := data["atmos_component"]; ok {
		ctx["atmos_component"] = v
	}
	if v, ok := data["atmos_stack"]; ok {
		ctx["atmos_stack"] = v
	}
	if v, ok := data["atmos_component_type"]; ok {
		ctx["atmos_component_type"] = v
	}
	if v, ok := data["vars"]; ok {
		ctx["vars"] = v
	}
	if v, ok := data["settings"]; ok {
		ctx["settings"] = v
	}
	if v, ok := data["metadata"]; ok {
		ctx["metadata"] = v
	}
	if v, ok := data["env"]; ok {
		ctx["env"] = v
	}
	if v, ok := data["enabled"]; ok {
		ctx["enabled"] = v
	}
	if v, ok := data["locked"]; ok {
		ctx["locked"] = v
	}
	if v, ok := data["abstract"]; ok {
		ctx["abstract"] = v
	}

	// For workflow data
	if v, ok := data["file"]; ok {
		ctx["file"] = v
	}
	if v, ok := data["name"]; ok {
		ctx["name"] = v
	}
	if v, ok := data["description"]; ok {
		ctx["description"] = v
	}
	if v, ok := data["steps"]; ok {
		ctx["steps"] = v
	}

	// For stack data
	if v, ok := data["stack"]; ok {
		ctx["stack"] = v
	}
	if v, ok := data["components"]; ok {
		ctx["components"] = v
	}

	// For vendor data
	if v, ok := data["atmos_vendor_type"]; ok {
		ctx["atmos_vendor_type"] = v
	}
	if v, ok := data["atmos_vendor_file"]; ok {
		ctx["atmos_vendor_file"] = v
	}
	if v, ok := data["atmos_vendor_target"]; ok {
		ctx["atmos_vendor_target"] = v
	}

	// Also allow direct access to unmapped fields
	for k, v := range data {
		if _, exists := ctx[k]; !exists {
			ctx[k] = v
		}
	}

	return ctx
}

// BuildColumnFuncMap returns template functions for column templates.
// These functions are safe for use in column value extraction.
func BuildColumnFuncMap() template.FuncMap {
	return template.FuncMap{
		// Type conversion
		"toString": toString,
		"toInt":    toInt,
		"toBool":   toBool,

		// Formatting
		"truncate": truncate,
		"pad":      pad,
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,

		// Data access
		"get":   mapGet,
		"getOr": mapGetOr,
		"has":   mapHas,

		// Collections
		"len":   length,
		"join":  strings.Join,
		"split": strings.Split,

		// Conditional
		"ternary": ternary,
	}
}

// Template function implementations.

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "yes" || val == "1"
	case int:
		return val != 0
	default:
		return false
	}
}

func truncate(s string, length any) string {
	// Convert length to int
	var l int
	switch v := length.(type) {
	case int:
		l = v
	case int64:
		l = int(v)
	case float64:
		l = int(v)
	default:
		return s
	}

	if len(s) <= l {
		return s
	}
	if l <= 0 {
		return ""
	}
	if l <= 3 {
		return s[:l]
	}
	return s[:l-3] + "..."
}

func pad(s string, length int) string {
	if len(s) >= length {
		return s
	}
	return s + strings.Repeat(" ", length-len(s))
}

func mapGet(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

func mapGetOr(m map[string]any, key string, defaultVal any) any {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		return v
	}
	return defaultVal
}

func mapHas(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func length(v any) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []any:
		return len(val)
	case map[string]any:
		return len(val)
	default:
		return 0
	}
}

func ternary(condition bool, trueVal, falseVal any) any {
	if condition {
		return trueVal
	}
	return falseVal
}
