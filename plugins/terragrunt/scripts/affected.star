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


def test_input_watches(root):
    # Inputs stay unevaluated, but optional files and glob membership affect the unit.
    seed_units(root)
    fixtures.write(root, "live/prod/app/terragrunt.hcl", unit("network") + '''
inputs = {
  value = file("input.json")
  members = fileset("config", "*.yaml")
  optional = fileexists("optional.txt") ? file("optional.txt") : "absent"
}
''')
    base = fixtures.commit(root, "declare unevaluated file inputs")
    fixtures.write(root, "live/prod/app/optional.txt", "new input")
    fixtures.commit(root, "create optional file")
    expect_components(affected(root, base), ["app"])
    base = fixtures.git(root, "rev-parse", "HEAD")
    fixtures.write(root, "live/prod/app/config/added.yaml", "key: value")
    fixtures.commit(root, "add glob member")
    expect_components(affected(root, base), ["app"])
    base = fixtures.git(root, "rev-parse", "HEAD")
    fixtures.git(root, "rm", "live/prod/app/config/added.yaml")
    fixtures.commit(root, "delete glob member")
    expect_components(affected(root, base), ["app"])

    # Locals that resolve to literals name an exact file, so unrelated project files select nothing.
    fixtures.write(root, "live/prod/app/terragrunt.hcl", unit("network") + '''
locals { input_path = "config.json" }
inputs = { value = file(local.input_path) }
''')
    base = fixtures.commit(root, "declare literal-local file input")
    fixtures.write(root, "new-directory/config.json", "{}")
    fixtures.commit(root, "create unrelated file with the same name")
    expect_components(affected(root, base), [])

    # Unknown path expressions (the environment is never read) watch project sources conservatively, never caches.
    fixtures.write(root, "live/prod/app/terragrunt.hcl", unit("network") + '''
inputs = { value = file("${get_env("INPUT_DIR", "")}/config.json") }
''')
    base = fixtures.commit(root, "declare dynamic file input")
    fixtures.write(root, "new-directory/config.json", '{"possible": "input"}')
    fixtures.commit(root, "create possible dynamic input")
    expect_components(affected(root, base), ["app"])
    base = fixtures.git(root, "rev-parse", "HEAD")
    fixtures.write(root, "live/prod/app/.terragrunt-cache/generated.tf", "# generated")
    fixtures.commit(root, "record cache content")
    expect_components(affected(root, base), [])


def selected_after(root, label, *edits):
    # Commit the edits on top of the current HEAD and report which units the change selects.
    base = fixtures.git(root, "rev-parse", "HEAD")
    for path, content in edits:
        fixtures.write(root, path, content)
    fixtures.commit(root, label)
    return sorted([row["component"] for row in affected(root, base)])


def expect_selected(root, label, expected, *edits):
    actual = selected_after(root, label, *edits)
    fixtures.check(actual == sorted(expected), label + ": expected " + str(expected) + ", selected " + str(actual))


HOOKS_UNIT = '''
terraform {
  source = "../../../modules/unit"
  extra_arguments "vars" {
    commands           = ["plan"]
    required_var_files = ["${get_terragrunt_dir()}/extra.tfvars"]
    optional_var_files = ["${get_terragrunt_dir()}/optional.tfvars"]
  }
  before_hook "check" {
    commands = ["plan"]
    execute  = ["../../../scripts/hook.sh"]
  }
}
generate "extra" {
  path      = "extra.tf"
  if_exists = "overwrite"
  contents  = file("gen.tpl")
}
'''


