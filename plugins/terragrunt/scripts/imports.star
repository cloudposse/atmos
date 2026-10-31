# Opt-in CLI acceptance tests for terragrunt:// imports, discovery patterns, and symlinks.
# Every fixture is generated here, uses only built-in terraform_data resources, and never
# touches a cloud. Importing this module has no effects.
fixtures = module.load("fixtures.star")

MODULE = """
variable "name" {
  type    = string
  default = "unset"
}
resource "terraform_data" "this" {
  input = var.name
}
output "name" {
  value = var.name
}
"""


def unit(name, dependency=None):
    text = 'terraform { source = "../../../modules/m" }\ninputs = { name = "' + name + '" }\n'
    if dependency:
        text += 'dependency "dep" {\n  config_path  = "../' + dependency + '"\n  mock_outputs = { name = "mock" }\n}\n'
    return text


def project(work, name, package, binary, discovery=True, scope="live/**", excluded=None):
    root = work + "/" + name
    fixtures.project(root, package)
    config = fixtures.configuration(package, scope=scope, binary=binary)
    settings = config["components"]["terraform"]["terragrunt"]["discovery"]
    settings["enabled"] = discovery
    if excluded:
        settings["excluded_paths"] = excluded
    config["stacks"] = {"base_path": "stacks", "included_paths": ["*.yaml"], "excluded_paths": ["mixins/**"]}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "modules/m/main.tf", MODULE)
    for path, text in [("dev/vpc", unit("dev/vpc")), ("dev/app", unit("dev/app", "vpc")), ("prod/vpc", unit("prod/vpc")), ("prod/db", unit("prod/db"))]:
        fixtures.write(root, "live/" + path + "/terragrunt.hcl", text)
    return root


def instances(root, *flags):
    rows = json.decode(fixtures.run(root, ["list", "instances", "--format=json"] + list(flags)).stdout)
    return sorted([row["Stack"] + "/" + row["Component"] for row in rows])


def stack_file(root, name, imports, extra=""):
    text = "import:\n" + "".join(["  - " + item + "\n" for item in imports]) + extra
    fixtures.write(root, "stacks/" + name + ".yaml", text)


def test_import_joins_the_importing_stack(work, package, binary):
    root = project(work, "joins", package, binary, discovery=False)
    # A mixin imported by one stack, a direct directory import, and a single-file import.
    stack_file(root, "mixins/dev-units", ["terragrunt://live/dev"])
    stack_file(root, "dev", ["mixins/dev-units"])
    stack_file(root, "staging", ["mixins/dev-units", "terragrunt://live/prod/db/terragrunt.hcl"])
    expected = ["dev/app", "dev/vpc", "staging/app", "staging/db", "staging/vpc"]
    fixtures.check(instances(root) == expected, "Unexpected instances: " + str(instances(root)))
    fixtures.check(instances(root, "--stack", "staging") == ["staging/app", "staging/db", "staging/vpc"], "Unexpected staging instances")

    # No stack is derived from the unit's directory: the importing stacks are the only ones.
    stacks = fixtures.run(root, ["list", "stacks"]).stdout.split()
    fixtures.check(sorted(stacks) == ["dev", "staging"], "Imports created stacks: " + str(stacks))
    described = json.decode(fixtures.run(root, ["describe", "stacks", "--format=json"]).stdout)
    fixtures.check(sorted(described.keys()) == ["dev", "staging"], "describe stacks created stacks: " + str(described.keys()))
    app = described["staging"]["components"]["terraform"]["app"]
    fixtures.check(app["dependencies"]["components"][0]["name"] == "vpc", "A unit depends on its sibling in the importing stack")

    # The unit runs in whichever stack imported it.
    for stack in ["dev", "staging"]:
        planned = fixtures.run(root, ["terraform", "plan", "vpc", "-s", stack, "--", "-no-color"])
        fixtures.check("terraform_data.this" in planned.stdout, "plan vpc -s " + stack + ":\n" + planned.stdout)
    component = json.decode(fixtures.run(root, ["describe", "component", "vpc", "-s", "dev", "--format=json"]).stdout)
    fixtures.check(component["settings"]["configuration"]["source"] == "live/dev/vpc/terragrunt.hcl", "Unexpected unit: " + str(component["settings"]))
    return root


def test_import_with_discovery(work, package, binary):
    root = project(work, "mixed", package, binary, excluded=["live/dev/app"])
    stack_file(root, "dev", ["terragrunt://live/dev"])
    # live/dev is imported (even the unit excluded from discovery); live/prod is discovered into
    # its own derived stacks, and nothing is both.
    expected = ["dev/app", "dev/vpc", "live/prod/db", "live/prod/vpc"]
    fixtures.check(instances(root) == expected, "Unexpected instances: " + str(instances(root)))


