Install AI skills by bundled name (offline) or from a GitHub repository.

The official Atmos skills are embedded in the binary, so installing one by its
bare name (e.g. `atmos-terraform`) works fully offline — no network or Git
clone. Run `atmos ai skill list` to see every available skill. You can also
install from any GitHub repository: single-skill repos (with SKILL.md at root)
and multi-skill packages (with skills/*/SKILL.md pattern) are both supported.

With no `<source>` given, every bundled skill is installed at once (a single
confirmation, not one per skill).

Skills will be validated and installed to ~/.atmos/skills/. You can then use
them in the AI TUI by switching with Ctrl+A.

By default, the skill is also auto-distributed into any detected AI client's
project-local skill directory (e.g. .github/skills/ for VS Code/Copilot,
.claude/skills/ for Claude Code, .gemini/skills/ for Gemini) so it's usable
with zero extra flags. Use --client/--all-clients to control which clients
receive a copy, or --path to take full manual control of the install
location instead (this skips auto-distribution).

Use --scope user (or --global) to distribute into each client's personal,
user-level skill directory instead (e.g. ~/.claude/skills/, ~/.copilot/skills/
for VS Code/Copilot, ~/.gemini/skills/), so the skill is available across
every project rather than just this one. When neither --scope nor --global is
given and the command is running in an interactive terminal, Atmos prompts
you to choose project or user scope; --yes, a non-TTY session, or CI skips the
prompt and defaults to project.

If a client's target directory already exists as a symbolic link (e.g. this
repo's own .claude/skills/<name> entries, which intentionally point into
agent-skills/skills/<name> for contributor auto-discovery), that client is
skipped with a warning rather than writing through the symlink.

Skills follow the Agent Skills open standard (https://agentskills.io)
and use the SKILL.md format with YAML frontmatter.

The `<source>` argument can be a bundled Atmos skill name or a GitHub repository
source. Omit it to install every bundled skill.

Source formats:
  - `atmos-terraform` - Bundled skill by name (offline)
  - `user/repo` - GitHub shorthand (GitHub assumed)
  - `user/repo@v1.2.3` - Specific version tag
  - `github.com/user/repo` - Full GitHub path
  - `github.com/user/repo@v1.2.3` - Full path with version
  - `https://github.com/user/repo.git` - Full HTTPS URL

Security:
  - Skills cannot execute arbitrary code
  - Tool access is explicitly declared in skill metadata
  - You will be prompted to confirm installation before proceeding
  - Use --yes to skip confirmation (for automation)

Flags:
  - --path overrides the install directory (default: ~/.atmos/skills); relative
    paths resolve against the current working directory
  - --client (repeatable) distributes the skill to specific AI clients:
    claude-code, vscode, gemini
  - --all-clients distributes the skill to every supported AI client
  - When no --client/--all-clients is given, detected clients are used
    automatically (interactively you'll be prompted to confirm)
  - --scope selects the distribution scope: project (default) or user
  - --global is an alias for --scope user
  - When neither --scope nor --global is given interactively, you'll be
    prompted to choose; --yes, a non-TTY session, or CI defaults to project
