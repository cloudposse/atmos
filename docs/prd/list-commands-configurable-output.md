# PRD: List Commands UI Overhaul

## Document Information

- **Status**: Draft
- **Created**: 2025-01-16
- **Author**: Claude (Conductor Agent)
- **Related Issues**:
  - [DEV-2803: Implement `atmos list deployments`](https://linear.app/cloudposse/issue/DEV-2803)
  - [DEV-2805: Improve `atmos list components`](https://linear.app/cloudposse/issue/DEV-2805)
  - [DEV-2806: Implement `atmos list vendor`](https://linear.app/cloudposse/issue/DEV-2806)

## Executive Summary

Modernize all Atmos list commands to support configurable column customization, universal filtering and sorting, and theme-aware output. Create highly reusable, well-tested infrastructure that eliminates code duplication across the 10 list commands by implementing generic utilities tested once and used everywhere.

### Key Goals

1. **Reusable Infrastructure**: Create generic filter/sort/column/render logic tested once (>90% coverage) and reused across all commands
2. **Proper Architecture**: Leverage existing UI/data layer (TTY handled automatically), maintain zero deep exits
3. **Feature Complete**: Implement column customization, filtering, sorting for all list commands
4. **Documentation Alignment**: Ensure all documented features are implemented (fix current documentation-implementation gap)
5. **High Test Coverage**: 80-90% coverage across all code (90%+ on reusables, 80%+ on commands)

## Problem Statement

### Current Issues

1. **Documentation-Implementation Gap**: Column customization is extensively documented but only implemented for `workflows` and `vendor` commands. The `stacks`, `components`, and other commands lack schema fields and implementation.

2. **Code Duplication**: Format handling, filtering, sorting, and output logic is duplicated across multiple list commands, making maintenance difficult and bug fixes inconsistent.

3. **Inconsistent Output**: Different list commands use different output patterns - some use `u.PrintMessageInColor()` (anti-pattern), others properly use data/ui layer.

4. **Limited Capabilities**:
   - No universal filter/sort support across commands
   - No column selection/ordering
   - No conditional styling (disabled=gray, locked=orange)
   - Filtering flags inconsistent across commands

5. **User Experience**: Teams exploring cloud architecture data model need better tools to query, list, and view infrastructure across stacks and components.

## Architecture Overview

### Core Principles

1. **Separation of Concerns**: Data fetching → Transformation → Rendering → Output
2. **Pure Functions**: Maximize testability, minimize side effects
3. **Reusable First**: Generic utilities in `pkg/list/`, command-specific logic in `cmd/list/`
4. **Leverage Existing Infrastructure**: Use `data.*` and `ui.*` methods (TTY handled automatically)
5. **No Deep Exits**: All functions return errors (already achieved, must maintain)

### Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│ 1. User Input (CLI flags + atmos.yaml config)              │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Data Fetching (command-specific)                        │
│    - Load atmos config                                      │
│    - Execute describe stacks / workflows / etc.             │
│    - Returns: map[string]any or []WorkflowRow               │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Filtering (pkg/list/filter - REUSABLE)                  │
│    - YQ expressions                                         │
│    - Glob patterns                                          │
│    - Column value filters                                   │
│    - Boolean filters (enabled/locked)                       │
│    - Returns: filtered data                                 │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Column Extraction (pkg/list/column - REUSABLE)          │
│    ⚠️  CRITICAL: Go template evaluation happens HERE        │
│    - Parse column configs from atmos.yaml                   │
│    - Evaluate Go templates against each row of data         │
│    - Template context: full component/stack configuration   │
│    - Returns: [][]string (headers + rows)                   │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Sorting (pkg/list/sort - REUSABLE)                      │
│    - Single or multi-column sorting                         │
│    - Type-aware (string, number, date, boolean)             │
│    - Returns: sorted [][]string                             │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Rendering (pkg/list/renderer - REUSABLE)                │
│    - Orchestrates steps 3-5                                 │
│    - Applies conditional styling (disabled, locked, etc.)   │
│    - Delegates to format-specific formatter                 │
│    - Returns: string (formatted output)                     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. Output (pkg/list/output - REUSABLE)                     │
│    - Routes to data.Write() (stdout) for structured formats │
│    - Routes to ui.Write() (stderr) for human-readable       │
│    - TTY detection handled by ui/data layer                 │
└─────────────────────────────────────────────────────────────┘
```

### Template Evaluation Timing ⚠️

**CRITICAL DISTINCTION:**

```yaml
# atmos.yaml - Configuration loaded at startup
components:
  list:
    columns:
      - name: Component
        value: '{{ .atmos_component }}'  # ← Template string stored, NOT evaluated
      - name: Region
        value: '{{ .vars.region }}'      # ← Template string stored, NOT evaluated
```

**When Config is Loaded:**
- ✅ Parse YAML structure
- ✅ Store template strings as-is
- ❌ Do NOT evaluate templates (no data available yet)

**When Rows are Processed:**
- ✅ For each component/stack/workflow
- ✅ Create template context with full data: `.atmos_component`, `.vars`, `.settings`, etc.
- ✅ Evaluate template against context
- ✅ Extract column value
- ✅ Build row: `[]string{component_name, region_value, ...}`

```go
// Pseudocode for template evaluation
for _, item := range data {
    row := []string{}
    for _, columnConfig := range listConfig.Columns {
        // Create template context with full item data
        context := map[string]any{
            "atmos_component": item.Component,
            "atmos_stack":     item.Stack,
            "vars":            item.Vars,
            "settings":        item.Settings,
            "enabled":         item.Enabled,
            "locked":          item.Locked,
            // ... all available fields
        }

        // Evaluate template NOW (not at config load time)
        tmpl := template.New("column").Parse(columnConfig.Value)
        var buf bytes.Buffer
        tmpl.Execute(&buf, context)

        row = append(row, buf.String())
    }
    rows = append(rows, row)
}
```

## Detailed Design

### Phase 1: Reusable Infrastructure

#### 1.1 Column System (`pkg/list/column/`)

**Purpose**: Manage column configuration and Go template evaluation during row processing.

**Files**:
- `column.go` - Core column extraction logic
- `column_test.go` - Test coverage target: >90%

**Key Types**:

```go
// Config matches schema from atmos.yaml
type Config struct {
    Name  string `yaml:"name" json:"name"`    // Display header
    Value string `yaml:"value" json:"value"`  // Go template string
    Width int    `yaml:"width" json:"width"`  // Optional width override
}

// Selector manages column extraction with template evaluation
type Selector struct {
    configs     []Config
    selected    []string           // Column names to display (nil = all)
    templateMap *template.Template // Pre-parsed templates with FuncMap
}

// Template context provided to each template evaluation
type TemplateContext struct {
    // Standard fields available in all templates
    AtmosComponent     string                 `json:"atmos_component"`
    AtmosStack         string                 `json:"atmos_stack"`
    AtmosComponentType string                 `json:"atmos_component_type"`

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
```

**Public API**:

```go
// NewSelector creates a selector with Go template support
// funcMap should include Atmos template functions (atmos.Component, etc.)
func NewSelector(configs []Config, funcMap template.FuncMap) (*Selector, error)

// Select restricts which columns to display (nil = all)
func (s *Selector) Select(columnNames []string) error

// Extract evaluates templates against data and returns table rows
// ⚠️  This is where Go template evaluation happens (NOT at config load)
func (s *Selector) Extract(data []map[string]any) (headers []string, rows [][]string, err error)

// Headers returns the header row
func (s *Selector) Headers() []string
```

**Template Function Map**:

```go
// BuildColumnFuncMap returns template functions for column templates
func BuildColumnFuncMap() template.FuncMap {
    return template.FuncMap{
        // Type conversion
        "toString":   toString,
        "toInt":      toInt,
        "toBool":     toBool,

        // Formatting
        "truncate":   truncate,
        "pad":        pad,
        "upper":      strings.ToUpper,
        "lower":      strings.ToLower,

        // Data access
        "get":        mapGet,        // Safe nested map access
        "getOr":      mapGetOr,      // With default value
        "has":        mapHas,        // Check if key exists

        // Collections
        "len":        length,
        "join":       strings.Join,
        "split":      strings.Split,

        // Conditional
        "ternary":    ternary,       // {{ ternary .enabled "yes" "no" }}

        // Include standard Gomplate functions if needed
        // (may need to restrict to safe subset)
    }
}
```

**Example Usage**:

```go
// From atmos.yaml
configs := []column.Config{
    {Name: "Component", Value: "{{ .atmos_component }}"},
    {Name: "Region", Value: "{{ .vars.region }}"},
    {Name: "Enabled", Value: "{{ ternary .enabled \"✓\" \"✗\" }}"},
}

// Create selector with template functions
selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())

// Data from component processing
data := []map[string]any{
    {
        "atmos_component": "vpc",
        "atmos_stack":     "plat-ue2-dev",
        "vars":            map[string]any{"region": "us-east-2"},
        "enabled":         true,
    },
    // ... more items
}

// Extract evaluates templates NOW (during row processing)
headers, rows, err := selector.Extract(data)
// headers: ["Component", "Region", "Enabled"]
// rows: [["vpc", "us-east-2", "✓"], ...]
```

**Test Coverage**:
- Template parsing and validation
- Template evaluation with various data types
- Template function map functions
- Error handling (invalid templates, missing fields)
- Column selection/filtering
- Width calculation
- Edge cases (nil values, nested maps, arrays)
- Target: >90%

#### 1.2 Filter System (`pkg/list/filter/`)

**Purpose**: Universal filtering for any data structure.

**Files**:
- `filter.go` - Filter implementations
- `filter_test.go` - Test coverage target: >90%

**Key Types**:

```go
// Filter interface for composability
type Filter interface {
    Apply(data interface{}) (interface{}, error)
}

// YQFilter uses yq expressions for filtering
type YQFilter struct {
    Query string
}

// GlobFilter matches patterns (e.g., "plat-*-dev")
type GlobFilter struct {
    Pattern string
}

// ColumnValueFilter filters rows by column value
type ColumnValueFilter struct {
    Column string
    Value  string
}

// BoolFilter filters by boolean field
type BoolFilter struct {
    Field string
    Value *bool  // nil = all, true = enabled only, false = disabled only
}

// Chain combines multiple filters (AND logic)
type Chain struct {
    filters []Filter
}
```

**Public API**:

```go
// Factory functions
func NewYQFilter(query string) (*YQFilter, error)
func NewGlobFilter(pattern string) (*GlobFilter, error)
func NewColumnFilter(column, value string) *ColumnValueFilter
func NewBoolFilter(field string, value *bool) *BoolFilter

// Apply filters data
func (f *YQFilter) Apply(data interface{}) (interface{}, error)
func (f *GlobFilter) Apply(data interface{}) (interface{}, error)
func (f *ColumnValueFilter) Apply(data interface{}) (interface{}, error)
func (f *BoolFilter) Apply(data interface{}) (interface{}, error)

// Chain combines filters
func NewChain(filters ...Filter) *Chain
func (c *Chain) Apply(data interface{}) (interface{}, error)
```

**Test Coverage**:
- Each filter type independently
- Filter chains (multiple filters)
- Edge cases (empty data, nil values)
- Error handling (invalid queries, type mismatches)
- Target: >90%

#### 1.3 Sort System (`pkg/list/sort/`)

**Purpose**: Universal sorting for any column, any direction.

**Files**:
- `sort.go` - Sort implementations
- `sort_test.go` - Test coverage target: >90%

**Key Types**:

```go
type Order int

const (
    Ascending Order = iota
    Descending
)

type DataType int

const (
    String DataType = iota
    Number
    Date
    Boolean
)

// Sorter handles single column sorting
type Sorter struct {
    Column   string
    Order    Order
    DataType DataType  // Auto-detected if not specified
}

// MultiSorter handles multi-column sorting
type MultiSorter struct {
    sorters []*Sorter
}
```

**Public API**:

```go
// NewSorter creates a sorter for a single column
func NewSorter(column string, order Order) *Sorter

// WithDataType sets explicit data type (otherwise auto-detected)
func (s *Sorter) WithDataType(dt DataType) *Sorter

// Sort sorts rows in-place by the column
func (s *Sorter) Sort(rows [][]string, headers []string) error

// NewMultiSorter creates a multi-column sorter
func NewMultiSorter(sorters ...*Sorter) *MultiSorter

// Sort applies all sorters in order (primary, secondary, etc.)
func (ms *MultiSorter) Sort(rows [][]string, headers []string) error

// ParseSortSpec parses CLI sort spec (e.g., "stack:asc,component:desc")
func ParseSortSpec(spec string) ([]*Sorter, error)
```

**Test Coverage**:
- Single column sorting (ascending/descending)
- Multi-column sorting (with precedence)
- Type-aware sorting (numeric vs lexicographic)
- Date parsing and sorting
- Boolean sorting
- Edge cases (empty rows, missing columns)
- Target: >90%

#### 1.4 Renderer (`pkg/list/renderer/`)

**Purpose**: Orchestrate the full rendering pipeline.

**Files**:
- `renderer.go` - Pipeline orchestration
- `renderer_test.go` - Test coverage target: >90%

**Key Types**:

```go
// RowStyleFunc provides conditional styling
type RowStyleFunc func(row []string, rowIndex int, headers []string) lipgloss.Style

// Options configure the rendering pipeline
type Options struct {
    Format       format.Format   // table, json, yaml, csv, tsv
    Columns      []column.Config // From atmos.yaml or CLI
    Filters      []filter.Filter // Applied before column extraction
    Sorters      []*sort.Sorter  // Applied after column extraction
    Delimiter    string          // For CSV/TSV
    StyleFunc    RowStyleFunc    // Conditional styling for tables
    ColumnWidths map[string]int  // Custom column widths
}

// Renderer executes the rendering pipeline
type Renderer struct {
    data    interface{}
    options Options
}
```

**Public API**:

```go
// New creates a renderer
func New(data interface{}, opts Options) *Renderer

// Render executes the full pipeline
func (r *Renderer) Render() (string, error)
```

**Internal Pipeline**:

```go
// Pipeline execution order
func (r *Renderer) Render() (string, error) {
    // 1. Apply filters to raw data
    if err := r.applyFilters(); err != nil {
        return "", err
    }

    // 2. Extract columns (⚠️ Go template evaluation happens here)
    if err := r.extractColumns(); err != nil {
        return "", err
    }

    // 3. Apply sorting to extracted rows
    if err := r.applySort(); err != nil {
        return "", err
    }

    // 4. Format output (table, json, yaml, csv, tsv)
    return r.formatOutput()
}
```

**Test Coverage**:
- Full pipeline (filter → columns → sort → format)
- Each format type (table, json, yaml, csv, tsv)
- Conditional styling
- Error propagation
- Edge cases (empty data, no columns configured)
- Target: >90%

#### 1.5 Output Manager (`pkg/list/output/`)

**Purpose**: Route output to correct stream using data/ui layer.

**Files**:
- `output.go` - Output routing
- `output_test.go` - Test coverage target: >90%

**Key Types**:

```go
// Manager routes output to data or ui layer
type Manager struct {
    format format.Format
}
```

**Public API**:

```go
// New creates an output manager
func New(format format.Format) *Manager

// Write routes to data.Write() or ui.Write() based on format
func (m *Manager) Write(content string) error {
    if m.format.IsStructured() {  // JSON, YAML, CSV, TSV
        return data.Write(content)  // → stdout (pipeable)
    }
    return ui.Write(content)  // → stderr (human readable, TTY-aware)
}
```

**Test Coverage**:
- Output routing for each format
- Format detection
- Target: >90%

### Phase 2: Schema & Configuration

#### 2.1 Schema Updates (`pkg/schema/schema.go`)

**Add `List` field to structs**:

```go
type Stacks struct {
    NamePattern     string     `yaml:"name_pattern" json:"name_pattern" mapstructure:"name_pattern"`
    NameTemplate    string     `yaml:"name_template" json:"name_template" mapstructure:"name_template"`
    IncludedPaths   []string   `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
    ExcludedPaths   []string   `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
    List            ListConfig `yaml:"list" json:"list" mapstructure:"list"`  // NEW
}

type Components struct {
    Terraform ComponentsSection `yaml:"terraform" json:"terraform" mapstructure:"terraform"`
    Helmfile  ComponentsSection `yaml:"helmfile" json:"helmfile" mapstructure:"helmfile"`
    List      ListConfig        `yaml:"list" json:"list" mapstructure:"list"`  // NEW
}

// Workflows and Vendor already have List field ✅
```

#### 2.2 Enhanced ListConfig

```go
type ListConfig struct {
    Format  string             `yaml:"format" json:"format" mapstructure:"format"`
    Columns []ListColumnConfig `yaml:"columns" json:"columns" mapstructure:"columns"`
    Sort    []SortConfig       `yaml:"sort" json:"sort" mapstructure:"sort"`  // NEW
}

type ListColumnConfig struct {
    Name  string `yaml:"name" json:"name" mapstructure:"name"`
    Value string `yaml:"value" json:"value" mapstructure:"value"`
    Width int    `yaml:"width" json:"width" mapstructure:"width"`  // NEW
}

type SortConfig struct {
    Column string `yaml:"column" json:"column" mapstructure:"column"`
    Order  string `yaml:"order" json:"order" mapstructure:"order"`  // "asc" or "desc"
}
```

#### 2.3 Configuration Example

```yaml
# atmos.yaml
components:
  list:
    format: table
    columns:
      - name: Component
        value: '{{ .atmos_component }}'
        width: 30
      - name: Type
        value: '{{ .atmos_component_type }}'
      - name: Stack
        value: '{{ .atmos_stack }}'
        width: 25
      - name: Region
        value: '{{ .vars.region }}'
      - name: Enabled
        value: '{{ ternary .enabled "✓" "✗" }}'
      - name: Locked
        value: '{{ ternary .locked "🔒" "" }}'
    sort:
      - column: Stack
        order: asc
      - column: Component
        order: asc

stacks:
  list:
    columns:
      - name: Stack
        value: '{{ .stack }}'
      - name: Terraform Components
        value: '{{ len .components.terraform }}'
      - name: Helmfile Components
        value: '{{ len .components.helmfile }}'

workflows:
  list:
    columns:
      - name: File
        value: '{{ .file }}'
      - name: Workflow
        value: '{{ .name }}'
      - name: Description
        value: '{{ .description }}'

vendor:
  list:
    columns:
      - name: Component
        value: '{{ .atmos_component }}'
      - name: Type
        value: '{{ .atmos_vendor_type }}'
      - name: Manifest
        value: '{{ .atmos_vendor_file }}'
      - name: Folder
        value: '{{ .atmos_vendor_target }}'
```

### Phase 3: Command Implementation

#### 3.1 Standard Pattern

**Every list command follows this structure**:

```go
// cmd/list/components.go
var componentsCmd = &cobra.Command{
    Use:   "components",
    Short: "List components",
    Long:  "List all components or filter by stack pattern",
    Example: componentsExample,
    RunE:  executeListComponents,
}

func executeListComponents(cmd *cobra.Command, args []string) error {
    // 1. Parse options
    opts, err := parseComponentsOptions(cmd)
    if err != nil {
        return err
    }

    // 2. Fetch data (command-specific)
    data, err := fetchComponentData(opts)
    if err != nil {
        return err
    }

    // 3. Render using generic renderer
    output, err := renderComponents(data, opts)
    if err != nil {
        return err
    }

    // 4. Write output
    return output.New(opts.Format).Write(output)
}

// Command-specific data fetching
func fetchComponentData(opts *ComponentsOptions) ([]map[string]any, error) {
    atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    if err != nil {
        return nil, err
    }

    stacksMap, err := e.ExecuteDescribeStacks(
        atmosConfig,
        "",  // stack
        nil, // components
        "",  // sections
        opts.Stack,  // pattern
        false,       // ignoreMissingFiles
    )
    if err != nil {
        return nil, err
    }

    // Convert to slice of maps for renderer
    return convertComponentsToMaps(stacksMap), nil
}

// Rendering using reusables
func renderComponents(data []map[string]any, opts *ComponentsOptions) (string, error) {
    // Get column config from atmos.yaml
    atmosConfig, _ := cfg.GetContextFromViper()
    columnConfigs := atmosConfig.Components.List.Columns

    // Override with CLI columns if provided
    if len(opts.Columns) > 0 {
        columnConfigs = parseColumnOverride(opts.Columns)
    }

    return renderer.New(data, renderer.Options{
        Format:    opts.Format,
        Columns:   columnConfigs,
        Filters:   buildComponentFilters(opts),
        Sorters:   buildComponentSorters(opts),
        StyleFunc: componentStyleFunc,
    }).Render()
}

// Build filters from options
func buildComponentFilters(opts *ComponentsOptions) []filter.Filter {
    var filters []filter.Filter

    // Type filter (real, abstract, all)
    if opts.Type != "all" {
        filters = append(filters, filter.NewColumnFilter("atmos_component_type", opts.Type))
    }

    // Enabled filter
    if opts.Enabled != nil {
        filters = append(filters, filter.NewBoolFilter("enabled", opts.Enabled))
    }

    // Locked filter
    if opts.Locked != nil {
        filters = append(filters, filter.NewBoolFilter("locked", opts.Locked))
    }

    // Custom filter expression
    if opts.Filter != "" {
        yqFilter, _ := filter.NewYQFilter(opts.Filter)
        filters = append(filters, yqFilter)
    }

    return filters
}

// Build sorters from options
func buildComponentSorters(opts *ComponentsOptions) []*sort.Sorter {
    if opts.Sort != "" {
        sorters, _ := sort.ParseSortSpec(opts.Sort)
        return sorters
    }

    // Use config default
    atmosConfig, _ := cfg.GetContextFromViper()
    return parseSortConfig(atmosConfig.Components.List.Sort)
}

// Conditional styling
func componentStyleFunc(row []string, idx int, headers []string) lipgloss.Style {
    styles := theme.GetCurrentStyles()

    // Find enabled/locked column indices
    enabledIdx := findColumnIndex(headers, "Enabled")
    lockedIdx := findColumnIndex(headers, "Locked")

    // Disabled = gray
    if enabledIdx >= 0 && row[enabledIdx] == "✗" {
        return styles.Muted
    }

    // Locked = orange/warning
    if lockedIdx >= 0 && row[lockedIdx] == "🔒" {
        return styles.Warning
    }

    return lipgloss.NewStyle()
}
```

#### 3.2 Flag Handler Enhancement

**Update `cmd/list/utils.go`** with named wrapper functions:

```go
// Named wrapper functions for list command flags
// Follow same With* naming convention as pkg/flags/ API
// Each function appends flag options to the slice

// WithFormatFlag adds output format flag with environment variable support
func WithFormatFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("format", "", "", "Output format: table, json, yaml, csv, tsv"),
        flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
    )
}

// WithDelimiterFlag adds CSV/TSV delimiter flag
func WithDelimiterFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("delimiter", "", "", "Delimiter for CSV/TSV output"),
    )
}

