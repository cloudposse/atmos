Install AI skills from a GitHub repository.

Supports both single-skill repos (with SKILL.md at root) and multi-skill
packages (with skills/*/SKILL.md pattern). The official Atmos skills package
contains 23+ specialized skills for infrastructure orchestration.

Skills will be downloaded, validated, and installed to ~/.atmos/skills/.
You can then use them in the AI TUI by switching with Ctrl+A.

Skills follow the Agent Skills open standard (https://agentskills.io)
and use the SKILL.md format with YAML frontmatter.

Source formats:
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