def test_unevaluated_blocks(root):
    # Files named only by extra_arguments, generate, and hooks are not evaluated during
    # discovery, yet an edit to any of them changes what the unit plans.
    seed_units(root)
    fixtures.write(root, "live/prod/hooks/terragrunt.hcl", HOOKS_UNIT)
    fixtures.write(root, "live/prod/hooks/extra.tfvars", "name = \"one\"\n")
    fixtures.write(root, "live/prod/hooks/gen.tpl", "# generated one\n")
    fixtures.write(root, "scripts/hook.sh", "echo one\n")
    fixtures.write(root, "docs/guide.md", "guide\n")
    fixtures.commit(root, "unit with var files, generate, and hook")
    expect_selected(root, "required var file", ["hooks"], ("live/prod/hooks/extra.tfvars", "name = \"two\"\n"))
    expect_selected(root, "optional var file created", ["hooks"], ("live/prod/hooks/optional.tfvars", "name = \"three\"\n"))
    expect_selected(root, "generate template", ["hooks"], ("live/prod/hooks/gen.tpl", "# generated two\n"))
    expect_selected(root, "hook script", ["hooks"], ("scripts/hook.sh", "echo two\n"))
    expect_selected(root, "lock file beside the unit", ["hooks"], ("live/prod/hooks/.terraform.lock.hcl", "# lock\n"))
    expect_selected(root, "unrelated documentation", [], ("docs/guide.md", "guide two\n"))


def test_copied_source_root(root):
    # Terragrunt copies everything before `//`, so a sibling of the selected subdirectory counts.
    seed_units(root)
    fixtures.write(root, "modules/sub/app/main.tf", 'module "shared" { source = "../shared" }\n')
    fixtures.write(root, "modules/sub/shared/main.tf", 'output "v" { value = 1 }\n')
    fixtures.write(root, "live/prod/subdir/terragrunt.hcl", 'terraform { source = "../../../modules/sub//app" }\n')
    fixtures.commit(root, "unit selecting a module subdirectory")
    expect_selected(root, "sibling of the selected subdirectory", ["subdir"], ("modules/sub/shared/main.tf", 'output "v" { value = 2 }\n'))
    expect_selected(root, "selected subdirectory", ["subdir"], ("modules/sub/app/main.tf", '# edited\nmodule "shared" { source = "../shared" }\n'))
    expect_selected(root, "unrelated module", [], ("modules/unused/main.tf", "# unused\n"), ("README.md", "readme\n"))


def test_symlinked_module(root):
    # Git reports edits under the symlink target, not the path the unit names.
    seed_units(root)
    fixtures.write(root, "modules/other/main.tf", 'output "vpc_id" { value = "other" }\n')
    fixtures.write(root, "scratch/link-target", "other")
    blob = fixtures.git(root, "hash-object", "-w", "scratch/link-target")
    fs.remove(root + "/scratch", recursive=True)
    fixtures.git(root, "update-index", "--add", "--cacheinfo", "120000," + blob + ",modules/linked")
    fixtures.git(root, "checkout-index", "-f", "modules/linked")
    if not fs.stat(root + "/modules/linked", follow_symlinks=False).is_symlink:
        ui.info("Symbolic links are unavailable on this host; skipping the symlinked module scenario")
        return
    fixtures.write(root, "live/prod/linked/terragrunt.hcl", 'terraform { source = "../../../modules/linked" }\n')
    fixtures.write(root, "live/prod/direct/terragrunt.hcl", 'terraform { source = "../../../modules/other" }\n')
    fixtures.commit(root, "units using a symlinked module and its target")
    expect_selected(root, "edit through the symlink target", ["direct", "linked"], ("modules/other/main.tf", 'output "vpc_id" { value = "edited" }\n'))
    expect_selected(root, "unrelated module", [], ("modules/unused/main.tf", "# unused\n"))


def test_dynamic_path_precision(root):
    # get_terragrunt_dir() style paths resolve to exact files; only unresolvable expressions
    # (here: the environment) keep the conservative project-wide watch.
    seed_units(root)
    fixtures.write(root, "live/prod/exact/terragrunt.hcl", unit() + '''
locals { name = "settings" }
inputs = {
  a = file("${get_terragrunt_dir()}/a.json")
  b = file(format("%s/%s.json", get_terragrunt_dir(), local.name))
  c = file(find_in_parent_folders("shared.json"))
}
''')
    fixtures.write(root, "live/prod/exact/a.json", "{}")
    fixtures.write(root, "live/prod/exact/settings.json", "{}")
    fixtures.write(root, "live/shared.json", "{}")
    fixtures.write(root, "live/prod/env/terragrunt.hcl", unit() + 'inputs = { a = file("${get_env("DYNAMIC_DIR", "")}/a.json") }\n')
    fixtures.commit(root, "units with dynamic file inputs")
    expect_selected(root, "unrelated files", ["env"], ("README.md", "readme\n"), ("docs/guide.md", "guide\n"))
    expect_selected(root, "get_terragrunt_dir file", ["env", "exact"], ("live/prod/exact/a.json", '{"a": 1}'))
    expect_selected(root, "format and local file", ["env", "exact"], ("live/prod/exact/settings.json", '{"b": 1}'))
    expect_selected(root, "find_in_parent_folders file", ["env", "exact"], ("live/shared.json", '{"c": 1}'))


