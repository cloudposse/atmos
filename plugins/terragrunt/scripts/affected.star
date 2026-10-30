# Read-only Git comparison scenarios. No Terraform binary, state, or cloud resources.
fixtures = module.load("fixtures.star")


def unit(dependency=None):
    text = 'terraform { source = "../../../modules/unit" }\n'
    if dependency:
        text += 'dependency "network" { config_path = "../' + dependency + '" }\n'
    return text


def seed_units(root):
    fixtures.write(root, "modules/unit/main.tf", 'output "vpc_id" { value = "fixture" }\n')
    fixtures.write(root, "live/prod/network/terragrunt.hcl", unit())
    fixtures.write(root, "live/prod/app/terragrunt.hcl", unit("network"))
    return fixtures.commit(root, "initial units")


def affected(root, ref, *flags):
    result = fixtures.run(root, [
        "describe", "affected", "--base", ref, "--format=json",
    ] + list(flags))
    fixtures.check("treating BASE as empty" not in result.stderr, result.stderr)
    return json.decode(result.stdout)


def expect_components(rows, expected):
    actual = sorted([row["component"] for row in rows])
    fixtures.check(actual == sorted(expected), "Unexpected affected components: " + str(actual))


def dry_plan(root, base, *flags):
    return fixtures.run(root, [
        "terraform", "plan", "--affected", "--ref", base, "--dry-run",
    ] + list(flags)).stderr


def test_rewired_dependency(root):
    # BASE must retain the old inventory and graph; expansion must use HEAD.
    base = seed_units(root)
    fixtures.check(not affected(root, base), "Unchanged units were selected")
    fixtures.write(root, "live/prod/network-v2/terragrunt.hcl", unit())
    fixtures.write(root, "live/prod/app/terragrunt.hcl", unit("network-v2"))
    fixtures.commit(root, "replace app dependency")
    expect_components(affected(root, base), ["app", "network-v2"])

    base = fixtures.git(root, "rev-parse", "HEAD")
    fixtures.write(root, "live/prod/network-v2/terragrunt.hcl", unit() + "# changed input\n")
    fixtures.commit(root, "change only new network")
    expect_components(affected(root, base), ["network-v2"])
    expanded = affected(root, base, "--include-dependents", "--flatten")
    expect_components(expanded, ["app", "network-v2"])

    plan = dry_plan(root, base, "--include-dependents")
    fixtures.check("Would plan network-v2 in live/prod" in plan, plan)
    fixtures.check("Would plan app in live/prod" in plan, plan)
    fixtures.check("Would plan network in" not in plan, plan)


def test_deleted_unit(root):
    # Removed identities belong in the report, but must never be executed.
    base = seed_units(root)
    fixtures.git(root, "rm", "-r", "live/prod/app")
    fixtures.commit(root, "remove app")
    removed = affected(root, base)
    expect_components(removed, ["app"])
    fixtures.check(removed[0]["deleted"], "Removed unit was not marked deleted")
    fixtures.check("Would plan" not in dry_plan(root, base), "Deleted unit selected for execution")


def test_file_watches_and_rename(root):
    # Shared HCL and module files must select all consumers, even when unit HCL is unchanged.
    seed_units(root)
    fixtures.write(root, "shared.hcl", 'locals { region = "east" }\n')
    for name in ["network", "app"]:
        path = "live/prod/" + name + "/terragrunt.hcl"
        content = 'include "shared" { path = "../../../shared.hcl" }\n'
        fixtures.write(root, path, content + fs.read_file(root + "/" + path))
    base = fixtures.commit(root, "include shared configuration")
    fixtures.write(root, "shared.hcl", 'locals { region = "west" }\n')
    fixtures.commit(root, "change shared configuration")
    expect_components(affected(root, base), ["app", "network"])

    base = fixtures.git(root, "rev-parse", "HEAD")
    fixtures.write(root, "modules/unit/main.tf", 'output "vpc_id" { value = "changed" }\n')
    fixtures.commit(root, "change module")
    expect_components(affected(root, base), ["app", "network"])

    base = fixtures.git(root, "rev-parse", "HEAD")
    fixtures.git(root, "mv", "live/prod/app", "live/prod/application")
    fixtures.commit(root, "rename unit")
    renamed = affected(root, base)
    expect_components(renamed, ["app", "application"])
    old = [row for row in renamed if row["component"] == "app"][0]
    fixtures.check(old["deleted"], "Rename must remove the old identity")


