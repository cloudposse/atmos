load("../cast_checks.star", "load_text", "strip_ansi", "assert_no_experimental", "assert_colored", "assert_no_error_output", "assert_no_local_paths")

def validate():
    text = load_text("../../website/static/casts/demo/fixtures/starlark/release-plan.cast")
    plain = strip_ansi(text)
    assert_no_experimental(text)
    assert_no_error_output(text)
    assert_no_local_paths(text)
    assert_colored(text, "All 3 component plans ready.")
    for needle in [
        "interpreter: starlark",
        'load("components.star", "plan_component")',
        "steps.parallel(",
        "function = plan_component",
        "max_concurrency = 2",
        "atmos release-plan api -s dev",
        "Release plan for api in dev",
        "All 3 component plans ready. No infrastructure changed.",
    ]:
        if needle not in plain:
            fail("starlark release-plan cast missing {!r}".format(needle))
    rows = [
        "api: v1.4.0 | replicas=2 | region=us-east-1",
        "worker: v1.4.0 | replicas=3 | region=us-east-1",
        "scheduler: v1.4.0 | replicas=1 | region=us-east-1",
    ]
    positions = [plain.index(row) for row in rows]
    if positions != sorted(positions) or any([plain.count(row) != 1 for row in rows]):
        fail("component results must appear once, in input order")
    for bad in ["unknown flag", "Traceback", "[inspect-step]", "[plan]"]:
        if bad in plain:
            fail("starlark release-plan cast contains {!r}".format(bad))
    ui.success("Starlark cast validated")

validate()