// WithColumnsFlag adds column selection flag with environment variable support
func WithColumnsFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringSliceFlag("columns", "", nil, "Columns to display (overrides atmos.yaml)"),
        flags.WithEnvVars("columns", "ATMOS_LIST_COLUMNS"),
    )
}

// WithStackFlag adds stack filter flag
func WithStackFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("stack", "s", "", "Filter by stack pattern (glob)"),
    )
}

// WithFilterFlag adds YQ filter expression flag with environment variable support
func WithFilterFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("filter", "", "", "Filter expression (YQ syntax)"),
        flags.WithEnvVars("filter", "ATMOS_LIST_FILTER"),
    )
}

// WithSortFlag adds sort specification flag with environment variable support
func WithSortFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("sort", "", "", "Sort by column:order (e.g., 'stack:asc,component:desc')"),
        flags.WithEnvVars("sort", "ATMOS_LIST_SORT"),
    )
}

// WithEnabledFlag adds enabled filter flag
func WithEnabledFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithBoolFlag("enabled", "", nil, "Filter by enabled (true/false, omit for all)"),
    )
}

// WithLockedFlag adds locked filter flag
func WithLockedFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithBoolFlag("locked", "", nil, "Filter by locked (true/false, omit for all)"),
    )
}

// WithTypeFlag adds component type filter flag with environment variable support
func WithTypeFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("type", "", "real", "Component type: real, abstract, all"),
        flags.WithEnvVars("type", "ATMOS_COMPONENT_TYPE"),
    )
}

