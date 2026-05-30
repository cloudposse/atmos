Validate that an external MCP server can stop and restart cleanly.

**This command does NOT leave the server running.** Stdio-based MCP servers
are subprocesses spawned on demand by AI commands (`atmos ai ask`, `chat`,
`exec`) and live only for the duration of those commands — there is no
long-running daemon to restart in place. `atmos mcp restart` exercises the
full stop+start cycle (so you can verify the server still launches and
exposes its tools after a configuration change) and then exits, leaving
nothing running.

Use this command to:

- Confirm a server still starts successfully after editing `atmos.yaml`,
  changing identities, or updating the underlying package version.
- Pick up `.tool-versions` / toolchain changes without rerunning a full
  AI command.
- Smoke-test connectivity and tool discovery for a specific server in
  isolation.

If you want to invoke the server's tools, use `atmos ai ask`, `chat`, or
`exec` — those commands spawn and tear down servers automatically.

**Examples:**
```bash
# Validate that aws-eks can stop+start cleanly and report its tool count.
atmos mcp restart aws-eks

# Example output:
# ✓ Restarted MCP server `aws-eks` (12 tools available)
# Note: the server is no longer running after the command exits.
```
