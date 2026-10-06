load("../cast_checks.star", "load_text", "strip_ansi", "assert_no_experimental", "assert_colored", "assert_no_error_output", "assert_no_local_paths")

def validate(name, filename, needles, expected_error=False):
    text = load_text("../../website/static/casts/examples/{}/{}.cast".format(name, filename))
    plain = strip_ansi(text)
    assert_no_experimental(text)
    assert_no_local_paths(text)
    if not expected_error:
        assert_no_error_output(text)
    for needle in needles:
        if needle not in plain:
            fail("{} cast missing {!r}".format(name, needle))
    for bad in ["unknown flag", "panic:", "failed to resolve", "SECRET_SENTINEL", "Peak memory", "Update available!"]:
        if bad in plain:
            fail("{} cast contains {!r}".format(name, bad))
    return text, plain

validate("starlark-script", "summarize", [
    "#!/usr/bin/env atmos",
    "./summarize.star services.json",
    "api: 2 replicas",
    "worker: 3 replicas",
    "Total: 2 services, 5 replicas",
])
validate("starlark-commands", "capacity", [
    "interpreter: starlark",
    "atmos capacity --replicas 3",
    "3 replicas x 4 workers = 12 workers",
])
hook_text, hook_plain = validate("starlark-hooks", "owner-check", [
    "atmos terraform plan api -s dev",
    "Owner check passed: api belongs to platform (dev).",
    "No changes.",
    "atmos terraform plan api -s unowned",
    "Set an owner before planning api",
], expected_error=True)
assert_colored(hook_text, "Owner check passed:")
if hook_plain.count("No changes.") != 1 or len([line for line in hook_plain.splitlines() if line.strip() in ["Error", "# Error"]]) != 1:
    fail("hook cast must show one successful plan and one rejected plan")
if hook_plain.index("Owner check passed:") > hook_plain.index("No changes."):
    fail("owner check must run before Terraform")
if "No changes." in hook_plain.split("atmos terraform plan api -s unowned")[-1]:
    fail("Terraform must not plan after the owner hook rejects the command")
ui.success("All three Starlark example casts validated")