// NewListParser creates a parser with specified flags
// NOT all commands use the same flags - only include what makes sense per command
func NewListParser(builders ...func(*[]flags.Option)) *flags.StandardParser {
    options := []flags.Option{}

    // Build flags from provided builder functions
    for _, builder := range builders {
        builder(&options)
    }

    return flags.NewStandardParser(options...)
}
```

**Command-specific flag composition**:

```go
// cmd/list/components.go - Has format, columns, filters
func init() {
    componentsParser = NewListParser(
        WithFormatFlag,      // Output format selection
        WithColumnsFlag,     // Column customization
        WithSortFlag,        // Sorting
        WithFilterFlag,      // YQ filtering
        WithStackFlag,       // Filter by stack
        WithTypeFlag,        // Filter by component type (real/abstract)
        WithEnabledFlag,     // Filter by enabled status
        WithLockedFlag,      // Filter by locked status
    )
    componentsParser.RegisterFlags(componentsCmd)
    _ = componentsParser.BindToViper(viper.GetViper())
}

// cmd/list/stacks.go - Simpler, just filtering
func init() {
    stacksParser = NewListParser(
        WithFormatFlag,      // Output format
        WithColumnsFlag,     // Column customization
        WithSortFlag,        // Sorting
        // WithFilterFlag - NOT needed, stacks is simple
        // WithStackFlag - NOT needed, this lists stacks
        WithComponentFlag,   // Filter stacks by component
    )
    stacksParser.RegisterFlags(stacksCmd)
    _ = stacksParser.BindToViper(viper.GetViper())
}

