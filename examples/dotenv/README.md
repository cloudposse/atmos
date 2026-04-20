# Dotenv Example

This example demonstrates Atmos's `.env` file support, which automatically loads environment variables from `.env` files and makes them available to all commands.

## Features Demonstrated

- Loading multiple `.env` files (`.env`, `.env.local`, `.env.dev`)
- Using glob patterns to match `.env.*` files
- Combining static `env.vars` with file-based environment variables
- Custom commands that use loaded environment variables

## Files

- `atmos.yaml` - Atmos configuration with env file settings and custom commands
- `.env` - Main environment file with common variables
- `.env.local` - Local overrides (typically gitignored)
- `.env.dev` - Development-specific variables

## Usage

Run the demo command to see all loaded environment variables:

```bash
atmos show-env
```

Check a specific variable:

```bash
atmos check-var DATABASE_URL
atmos check-var API_KEY
atmos check-var LOCAL_SECRET
```

## Configuration

The `.env` file loading is configured in `atmos.yaml`:

```yaml
env:
  # Static variables defined directly
  vars:
    STATIC_VAR: "I am defined in atmos.yaml"

  # .env file loading
  files:
    enabled: true
    paths:
      - .env           # Load .env file
      - .env.local     # Load .env.local
      - ".env.*"       # Load any .env.* files
    parents: false     # Don't walk up parent directories
```

## Load Order

Files are loaded in the order specified in `paths`. Later files override earlier ones:

1. `.env` - Base variables
2. `.env.local` - Local overrides
3. `.env.*` - Pattern-matched files (alphabetically)

Static `env.vars` take precedence over file-loaded variables.
