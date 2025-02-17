– List metadata across stacks

Usage:
```bash
atmos list metadata [flags]
```

Flags:
```bash
  --delimiter string         Delimiter for CSV/TSV output (default "," for CSV, "\t" for TSV)
  --format string            Output format: table, json, yaml, csv, tsv (default "table")
  --max-columns int         Maximum number of columns to display (default 50)
  --query string            Filter output using JMESPath query (default ".metadata")
  --stack string            Filter by stack pattern (e.g., '*-dev-*', 'prod-*')
```

Examples:
```bash
# List all metadata
atmos list metadata

# List metadata for specific stacks
atmos list metadata --stack '*-dev-*'

# List specific metadata fields
atmos list metadata --query '.metadata.component'
atmos list metadata --query '.metadata.type'

# List metadata in different formats
atmos list metadata --format json
atmos list metadata --format yaml
atmos list metadata --format csv
atmos list metadata --format tsv

# Filter by stack and specific metadata
atmos list metadata --stack '*-ue2-*' --query '.metadata.version'
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
- Metadata is typically found under component configurations
