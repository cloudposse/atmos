# Shared entry point for custom commands and the installed Git hook.
load("suite.star", "run_suite")

# Default comparison base for branch-wide checks such as the 150-file limit.
DEFAULT_BASE = "origin/main"

# Resolve the branch comparison base. A layer in a PR stack compares against its parent
# branch, so an explicit base wins, then ATMOS_BRANCH_BASE_REF, then the per-branch Git
# config key branch.<name>.stackbase, and finally origin/main.
def resolve_base(base):
    if base != DEFAULT_BASE:
        return base
    override = os.getenv("ATMOS_BRANCH_BASE_REF")
    if override:
        return override
    branch = exec.run(["git", "rev-parse", "--abbrev-ref", "HEAD"], output = "capture", check = False)
    name = branch.stdout.strip() if branch.exit_code == 0 else ""
    if name and name != "HEAD":
        configured = exec.run(["git", "config", "--get", "branch." + name + ".stackbase"], output = "capture", check = False)
        if configured.exit_code == 0 and configured.stdout.strip():
            return configured.stdout.strip()
    return base

# Run the shared suite in a Git sandbox with read-only checks or unstaged fixes.
def run(scope, mode, base = DEFAULT_BASE, only = "", skip = ""):
    if mode not in ["check", "format", "commit"]:
        fail("Unknown check mode: " + mode)
    base = resolve_base(base)
    git.sandbox(
        scope = scope,
        base_ref = base,
        fixes = "none" if mode == "check" else "unstaged",
        fail_on_changes = mode != "format",
        path_prepend = ["build/check-tools"],
        run = lambda sandbox: run_suite(sandbox, mode, only, base, skip),
    )