def test_submodule_module_source(root):
    # A submodule is empty in the BASE checkout; comparing a revision with itself selects nothing.
    seed_units(root)
    repository = fs.mkdtemp(prefix="atmos-terragrunt-submodule-")
    defer(fs.remove, repository, recursive=True)
    fixtures.git(repository, "init", "-b", "main")
    fixtures.git(repository, "config", "user.name", "Atmos fixture")
    fixtures.git(repository, "config", "user.email", "fixture@example.invalid")
    fixtures.write(repository, "main.tf", 'output "vpc_id" { value = "submodule" }\n')
    fixtures.commit(repository, "submodule module")
    fixtures.git(root, "-c", "protocol.file.allow=always", "submodule", "add", repository, "modules/submodule")
    fixtures.write(root, "live/prod/submodule/terragrunt.hcl", 'terraform { source = "../../../modules/submodule" }\n')
    fixtures.commit(root, "unit using a submodule module")
    fixtures.check(not affected(root, "HEAD"), "A submodule module made an unchanged unit affected")
    expect_selected(root, "unrelated change", [], ("README.md", "readme\n"))


def test_explicit_imports(root, config):
    # Explicit imports opt into roots without enabling automatic filesystem discovery.
    seed_units(root)
    config["components"]["terraform"]["terragrunt"]["discovery"]["enabled"] = False
    config["stacks"] = {"base_path": "stacks", "included_paths": ["**/*"]}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "stacks/adopt.yaml", 'import: ["terragrunt://live", "terragrunt://live/prod/app/terragrunt.hcl"]\n')
    base = fixtures.commit(root, "adopt roots explicitly")
    instances = json.decode(fixtures.run(root, ["list", "instances", "--format=json"]).stdout)
    fixtures.check(sorted([row["Component"] for row in instances]) == ["app", "network"], "Explicit imports changed instance names")
    expect_components(affected(root, base), [])
    fixtures.write(root, "live/prod/network/terragrunt.hcl", unit() + "# import-selected source changed\n")
    fixtures.commit(root, "change imported dependency")
    expect_components(affected(root, base, "--include-dependents", "--flatten"), ["app", "network"])


def main():
    package = fs.resolve(".")
    work = fixtures.workspace("atmos-terragrunt-affected-")
    for name, scenario in [
        ("rewired-dependency", test_rewired_dependency),
        ("deleted-unit", test_deleted_unit),
        ("file-watches-and-rename", test_file_watches_and_rename),
        ("input-watches", test_input_watches),
        ("unevaluated-blocks", test_unevaluated_blocks),
        ("copied-source-root", test_copied_source_root),
        ("symlinked-module", test_symlinked_module),
        ("dynamic-path-precision", test_dynamic_path_precision),
        ("submodule-module-source", test_submodule_module_source),
    ]:
        root = work + "/" + name
        fixtures.project(root, package)
        scenario(root)
        ui.success(name)

    config = fixtures.project(work + "/imports", package)
    test_explicit_imports(work + "/imports", config)
    ui.success("Explicit imports without automatic discovery")
    fixtures.project(work + "/nested", package)
    test_nested_stacks(work + "/nested", package)
    config = fixtures.project(work + "/mixed", package)
    test_mixed_graph(work + "/mixed", config)
    fixtures.project(work + "/subdirectory", package, base_path="project")
    test_project_root(work + "/subdirectory")
    ui.success("Nested stacks, mixed graphs, and project-relative watches")
