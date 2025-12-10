List all configured profiles across all locations with their details.

Profiles are discovered from multiple locations in precedence order:
1. Configurable (profiles.base_path in atmos.yaml)
2. Project-hidden (.atmos/profiles/)
3. XDG user (~/.config/atmos/profiles/ or $XDG_CONFIG_HOME/atmos/profiles/)
4. Project (profiles/)

Supports multiple output formats:
- **table** (default): tabular view with profile details
- **json**/**yaml**: structured data for programmatic access
