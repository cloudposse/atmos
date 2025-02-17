– List component values across stacks

Usage:
```bash
atmos list values <component> [flags]
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
# List all values for a component
atmos list values vpc

# List only variables for a component
atmos list values vpc --query .vars

# List settings for a specific component in a stack
atmos list values vpc --query .settings --stack 'plat-ue2-*'

# List values in different formats
atmos list values vpc --format json
atmos list values vpc --format yaml
atmos list values vpc --format csv
atmos list values vpc --format tsv

# Filter stacks and include abstract components
atmos list values vpc --stack '*-prod-*' --abstract

# Custom query with specific stack pattern
atmos list values vpc --query '.vars.tags' --stack '*-ue2-*'
```

Output Formats:
- `table`: Human-readable table format (default for TTY)
- `json`: JSON format with 2-space indentation
- `yaml`: YAML format
- `csv`: Comma-separated values
- `tsv`: Tab-separated values

Note:
- For wide tables, try using more specific queries or reduce the number of stacks
- Use `--query` to filter specific sections (.vars, .settings, .metadata)
- Stack patterns support glob matching (e.g., '*-dev-*', 'prod-*')

