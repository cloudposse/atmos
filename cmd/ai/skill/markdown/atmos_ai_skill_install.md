Install AI skills by bundled name (offline) or from a GitHub repository.

The official Atmos skills are embedded in the binary, so installing one by its
bare name (e.g. `atmos-terraform`) works fully offline — no network or Git
clone. Run `atmos ai skill list` to see every available skill. You can also
install from any GitHub repository: single-skill repos (with SKILL.md at root)
and multi-skill packages (with skills/*/SKILL.md pattern) are both supported.

Skills will be validated and installed to ~/.atmos/skills/. You can then use
them in the AI TUI by switching with Ctrl+A.

Skills follow the Agent Skills open standard (https://agentskills.io)
and use the SKILL.md format with YAML frontmatter.

Source formats:
  atmos-terraform                   Bundled skill by name (offline)
  user/repo                         GitHub shorthand (GitHub assumed)
  user/repo@v1.2.3                  Specific version tag
  github.com/user/repo              Full GitHub path
  github.com/user/repo@v1.2.3       Full path with version
  https://github.com/user/repo.git  Full HTTPS URL

Security:
  - Skills cannot execute arbitrary code
  - Tool access is explicitly declared in skill metadata
  - You will be prompted to confirm installation before proceeding
  - Use --yes to skip confirmation (for automation)
