Manage community and custom AI skills.

Skills are specialized AI assistants that provide expert knowledge for specific domains.
Skills follow the Agent Skills open standard (https://agentskills.io).

You can install community-contributed skills from GitHub repositories and manage them
using this command.

Available Commands:
  install     Install a skill from a GitHub repository
  list        List installed skills
  uninstall   Remove an installed skill
  info        Show detailed information about a skill

Examples:
  # Install a skill from GitHub
  atmos ai skill install github.com/user/skill-name
  atmos ai skill install github.com/user/skill-name@v1.2.3

  # List all installed skills
  atmos ai skill list

  # Uninstall a skill
  atmos ai skill uninstall skill-name