def test_nested_stacks(root, package):
    # Generated units retain provenance to the checked-in stack, on both revisions.
    fixtures.copy_tree(package + "/fixtures/explicit-stack", root)
    base = fixtures.commit(root, "explicit stacks")
    fixtures.check(not affected(root, base), "Unchanged explicit stacks selected")
    path = "live/terragrunt.stack.hcl"
    fixtures.write(root, path, fs.read_file(root + "/" + path) + "\n# changed definition\n")
    fixtures.commit(root, "change explicit root")
    expect_components(affected(root, base), [
        "dev/app", "dev/network", "prod/app", "prod/network",
    ])
    fixtures.check(not fs.exists(root + "/live/.terragrunt-stack"), "Discovery wrote generated stacks")
    fixtures.check(not fs.glob(root + "/**/*.tfstate"), "Discovery created state")


def test_mixed_graph(root, configuration):
    # Output and ordering edges compose with YAML consumers and existing selection flags.
    seed_units(root)
    configuration["stacks"] = {"base_path": "stacks", "included_paths": ["**/*.yaml"]}
    fixtures.write(root, "atmos.yaml", json.encode(configuration))
    fixtures.write(root, "live/prod/api/terragrunt.hcl", unit() + 'dependencies { paths = ["../app"] }\n')
    consumer = {
        "source": "./modules/unit",
        "dependencies": {
            "components": [{"name": "api", "stack": "live/prod", "kind": "terraform"}],
        },
    }
    stack = {"name": "native", "components": {"terraform": {"native": consumer}}}
    fixtures.write(root, "stacks/native.yaml", json.encode(stack))
    base = fixtures.commit(root, "mixed YAML and HCL dependency chain")
    fixtures.check(not affected(root, base), "Unchanged mixed inventory selected")
    fixtures.write(root, "live/prod/network/terragrunt.hcl", unit() + "# changed upstream\n")
    fixtures.commit(root, "change upstream in mixed graph")

    expect_components(affected(root, base), ["network"])
    expanded = affected(root, base, "--include-dependents", "--flatten")
    expect_components(expanded, ["api", "app", "native", "network"])
    filtered = affected(root, base, "--stack=live/prod", "--include-dependents", "--flatten")
    expect_components(filtered, ["api", "app", "network"])
    plan = dry_plan(root, base, "--include-dependents=1")
    fixtures.check("Would plan network in" in plan and "Would plan app in" in plan, plan)
    fixtures.check("Would plan api in" not in plan and "Would plan native in" not in plan, plan)


def test_project_root(root):
    # Watch paths use Atmos base_path, not the CLI's working directory.
    fixtures.write(root, "project/live/prod/network/terragrunt.hcl", unit())
    fixtures.write(root, "project/modules/unit/main.tf", 'output "vpc_id" { value = "fixture" }\n')
    base = fixtures.commit(root, "project under base_path")
    fixtures.check(not affected(root, base), "Unchanged subdirectory inventory selected")
    fixtures.write(root, "project/modules/unit/main.tf", 'output "vpc_id" { value = "changed" }\n')
    fixtures.commit(root, "change module under base_path")
    expect_components(affected(root, base), ["network"])


def main():
    package = fs.resolve(".")
    work = fixtures.workspace("atmos-terragrunt-affected-")
    for name, scenario in [
        ("rewired-dependency", test_rewired_dependency),
        ("deleted-unit", test_deleted_unit),
        ("file-watches-and-rename", test_file_watches_and_rename),
    ]:
        root = work + "/" + name
        fixtures.project(root, package)
        scenario(root)
        ui.success(name)

    fixtures.project(work + "/nested", package)
    test_nested_stacks(work + "/nested", package)
    config = fixtures.project(work + "/mixed", package)
    test_mixed_graph(work + "/mixed", config)
    fixtures.project(work + "/subdirectory", package, base_path="project")
    test_project_root(work + "/subdirectory")
    ui.success("Nested stacks, mixed graphs, and project-relative watches")
