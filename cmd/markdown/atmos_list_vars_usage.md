– List component variables across stacks

Usage:
```bash
atmos list vars <component> [flags]
```

Flags:
```bash
  --abstract                 Include abstract components
  --delimiter string         Delimiter for CSV/TSV output (default "," for CSV, "\t" for TSV)
  --format string            Output format: table, json, yaml, csv, tsv (default "table")
  --max-columns int         Maximum number of columns to display (default 50)
  --query string            Filter output using JMESPath query
  --stack string            Filter by stack pattern (e.g., '*-dev-*', 'prod-*')
```

Examples:
```bash
# List all variables for a component
atmos list vars vpc

# List specific variables for a component
atmos list vars vpc --query '.vars.tags'

# Filter by stack pattern
atmos list vars vpc --stack '*-dev-*'

# List variables in different formats
atmos list vars vpc --format json
atmos list vars vpc --format yaml
atmos list vars vpc --format csv
atmos list vars vpc --format tsv

# Include abstract components
atmos list vars vpc --abstract

# Filter by stack and specific variables
atmos list vars vpc --stack '*-ue2-*' --query '.vars.region'
```

Output Formats:
- `table`: Human-readable table format (default for TTY)
- `json`: JSON format with 2-space indentation
- `yaml`: YAML format
- `csv`: Comma-separated values
- `tsv`: Tab-separated values

Note:
- This is an alias for `atmos list values <component> --query .vars`
- For wide tables, try using more specific queries or reduce the number of stacks
- Stack patterns support glob matching (e.g., '*-dev-*', 'prod-*')

