load("../cast_checks.star", "load_text", "strip_ansi", "assert_no_experimental", "assert_no_error_output", "assert_no_local_paths")

# Require real hook results, terminal progress, and no leaked machine-specific output.
def validate():
    text = load_text("../../website/static/casts/demo/repository/git-hooks.cast")
    plain = strip_ansi(text)
    assert_no_experimental(text)
    assert_no_error_output(text)
    assert_no_local_paths(text)
    for needle in [
        "atmos git hooks run pre-commit",
        "Repository checks · staged",
        "branch-file-limit",
        "check-symlinks",
        "trailing-whitespace",
        "end-of-file-fixer",
        "check-yaml",
        "gitleaks",
        "14/14",
        "0 failed",
    ]:
        if needle not in plain:
            fail("Repository hook cast is missing {!r}".format(needle))
    for name in ["trailing-whitespace", "end-of-file-fixer", "check-yaml"]:
        if name + " [skipped]" in plain:
            fail("Repository hook cast must exercise " + name)
    for bad in ["Traceback", "panic:", "unknown flag", "No files selected", "Formatting changed"]:
        if bad in plain:
            fail("Unexpected hook output: " + bad)
    if not regex.search(r"\x1b\[[0-9;]*m", text) or not regex.search("[⠁-⣿]", plain):
        fail("Repository hook cast must retain colored live progress.")
    ui.success("Repository Git hook cast validated")

validate()
