Uninstall a community-contributed skill from this system.

This will remove the skill from ~/.atmos/skills/ and delete its registry entry.
You will be prompted to confirm the uninstallation unless --force is specified.

The skill name is the short identifier (not the display name).
Use 'atmos ai skill list' to see installed skill names.

With no `<name>` given, every installed skill is removed at once (a single
confirmation, not one per skill).

Any copies auto-distributed to AI clients during install (e.g. .github/skills/
for VS Code/Copilot) are removed as well. Use --client/--all-clients to control
which clients are cleaned up; by default, detected clients are used
automatically (interactively you'll be prompted to confirm).

Use --scope user (or --global) if the skill was installed with --scope user,
so cleanup targets each client's personal, user-level skill directory instead
of the project one. When neither --scope nor --global is given and the
command is running in an interactive terminal, Atmos prompts you to choose
project or user scope; --force, a non-TTY session, or CI skips the prompt and
defaults to project.

If a client's copy is actually a symbolic link (e.g. this repo's own
.claude/skills/<name> entries, which intentionally point into
agent-skills/skills/<name> for contributor auto-discovery), it is left in
place with a warning rather than being deleted.
