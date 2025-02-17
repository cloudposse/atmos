– List settings across stacks

Usage:
```bash
atmos list settings [flags]
```

Flags:
```bash
  --delimiter string         Delimiter for CSV/TSV output (default "," for CSV, "\t" for TSV)
  --format string            Output format: table, json, yaml, csv, tsv (default "table")
  --max-columns int         Maximum number of columns to display (default 50)
  --query string            Filter output using JMESPath query (default ".settings")
  --stack string            Filter by stack pattern (e.g., '*-dev-*', 'prod-*')
```

Examples:
```bash
# List all settings
atmos list settings

# List settings for specific stacks
atmos list settings --stack '*-dev-*'

# List specific settings using query
atmos list settings --query '.settings.templates'
atmos list settings --query '.settings.validation'

# List settings in different formats
atmos list settings --format json
atmos list settings --format yaml
atmos list settings --format csv
atmos list settings --format tsv

# Filter by stack and specific settings
atmos list settings --stack '*-ue2-*' --query '.settings.templates.gomplate'
```

Output Formats:
- `table`: Human-readable table format (default for TTY)
- `json`: JSON format with 2-space indentation
- `yaml`: YAML format
- `csv`: Comma-separated values
- `tsv`: Tab-separated values

Note:
- For wide tables, try using more specific queries or reduce the number of stacks
- Stack patterns support glob matching (e.g., '*-dev-*', 'prod-*')
- Settings are typically found under component configurations
