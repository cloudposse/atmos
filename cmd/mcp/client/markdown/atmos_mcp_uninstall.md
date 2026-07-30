Remove MCP servers previously installed into AI client config files.

The command reads `mcp.servers` (or the server names given on the command line) and
removes matching entries from each targeted client's config. It's the mirror image
of `atmos mcp install` — it only touches client config files, never `atmos.yaml`.

When neither `--scope` nor `--global` is given and the command is running in an
interactive terminal, Atmos prompts you to choose project or user scope; `--yes`,
a non-TTY session, or CI skips the prompt and defaults to `project`.

**Examples:**
```bash
# Remove all configured servers from detected project clients.
atmos mcp uninstall

# Remove one server from Cursor and Claude Code project configs.
atmos mcp uninstall aws-docs --client cursor --client claude-code

# Remove from user-level config.
atmos mcp uninstall --scope user --client codex

# Preview what would be removed without writing files.
atmos mcp uninstall --dry-run
```