// cmd/list/workflows.go - File filtering, format output
func init() {
    workflowsParser = NewListParser(
        WithFormatFlag,      // Output format
        WithDelimiterFlag,   // For CSV/TSV
        WithColumnsFlag,     // Column customization
        WithSortFlag,        // Sorting
        WithFileFlag,        // Filter by workflow file (existing flag)
        // WithStackFlag - NOT relevant to workflows
        // WithFilterFlag - Could add later, not critical
    )
    workflowsParser.RegisterFlags(workflowsCmd)
    _ = workflowsParser.BindToViper(viper.GetViper())
}

// cmd/list/values.go - Complex with YQ filtering
func init() {
    valuesParser = NewListParser(
        WithFormatFlag,          // Output format
        WithDelimiterFlag,       // For CSV/TSV
        WithMaxColumnsFlag,      // Limit columns displayed
        WithQueryFlag,           // YQ expression filtering
        WithStackFlag,           // Filter by stack pattern
        WithAbstractFlag,        // Include abstract components
        WithProcessTemplatesFlag, // Process Go templates
        WithProcessFunctionsFlag, // Process template functions
        // WithColumnsFlag - NOT needed, uses max-columns instead
        // WithSortFlag - Could add, not critical
    )
    valuesParser.RegisterFlags(valuesCmd)
    _ = valuesParser.BindToViper(viper.GetViper())
}
```

**Flag Mapping by Command**:

| Command | Format | Columns | Sort | Filter | Stack | Delimiter | Command-Specific |
|---------|--------|---------|------|--------|-------|-----------|------------------|
| **stacks** | ✓ | ✓ | ✓ | - | - | - | `--component` |
| **components** | ✓ | ✓ | ✓ | ✓ | ✓ | - | `--type`, `--enabled`, `--locked` |
| **workflows** | ✓ | ✓ | ✓ | - | - | ✓ | `--file` |
| **vendor** | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | - |
| **values** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--abstract`, `--process-*` |
| **vars** | ✓ | - | - | - | ✓ | ✓ | Same as values (alias) |
| **metadata** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--process-*` |
| **settings** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--process-*` |
| **instances** | ✓ | ✓ | ✓ | - | ✓ | ✓ | `--upload` |

