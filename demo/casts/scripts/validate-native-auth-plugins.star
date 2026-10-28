load("../cast_checks.star", "load_text", "strip_ansi", "assert_no_experimental", "assert_colored", "assert_no_error_output", "assert_no_local_paths")

def validate():
    text = load_text("../../website/static/casts/examples/native-auth-plugins/lifecycle.cast")
    plain = strip_ansi(text)
    assert_no_experimental(text)
    assert_no_error_output(text)
    assert_no_local_paths(text)
    for needle in [
        "atmos plugin create corporate-oidc",
        "atmos --chdir corporate-oidc build",
        "Building plugin",
        "Plugin built successfully",
        "atmos plugin validate corporate-oidc",
        "Validated",
        "Capability",
        "Export",
        "atmos --chdir corporate-oidc test",
        "atmos --chdir corporate-oidc push",
        "Packed",
        "Platforms",
        "Layout",
        "Published",
        "Repository",
        "atmos plugin search oidc --registry local",
        "atmos plugin install local/acme/oidc --as corporate --version 0.1.0",
        "atmos plugin registry list",
        "atmos plugin registry list --format json",
        '"host": "localhost:15556"',
        "atmos plugin list",
        "atmos plugin info corporate",
        "Source",
        "oci://localhost:15556/plugins/acme/oidc",
        "Index digest",
        "Signature",
        "installed",
        "acme/oidc: OIDC PKCE login and AWS credential exchange",
        "No authentication was performed.",
        "atmos auth login --identity <identity>",
        "atmos demo test-downloaded",
        "emulator acme-oidc-registry is down",
    ]:
        if needle not in plain:
            fail("plugin lifecycle cast missing {!r}".format(needle))
    for outcome in ["Plugin contract:", "OIDC lifecycle:", "output and exit status match the fixture", "3 plugin checks passed"]:
        if plain.count(outcome) != 3:
            fail("Expected test, push, and downloaded conformance results: {!r}".format(outcome))
    cached = plain.split("# search the cached index after the registry is stopped", 1)
    if len(cached) != 2 or "acme/oidc" not in cached[1]:
        fail("Expected cached discovery to work after stopping the registry")
    details = plain.split("atmos plugin info corporate", 1)[1].split("# execute the installed package", 1)[0]
    # This fixture uses single-column glyphs; count code points, not UTF-8 bytes.
    if any([len([char for char in line.codepoints()]) > 120 for line in details.splitlines()]):
        fail("Plugin details overflow the 120-column recording")
    installed_list = text.split("atmos plugin list", 1)[1].split("# inspect the installed version", 1)[0]
    assert_colored(installed_list, "●")
    ui.success("Native plugin lifecycle cast validated")

validate()
