package list

// Example usage patterns for different list commands.
// This file demonstrates how each command composes only the flags it needs.

/*
// ============================================================================
// EXAMPLE 1: List Components (Full-Featured)
// ============================================================================
// Needs: Format, Columns, Sort, Filter, Stack pattern, Component type, Enabled, Locked

func init() {
    componentsParser = NewListParser(
        WithFormatFlag,      // --format (table/json/yaml/csv/tsv)
        WithColumnsFlag,     // --columns (override atmos.yaml)
        WithSortFlag,        // --sort "stack:asc,component:desc"
        WithFilterFlag,      // --filter (YQ expression)
        WithStackFlag,       // --stack "plat-*-prod"
        WithTypeFlag,        // --type real/abstract/all
        WithEnabledFlag,     // --enabled=true
        WithLockedFlag,      // --locked=false
    )

    componentsParser.RegisterFlags(componentsCmd)
    _ = componentsParser.BindToViper(viper.GetViper())
}

// ============================================================================
// EXAMPLE 2: List Stacks (Simple)
// ============================================================================
// Needs: Format, Columns, Sort, Component filter

func init() {
    stacksParser = NewListParser(
        WithFormatFlag,      // --format
        WithColumnsFlag,     // --columns
        WithSortFlag,        // --sort
        WithComponentFlag,   // --component (filter stacks by component)
    )

    stacksParser.RegisterFlags(stacksCmd)
    _ = stacksParser.BindToViper(viper.GetViper())
}

// ============================================================================
// EXAMPLE 3: List Workflows
// ============================================================================
// Needs: Format, Delimiter, Columns, Sort, File filter

func init() {
    workflowsParser = NewListParser(
        WithFormatFlag,      // --format
        WithDelimiterFlag,   // --delimiter (for CSV/TSV)
        WithColumnsFlag,     // --columns
        WithSortFlag,        // --sort
        WithFileFlag,        // --file (filter by workflow file)
    )

    workflowsParser.RegisterFlags(workflowsCmd)
    _ = workflowsParser.BindToViper(viper.GetViper())
}

// ============================================================================
// EXAMPLE 4: List Vendor
// ============================================================================
// Needs: Format, Delimiter, Columns, Sort, Filter, Stack pattern

func init() {
    vendorParser = NewListParser(
        WithFormatFlag,      // --format
        WithDelimiterFlag,   // --delimiter
        WithColumnsFlag,     // --columns
        WithSortFlag,        // --sort
        WithFilterFlag,      // --filter
        WithStackFlag,       // --stack
    )

    vendorParser.RegisterFlags(vendorCmd)
    _ = vendorParser.BindToViper(viper.GetViper())
}

// ============================================================================
// EXAMPLE 5: List Values/Vars (Complex YQ Filtering)
// ============================================================================
// Needs: Format, Delimiter, Max Columns, Query, Stack, Abstract, Process Templates/Functions

func init() {
    valuesParser = NewListParser(
        WithFormatFlag,              // --format
        WithDelimiterFlag,           // --delimiter
        WithMaxColumnsFlag,          // --max-columns
        WithQueryFlag,               // --query (YQ expression)
        WithStackFlag,               // --stack
        WithAbstractFlag,            // --abstract
        WithProcessTemplatesFlag,    // --process-templates
        WithProcessFunctionsFlag,    // --process-functions
    )

    valuesParser.RegisterFlags(valuesCmd)
    _ = valuesParser.BindToViper(viper.GetViper())
}

// ============================================================================
// EXAMPLE 6: List Metadata/Settings
// ============================================================================
// Needs: Same as values but without abstract flag

func init() {
    metadataParser = NewListParser(
        WithFormatFlag,              // --format
        WithDelimiterFlag,           // --delimiter
        WithMaxColumnsFlag,          // --max-columns
        WithQueryFlag,               // --query
        WithStackFlag,               // --stack
        WithProcessTemplatesFlag,    // --process-templates
        WithProcessFunctionsFlag,    // --process-functions
    )

    metadataParser.RegisterFlags(metadataCmd)
    _ = metadataParser.BindToViper(viper.GetViper())
}

// ============================================================================
// EXAMPLE 7: List Instances
// ============================================================================
// Needs: Format, Delimiter, Columns, Sort, Stack, Upload

func init() {
    instancesParser = NewListParser(
        WithFormatFlag,      // --format
        WithDelimiterFlag,   // --delimiter
        WithColumnsFlag,     // --columns
        WithSortFlag,        // --sort
        WithStackFlag,       // --stack
        WithUploadFlag,      // --upload (to Pro API)
    )

    instancesParser.RegisterFlags(instancesCmd)
    _ = instancesParser.BindToViper(viper.GetViper())
}

// ============================================================================
// FLAG MAPPING REFERENCE
// ============================================================================
//
// Command      | Format | Columns | Sort | Filter | Stack | Delimiter | Command-Specific
// -------------|--------|---------|------|--------|-------|-----------|------------------
// stacks       | ✓      | ✓       | ✓    | -      | -     | -         | --component
// components   | ✓      | ✓       | ✓    | ✓      | ✓     | -         | --type, --enabled, --locked
// workflows    | ✓      | ✓       | ✓    | -      | -     | ✓         | --file
// vendor       | ✓      | ✓       | ✓    | ✓      | ✓     | ✓         | -
// values       | ✓      | -       | -    | -      | ✓     | ✓         | --max-columns, --query, --abstract, --process-*
// vars         | ✓      | -       | -    | -      | ✓     | ✓         | Same as values (alias)
// metadata     | ✓      | -       | -    | -      | ✓     | ✓         | --max-columns, --query, --process-*
// settings     | ✓      | -       | -    | -      | ✓     | ✓         | --max-columns, --query, --process-*
// instances    | ✓      | ✓       | ✓    | -      | ✓     | ✓         | --upload
//
// ============================================================================
*/
