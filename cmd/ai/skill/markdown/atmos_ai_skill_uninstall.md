Uninstall a community-contributed skill from this system.

This will remove the skill from ~/.atmos/skills/ and delete its registry entry.
You will be prompted to confirm the uninstallation unless --force is specified.

The skill name is the short identifier (not the display name).
Use 'atmos ai skill list' to see installed skill names.

Any copies auto-distributed to AI clients during install (e.g. .github/skills/
for VS Code/Copilot) are removed as well. Use --client/--all-clients to control
which clients are cleaned up; by default, detected clients are used
automatically (interactively you'll be prompted to confirm).