**Additional Command-Specific Flag Helpers**:

```go
// WithComponentFlag adds component filter flag (for list stacks)
func WithComponentFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("component", "c", "", "Filter stacks by component"),
        flags.WithEnvVars("component", "ATMOS_COMPONENT"),
    )
}

// WithFileFlag adds workflow file filter flag
func WithFileFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("file", "f", "", "Filter by workflow file"),
    )
}

// WithMaxColumnsFlag adds max columns limit flag (for values/metadata/settings)
func WithMaxColumnsFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithIntFlag("max-columns", "", 0, "Maximum number of columns to display"),
    )
}

// WithQueryFlag adds YQ query expression flag
func WithQueryFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("query", "", "", "YQ expression to filter data"),
    )
}

// WithAbstractFlag adds abstract component inclusion flag
func WithAbstractFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithBoolFlag("abstract", "", false, "Include abstract components"),
    )
}

// WithProcessTemplatesFlag adds template processing flag
func WithProcessTemplatesFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithBoolFlag("process-templates", "", true, "Process Go templates"),
    )
}

// WithProcessFunctionsFlag adds function processing flag
func WithProcessFunctionsFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithBoolFlag("process-functions", "", true, "Process template functions"),
    )
}

// WithUploadFlag adds upload to Pro API flag (for instances)
func WithUploadFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithBoolFlag("upload", "", false, "Upload instances to Atmos Pro API"),
    )
}
```

