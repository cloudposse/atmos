"""Check actual CLI output in the native Terragrunt recording."""

load("../cast_checks.star", "load_text", "strip_ansi", "assert_no_error_output", "assert_no_local_paths", "assert_colored")


def validate():
    text = load_text("../../website/static/casts/demo/fixtures/terragrunt-native/plan.cast")
    plain = strip_ansi(text)
    assert_no_error_output(text)
    assert_no_local_paths(text)
    assert_colored(text, "Plan:")
    # Commands alone are not proof: require the inventory and real engine plan.
    for needle in [
        "components.terraform.terragrunt.discovery.enabled true",
        "atmos list stacks",
        "atmos list instances",
        "Component", "Stack", "Type", "Engine", "opentofu",
        "atmos terraform plan demo -s live",
        "terraform_data.demo",
        "HELLO FROM TERRAGRUNT",
        "Plan: 1 to add, 0 to change, 0 to destroy.",
    ]:
        if needle not in plain:
            fail("native Terragrunt cast missing {!r}".format(needle))
    for marker in ["Error:", "unknown flag", "rpc error", "exit status 1"]:
        if marker in plain:
            fail("native Terragrunt cast contains {!r}".format(marker))
    ui.success("Native Terragrunt discovery and plan cast validated")


validate()
