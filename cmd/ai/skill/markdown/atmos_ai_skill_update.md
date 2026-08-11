Update installed bundled skills to their latest catalog version.

Bundled skills (installed by bare name, e.g. `atmos-terraform`) are static
copies made at install time -- upgrading the `atmos` binary alone does not
refresh a skill you already installed, even if that release bundles newer
skill content. `update` closes that gap: it compares each installed bundled
skill's recorded version against the catalog embedded in the running binary,
and reinstalls only the ones that are actually outdated.

With no `<name>` given, every installed bundled skill with an update available
is refreshed at once (a single confirmation, not one per skill). Skills that
are already current are left untouched and reported separately, so a bare
`atmos ai skill update` is safe to run repeatedly.

Skills installed from a GitHub repository are not supported yet -- there's no
cheap way to check whether a git-sourced skill's upstream has moved without
actually re-fetching it. Run `atmos ai skill install <source> --force` to
refresh one of those manually.

An outdated skill is reinstalled the same way `atmos ai skill install <name>
--force` would install it: the same client-distribution and scope flags
apply, since update re-runs that same logic once it's confirmed a reinstall
is actually needed.

Flags:
  - --path overrides the install directory (default: ~/.atmos/skills); relative
    paths resolve against the current working directory
  - --client (repeatable) distributes the updated skill to specific AI clients:
    claude-code, vscode, gemini
  - --all-clients distributes the updated skill to every supported AI client
  - When no --client/--all-clients is given, detected clients are used
    automatically (interactively you'll be prompted to confirm)
  - --scope selects the distribution scope: project (default) or user
  - --global is an alias for --scope user
  - --yes skips the confirmation prompt (for automation)