**Benefits of Named Functions**:
- ✅ More readable: `WithLockedFlag` vs `flags.WithBoolFlag("locked", "", nil, "...")`
- ✅ **Consistent with pkg/flags/ naming** (`With*` pattern)
- ✅ Reusable across multiple commands
- ✅ Consistent flag definitions (description, env vars)
- ✅ Easy to discover available flags via autocomplete
- ✅ Single source of truth for each flag's configuration
- ✅ Easier to test flag configurations
- ✅ **Each command chooses only the flags it needs**

#### 3.3 Commands to Update

**Priority Tier 1** (Linear Issues):
1. `cmd/list/stacks.go` - Add schema field, use reusables
2. `cmd/list/components.go` - Add filters, use reusables (DEV-2805)
3. `cmd/list/vendor.go` - Use reusables (DEV-2806)

**Priority Tier 2**:
4. `cmd/list/workflows.go` - Migrate to reusables
5. `cmd/list/values.go` - Use reusables
6. `cmd/list/vars.go` - Use reusables (alias of values)
7. `cmd/list/instances.go` - Verify reusable usage

**Priority Tier 3**:
8. `cmd/list/metadata.go` - Use reusables
9. `cmd/list/settings.go` - Use reusables

**Reference** (no changes):
10. `cmd/list/themes.go` - Already complete ✅

### Phase 4: Business Logic Refactoring

#### 4.1 Separate Data from Formatting

**Before (mixed concerns)**:
```go
// pkg/list/list_workflows.go
func FilterAndListWorkflows(..., format string) (string, error) {
    rows := fetchData()
    return formatOutput(rows, format)  // ❌ Mixed
}
```

**After (separated)**:
```go
// pkg/list/list_workflows.go
func FetchWorkflowData(...) ([]WorkflowRow, error) {
    // Pure data fetching
    return rows, nil
}

// Formatting handled by renderer in cmd layer
```

#### 4.2 Eliminate Anti-Patterns

**Replace**:
- ❌ `u.PrintMessageInColor()` → ✅ `data.Write()` or `ui.Write()`
- ❌ Direct format handling in pkg → ✅ Use `pkg/list/renderer/`
- ❌ Custom CSV/JSON logic → ✅ Use `pkg/list/format/` formatters
- ❌ Duplicated validation → ✅ Centralize in reusables

### Phase 5: Documentation

#### 5.1 Update Existing Documentation

**Files**:
- `website/docs/cli/commands/list/usage.mdx` - Verify examples
- `website/docs/cli/commands/list/list-stacks.mdx`
- `website/docs/cli/commands/list/list-components.mdx` (DEV-2805)
- `website/docs/cli/commands/list/list-vendor.mdx` (DEV-2806)
- All other list command docs

**Content**:
- Template variable reference for each command
- Filter flag examples
- Sort flag examples
- Conditional styling behavior
- Configuration examples

#### 5.2 Template Variable Reference

**Document available template variables per command**:

**Components**:
```
{{ .atmos_component }}       - Component name
{{ .atmos_stack }}           - Stack name
{{ .atmos_component_type }}  - "real" or "abstract"
{{ .vars.* }}                - Component variables
{{ .settings.* }}            - Component settings
{{ .metadata.* }}            - Component metadata
{{ .env.* }}                 - Environment variables
{{ .enabled }}               - Boolean: component enabled
{{ .locked }}                - Boolean: component locked
{{ .abstract }}              - Boolean: component is abstract
```

