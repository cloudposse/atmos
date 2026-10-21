"""Check the discovery story and the real installation result."""

load("../cast_checks.star", "load_text", "strip_ansi", "assert_no_local_paths", "assert_no_error_output", "assert_no_experimental", "assert_colored")

def validate():
    text = load_text("../../website/static/casts/cli/command-discovery.cast")
    plain = strip_ansi(text)
    assert_no_local_paths(text)
    assert_no_error_output(text)
    assert_no_experimental(text)
    assert_colored(text, "https://atmos.tools/cli/commands/terraform/usage")
    for needle in [
        "atmos terraform --help=usage",
        "Documentation:",
        "atmos terraform --example",
        "Quick Start (Simple) (recommended)",
        "Quick Start (Advanced)",
        "Initialize this example?",
        "Destination: /demo/quick-start-simple",
        "--force=false",
        "Fetching source",
        "Initialized quick-start-simple",
        "Example: Quick Start Simple",
        "atmos list stacks",
        "dev",
        "staging",
        "prod",
    ]:
        if needle not in plain:
            fail("command discovery cast is missing: " + needle)
    if not regex.search(r"Yes\s+No", plain):
        fail("confirmation buttons are not visible")
    ui.success("Command discovery cast validated")

validate()