def test_conflicts(work, package, binary):
    root = project(work, "conflicts", package, binary, discovery=False)
    # Two units with one name in one stack.
    stack_file(root, "both", ["terragrunt://live/dev/vpc", "terragrunt://live/prod/vpc"])
    result = fixtures.run_failing(root, ["list", "instances"])
    output = result.stdout + result.stderr
    fixtures.check("live/dev/vpc/terragrunt.hcl" in output and "live/prod/vpc/terragrunt.hcl" in output, "Both units must be named:\n" + output)
    fs.remove(root + "/stacks/both.yaml")

    # A YAML component with the name of an imported unit.
    stack_file(root, "clash", ["terragrunt://live/dev/vpc"], "components:\n  terraform:\n    vpc:\n      vars:\n        a: 1\n")
    result = fixtures.run_failing(root, ["list", "instances"])
    output = result.stdout + result.stderr
    fixtures.check("live/dev/vpc/terragrunt.hcl" in output and "clash" in output and "vpc" in output, "The unit and the manifest must be named:\n" + output)
    fs.remove(root + "/stacks/clash.yaml")

    # An import that selects no unit is an error, not an empty result.
    fixtures.write(root, "catalog/terragrunt.hcl", "locals { shared = true }\n")
    stack_file(root, "empty", ["terragrunt://catalog"])
    result = fixtures.run_failing(root, ["list", "instances"])
    fixtures.check("matched no components" in result.stdout + result.stderr, "Empty import must be explained:\n" + result.stdout + result.stderr)


def test_patterns_select_alike(work, package, binary):
    expected = ["live/dev/app", "live/dev/vpc"]
    for scope in ["live/dev", "live/dev/", "./live/dev", "live/dev/*", "live/dev/**"]:
        root = project(work, "include-" + scope.replace("/", "_").replace("*", "S").replace(".", "D"), package, binary, scope=scope)
        fixtures.check(instances(root) == expected, scope + " selected " + str(instances(root)))
    for excluded in ["live/prod", "live/prod/", "./live/prod", "live/prod/*", "live/prod/**"]:
        root = project(work, "exclude-" + excluded.replace("/", "_").replace("*", "S").replace(".", "D"), package, binary, scope="./**", excluded=[excluded])
        fixtures.check(instances(root) == expected, excluded + " left " + str(instances(root)))
    root = project(work, "warns", package, binary, scope="live/nothing")
    result = fixtures.run(root, ["list", "instances"])
    fixtures.check("live/nothing" in result.stderr and "matched no" in result.stderr, "An unmatched pattern must be reported:\n" + result.stderr)


def symlink(root, path, target):
    # Starlark cannot create links, so git materializes them (mode 120000) in the disposable repo.
    fixtures.write(root, "scratch/link-target", target)
    blob = fixtures.git(root, "hash-object", "-w", "scratch/link-target")
    fs.remove(root + "/scratch", recursive=True)
    fixtures.git(root, "update-index", "--add", "--cacheinfo", "120000," + blob + "," + path)
    fixtures.git(root, "checkout-index", "-f", path)
    return fs.stat(root + "/" + path).is_symlink


def test_symlinks_follow_upstream(work, package, binary):
    root = project(work, "links", package, binary)
    if not symlink(root, "live/linked", "prod") or not symlink(root, "live/dev/copy/terragrunt.hcl", "../vpc/terragrunt.hcl"):
        ui.info("Symbolic links are unavailable on this host; skipping the symlink scenario")
        return
    found = instances(root)
    fixtures.check("live/dev/copy" in found and "live/dev/vpc" in found, "A symlinked terragrunt.hcl is a unit at its own location: " + str(found))
    fixtures.check(not any(["linked" in item for item in found]), "A symlinked directory is not followed: " + str(found))
    result = fixtures.run(root, ["list", "instances"])
    fixtures.check("live/linked" in result.stderr and "not followed" in result.stderr, "The skipped link must be reported:\n" + result.stderr)
    fixtures.check(instances(root) == found, "Discovery must be deterministic")


def imports_and_discovery():
    package = fs.resolve(".")
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    work = fixtures.workspace("atmos-terragrunt-imports-")
    test_import_joins_the_importing_stack(work, package, binary)
    test_import_with_discovery(work, package, binary)
    test_conflicts(work, package, binary)
    test_patterns_select_alike(work, package, binary)
    test_symlinks_follow_upstream(work, package, binary)