**Stacks**:
```
{{ .stack }}                     - Stack name
{{ .components.terraform }}      - Map of Terraform components
{{ .components.helmfile }}       - Map of Helmfile components
{{ len .components.terraform }}  - Count of Terraform components
```

**Workflows**:
```
{{ .file }}         - Workflow file path
{{ .name }}         - Workflow name
{{ .description }}  - Workflow description
{{ .steps }}        - Workflow steps array
```

**Vendor**:
```
{{ .atmos_component }}    - Component name
{{ .atmos_vendor_type }}  - "component" or "vendor"
{{ .atmos_vendor_file }}  - Manifest file path
{{ .atmos_vendor_target }}- Target folder
```

#### 5.3 Build Verification

```bash
cd website
npm run build
```

### Phase 6: Testing

#### 6.1 Reusable Utilities (>90% Coverage)

**`pkg/list/column/column_test.go`**:
```go
func TestNewSelector(t *testing.T) { /* ... */ }
func TestSelector_Extract_TemplateEvaluation(t *testing.T) { /* ... */ }
func TestSelector_Extract_WithNestedVars(t *testing.T) { /* ... */ }
func TestSelector_Extract_WithMissingFields(t *testing.T) { /* ... */ }
func TestSelector_Select(t *testing.T) { /* ... */ }
func TestTemplateFunctions(t *testing.T) { /* ... */ }
```

**`pkg/list/filter/filter_test.go`**:
```go
func TestYQFilter(t *testing.T) { /* ... */ }
func TestGlobFilter(t *testing.T) { /* ... */ }
func TestColumnValueFilter(t *testing.T) { /* ... */ }
func TestBoolFilter(t *testing.T) { /* ... */ }
func TestChain(t *testing.T) { /* ... */ }
```

**`pkg/list/sort/sort_test.go`**:
```go
func TestSorter_Sort_Ascending(t *testing.T) { /* ... */ }
func TestSorter_Sort_Descending(t *testing.T) { /* ... */ }
func TestSorter_Sort_Numeric(t *testing.T) { /* ... */ }
func TestMultiSorter(t *testing.T) { /* ... */ }
func TestParseSortSpec(t *testing.T) { /* ... */ }
```

**`pkg/list/renderer/renderer_test.go`**:
```go
func TestRenderer_FullPipeline(t *testing.T) { /* ... */ }
func TestRenderer_AllFormats(t *testing.T) { /* ... */ }
func TestRenderer_ConditionalStyling(t *testing.T) { /* ... */ }
```

**`pkg/list/output/output_test.go`**:
```go
func TestManager_Write_Structured(t *testing.T) { /* ... */ }
func TestManager_Write_HumanReadable(t *testing.T) { /* ... */ }
```

**Coverage Target**: >90% on all reusable packages

#### 6.2 Command Tests (80-90% Coverage)

**`cmd/list/*_test.go`**:
- Test data fetching logic
- Test flag parsing
- Test integration with reusables
- Mock renderer for isolation

**Coverage Target**: 80-90% on command files

#### 6.3 Integration Tests

**Golden snapshots**:
```bash
# Regenerate all list command snapshots
go test ./tests -run 'TestCLICommands/atmos_list_*' -regenerate-snapshots

# Verify specific command
go test ./tests -run 'TestCLICommands/atmos_list_components' -v
```

**Test scenarios**:
- Custom columns from atmos.yaml
- CLI column override
- Filter combinations
- Sort combinations
- All format types
- Conditional styling output

## Implementation Timeline

### Week 1: Reusable Foundation
- **Day 1-2**: `pkg/list/column/` + tests (90%+ coverage)
- **Day 3**: `pkg/list/filter/` + tests (90%+ coverage)
- **Day 4**: `pkg/list/sort/` + tests (90%+ coverage)
- **Day 5**: `pkg/list/renderer/` + `pkg/list/output/` + tests (90%+ coverage)

### Week 2: Schema & Tier 1 Commands
- **Day 1**: Schema updates (Stacks, Components)
- **Day 2-3**: Update `list components` (DEV-2805) + tests (80%+ coverage)
- **Day 4**: Update `list stacks` + tests (80%+ coverage)
- **Day 5**: Update `list vendor` (DEV-2806) + tests (80%+ coverage)

### Week 3: Tier 2 & 3 Commands
- **Day 1-2**: Update workflows, values, vars + tests
- **Day 3**: Update instances, metadata, settings + tests
- **Day 4-5**: Remove deprecated code, refactor business logic

### Week 4: Documentation & Testing
- **Day 1-2**: Update all documentation
- **Day 3**: Integration tests and golden snapshots
- **Day 4**: PRD documentation (this document)
- **Day 5**: Final review and cleanup

## Success Criteria

### Reusable Infrastructure ✅
- [x] Generic column selection with Go template evaluation (>90% coverage)
- [x] Generic filtering (YQ, glob, value, bool) (>90% coverage)
- [x] Generic sorting (any column, any order) (>90% coverage)
- [x] Generic renderer supporting all formats (>90% coverage)
- [x] Output manager enforcing data/ui layer usage (>90% coverage)

### Architecture ✅
- [x] No deep exits (maintain existing clean pattern)
- [x] Data fetching separated from formatting
- [x] Uses `data.Write*()` for structured output (stdout)
- [x] Uses `ui.Write*()` for human output (stderr)
- [x] TTY detection automatic (no manual checks)
- [x] No `u.PrintMessageInColor()` usage

