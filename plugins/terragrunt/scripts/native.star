# Opt-in CLI integration tests with real Terraform/OpenTofu and local state.
# Fixtures use built-in terraform_data resources, never cloud resources.
fixtures = module.load("fixtures.star")


def snapshot(root):
    # Detect any project writes by inventory commands before execution begins.
    return {
        path: digest.sha256(fs.read_file(path))
        for path in fs.glob(root + "/**/*")
        if fs.stat(path).is_file
    }


def prepare_fixture(work, package, binary, case):
    root = work + "/" + case.name
    fixtures.copy_tree(package + "/fixtures/" + case.name, root)
    config = fixtures.configuration(package, scope=case.scope, binary=binary)
    fixtures.write(root, "atmos.yaml", json.encode(config))
    return struct(root=root, case=case)


def terraform(fixture, operation, arguments=[], revision="r1"):
    command = ["terraform", operation, "--all", "-s", fixture.case.stack, "--"]
    result = fixtures.run(
        fixture.root,
        command + arguments + ["-no-color"],
        revision=revision,
    )
    return result.stdout


def verify_read_only_inventory(fixture):
    before = snapshot(fixture.root)
    result = fixtures.run(fixture.root, ["list", "instances", "--format=json"])
    instances = json.decode(result.stdout)
    components = [row["Component"] for row in instances]
    stacks_match = all([row["Stack"] == fixture.case.stack for row in instances])
    fixtures.check(components == fixture.case.expected, "Unexpected instances")
    fixtures.check(stacks_match, "Unexpected stack names")

    fixtures.run(fixture.root, ["list", "stacks", "--format=json"])
    result = fixtures.run(fixture.root, ["describe", "stacks", "--format=json"])
    fixtures.check(fixture.case.stack in json.decode(result.stdout), "Stack not described")
    fixtures.check(before == snapshot(fixture.root), "Discovery changed project files")


def apply_and_verify_unchanged_plan(fixture):
    terraform(fixture, "apply", ["-auto-approve"])
    # Exit two indicates drift; fixtures.run requires zero from this plan.
    plan = terraform(fixture, "plan", ["-detailed-exitcode"])
    fixtures.check(plan.count("No changes.") == 4, plan)
    state_files = fs.glob(fixture.root + fixture.case.state_pattern)
    fixtures.check(len(state_files) == 4, "Expected four local state files")
    return state_files


def verify_fresh_dependency_outputs(fixture, state_files):
    # A changed network input must reach app through database in this same apply.
    terraform(fixture, "apply", ["-auto-approve"], revision="r2")
    values = [
        json.decode(fs.read_file(path))["outputs"]["value"]["value"]
        for path in state_files
    ]
    app = [value for value in values if value["unit"] == "app"][0]
    database = app["upstream"]["database"]
    network = database["upstream"]["network"]
    fixtures.check(network["revision"] == "r2", "Dependency outputs were stale")


def destroy_and_verify_empty_state(fixture, state_files):
    # Exercise reverse dependency ordering, then inspect every unit's local state.
    terraform(fixture, "destroy", ["-auto-approve"])
    for path in state_files:
        state = json.decode(fs.read_file(path))
        fixtures.check(not state["resources"], "Destroy left local resources")


def run_fixture(work, package, binary, case):
    fixture = prepare_fixture(work, package, binary, case)
    verify_read_only_inventory(fixture)
    if case.name == "advanced":
        # Command-scoped mocks permit a plan before the dependencies are applied.
        terraform(fixture, "plan")
    state_files = apply_and_verify_unchanged_plan(fixture)
    if case.name == "advanced":
        verify_fresh_dependency_outputs(fixture, state_files)
    destroy_and_verify_empty_state(fixture, state_files)
    ui.success(case.name + ": inventory, apply, unchanged plans, outputs, reverse destroy")


def main():
    package = fs.resolve(".")
    # Prefer OpenTofu and pass the absolute executable through Atmos preparation.
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    work = fixtures.workspace("atmos-terragrunt-native-")
    cases = [
        struct(
            name="advanced",
            scope="live/dev/**",
            stack="live/dev",
            expected=["app", "cache", "database", "network"],
            state_pattern="/live/dev/*/state/terraform.tfstate",
        ),
        struct(
            name="explicit-stack",
            scope="live/**",
            stack="live",
            expected=["dev/app", "dev/network", "prod/app", "prod/network"],
            state_pattern="/state/*/*.tfstate",
        ),
    ]
    for case in cases:
        run_fixture(work, package, binary, case)

main()
