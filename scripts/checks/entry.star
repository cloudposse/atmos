# Shared entry point for custom commands and the installed Git hook.
load("suite.star", "run_suite")

# Run the shared suite in a Git sandbox with read-only checks or unstaged fixes.
def run(scope, mode, base = "origin/main", only = "", skip = ""):
    if mode not in ["check", "format", "commit"]:
        fail("Unknown check mode: " + mode)
    git.sandbox(
        scope = scope,
        base_ref = base,
        fixes = "none" if mode == "check" else "unstaged",
        fail_on_changes = mode != "format",
        path_prepend = ["build/check-tools"],
        run = lambda sandbox: run_suite(sandbox, mode, only, base, skip),
    )