### Features ✅
- [x] DEV-2805: `atmos list components` with filters
- [x] DEV-2806: `atmos list vendor` with columns
- [x] All commands support column customization
- [x] All commands support --columns CLI override
- [x] All commands support --sort flag
- [x] All commands support --filter flag

### Documentation ✅
- [x] All documented features implemented
- [x] Configuration examples accurate
- [x] Template variable reference complete
- [x] Website builds without errors

### Testing ✅
- [x] >90% coverage on reusable utilities
- [x] 80-90% coverage on commands
- [x] Integration tests with golden snapshots
- [x] All tests pass

## Risks & Mitigations

### Risk: Template Evaluation Performance

**Risk**: Evaluating Go templates for every row could be slow for large datasets.

**Mitigation**:
- Pre-parse and cache templates (done once per selector)
- Benchmark with realistic datasets
- Consider lazy evaluation or pagination for very large lists
- Profile and optimize hot paths

### Risk: Breaking Changes in Column Config

**Risk**: Existing `workflows` and `vendor` column configs might break.

**Mitigation**:
- Maintain backward compatibility with existing configs
- Test with existing configurations in fixtures
- Document migration path if changes needed
- Version column config format in schema

### Risk: Complex Template Debugging

**Risk**: Users may struggle with template syntax errors.

**Mitigation**:
- Provide clear error messages with line/column info
- Include template context in error output
- Add `--debug-templates` flag for troubleshooting
- Document common template patterns and gotchas

## Future Enhancements

**Out of scope for this PRD but worth considering**:

1. **Interactive TUI**: Browse/filter list results interactively
2. **Pagination**: For very large datasets
3. **Column Auto-Sizing**: Smart column width calculation
4. **Export Templates**: Save custom column configs as templates
5. **List Profiles**: Predefined column sets (minimal, default, full)
6. **Watch Mode**: Auto-refresh list output (`--watch`)
7. **Diff Mode**: Compare two list outputs

## Appendix A: Template Function Reference

### Type Conversion
- `toString` - Convert any value to string
- `toInt` - Convert to integer
- `toBool` - Convert to boolean

### Formatting
- `truncate n` - Truncate string to n characters
- `pad n` - Pad string to n characters
- `upper` - Convert to uppercase
- `lower` - Convert to lowercase

### Data Access
- `get map key` - Safe nested map access
- `getOr map key default` - Get with default value
- `has map key` - Check if key exists

### Collections
- `len` - Get length of array/map/string
- `join array sep` - Join array with separator
- `split string sep` - Split string by separator

### Conditional
- `ternary condition true false` - Ternary operator

### Examples

```yaml
# Conditional icon
value: '{{ ternary .enabled "✓" "✗" }}'

# Nested field access
value: '{{ get .vars "region" }}'

# With default
value: '{{ getOr .vars "region" "unknown" }}'

# Array length
value: '{{ len .steps }}'

# Formatting
value: '{{ .description | truncate 50 }}'

# Multiple operations
value: '{{ if .enabled }}{{ .vars.region | upper }}{{ else }}disabled{{ end }}'
```

## Appendix B: CLI Examples

```bash
# List components with default columns from atmos.yaml
atmos list components

# Override columns via CLI
atmos list components --columns component,stack,region

# Filter by type
atmos list components --type abstract

# Filter by enabled/locked status
atmos list components --enabled=true --locked=false

# Sort by multiple columns
atmos list components --sort "stack:asc,component:desc"

# YQ filter expression
atmos list components --filter '.vars.region == "us-east-2"'

# Combine filters and custom columns
atmos list components \
  --type real \
  --enabled=true \
  --columns component,stack,region \
  --sort "region:asc,stack:asc" \
  --format table

# Export to CSV
atmos list components --format csv > components.csv

# Export to JSON for jq processing
atmos list components --format json | jq '.[] | select(.region == "us-east-2")'

# List stacks with custom columns
atmos list stacks --columns stack,terraform_count,helmfile_count

# List workflows sorted by name
atmos list workflows --sort "name:asc"

# List vendor components
atmos list vendor --columns component,type,manifest,folder
```

## Appendix C: Configuration Precedence

**Column Selection**:
1. CLI `--columns` flag (highest priority)
2. `atmos.yaml` `{command}.list.columns`
3. Default columns per command (hardcoded fallback)

**Sort Order**:
1. CLI `--sort` flag (highest priority)
2. `atmos.yaml` `{command}.list.sort`
3. No sorting (default)

**Output Format**:
1. CLI `--format` flag (highest priority)
2. Command-specific `{command}.list.format` (for commands with config sections)
3. Environment variable `ATMOS_LIST_FORMAT`
4. `"table"` (default)

**Note**: Only commands with dedicated config sections support `list.format` configuration:
- `stacks.list.format`
- `components.list.format`
- `workflows.list.format`
- `vendor.list.format`

Commands without config sections (instances, values, vars, metadata, settings) use env var and CLI flag only.

## Appendix D: Error Messages

**Template Evaluation Errors**:
```
Error evaluating column template for "Region":
  Template: {{ .vars.region }}
  Error: map has no entry for key "region"
  Context: component=vpc, stack=plat-ue2-dev

Hint: Check that the field exists in your component configuration.
Use --debug-templates to see full template context.
```

**Filter Errors**:
```
Error applying YQ filter:
  Filter: .vars.region == "us-east-2"
  Error: invalid syntax at line 1, column 15

Hint: Use YQ syntax for filters. See: https://mikefarah.gitbook.io/yq/
```

**Sort Errors**:
```
Error sorting by column "Region":
  Column not found in output
  Available columns: Component, Stack, Enabled

Hint: Sorting applies to output columns, not raw data fields.
Ensure the column exists in your column configuration.
```
