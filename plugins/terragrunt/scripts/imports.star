# Opt-in CLI acceptance tests for terragrunt:// imports, discovery patterns, and symlinks.
# Every fixture is generated here, uses only built-in terraform_data resources, and never
# touches a cloud. Importing this module has no effects.
fixtures = module.load("fixtures.star")
hardening = module.load("hardening.star")
lifecycle = module.load("lifecycle_extended.star")

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

    # A YAML component with the name of an imported unit is an override, not a conflict.
    stack_file(root, "clash", ["terragrunt://live/dev/vpc"], "components:\n  terraform:\n    vpc:\n      vars:\n        a: 1\n")
    fixtures.check(instances(root, "--stack", "clash") == ["clash/vpc"], "A YAML component named like a unit must merge: " + str(instances(root)))
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


OVERRIDE_MODULE = """
variable "name" {
  type    = string
  default = "unset"
}
variable "tags" {
  type    = map(string)
  default = {}
}
variable "extra" {
  type    = string
  default = "module-default"
}
variable "zones" {
  type    = list(string)
  default = []
}
resource "terraform_data" "this" {
  input = var.name
}
output "zones" {
  value = var.zones
}
output "name" {
  value = var.name
}
output "tags" {
  value = var.tags
}
output "extra" {
  value = var.extra
}
output "workspace" {
  value = terraform.workspace
}
"""

OVERRIDE_VPC = """
terraform { source = "../../../modules/o" }
inputs = {
  name  = "from-hcl"
  extra = "from-hcl"
  tags  = { team = "hcl", env = "dev" }
  zones = ["a", "b"]
}
"""

# The unit sets TF_VAR_extra through extra_arguments and gives no input for it.
OVERRIDE_ENVU = """
terraform {
  source = "../../../modules/o"
  extra_arguments "e" {
    commands = ["plan", "apply"]
    env_vars = { TF_VAR_extra = "unit-env" }
  }
}
inputs = { name = "envu" }
"""

UNIT_IMPORTS = ["terragrunt://live/over/vpc", "terragrunt://live/over/envu"]


def override_project(work, package, binary):
    root = work + "/override"
    fixtures.project(root, package)
    config = fixtures.configuration(package, binary=binary)
    config["components"]["terraform"]["terragrunt"]["discovery"]["enabled"] = False
    config["stacks"] = {"base_path": "stacks", "included_paths": ["*.yaml"], "excluded_paths": ["mixins/**"]}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "modules/o/main.tf", OVERRIDE_MODULE)
    fixtures.write(root, "live/over/vpc/terragrunt.hcl", OVERRIDE_VPC)
    fixtures.write(root, "live/over/envu/terragrunt.hcl", OVERRIDE_ENVU)
    return root


def apply_outputs(root, unit, stack):
    result = fixtures.run(root, ["terraform", "apply", unit, "-s", stack, "--", "-auto-approve", "-no-color"])
    outputs = json.decode(fixtures.run(root, ["terraform", "output", unit, "-s", stack, "--", "-json"]).stdout)
    return {"values": {key: outputs[key]["value"] for key in outputs}, "result": result}


def check_rejected(root, stack, unit, expected):
    result = fixtures.run_failing(root, ["terraform", "plan", unit, "-s", stack, "--", "-no-color"])
    output = result.stdout + result.stderr
    for text in expected:
        fixtures.check(text in output, "Expected '" + text + "' in:\n" + output)


def test_stack_overrides(work, package, binary):
    root = override_project(work, package, binary)
    fixtures.write(root, "stacks/plain.yaml", "import:\n  - " + "\n  - ".join(UNIT_IMPORTS) + "\n")
    fixtures.write(root, "stacks/override.yaml", "import:\n  - " + "\n  - ".join(UNIT_IMPORTS) + """
components:
  terraform:
    vpc:
      metadata:
        terraform_workspace: blue
      vars:
        name: from-stack
        zones: [c]
        tags:
          team: stack
      settings:
        owner: platform
    envu:
      env:
        TF_VAR_extra: stack-env
      providers:
        terraform: {}
      hooks:
        announce:
          kind: step
          type: log
          events: [after.terraform.apply]
          with:
            level: warn
            content: stack-hook-ran
""")

    # Without a YAML component the unit's own inputs and extra_arguments environment apply.
    plain = apply_outputs(root, "vpc", "plain")["values"]
    fixtures.check(plain["name"] == "from-hcl" and plain["extra"] == "from-hcl" and plain["tags"] == {"team": "hcl", "env": "dev"}, "Unexpected unit values: " + str(plain))
    fixtures.check(plain["workspace"] == "default", "Unexpected workspace: " + str(plain))
    fixtures.check(apply_outputs(root, "envu", "plain")["values"]["extra"] == "unit-env", "The unit's environment was not applied")

    # The YAML component deep-merges over the unit: a scalar replaces, a map merges key by key,
    # untouched inputs stay, and metadata, settings, env, hooks, and providers all take effect.
    over = apply_outputs(root, "vpc", "override")["values"]
    fixtures.check(over["name"] == "from-stack", "A stack var did not override the unit input: " + str(over))
    fixtures.check(over["tags"] == {"team": "stack", "env": "dev"}, "Tags must merge deeply: " + str(over))
    fixtures.check(over["extra"] == "from-hcl", "An untouched unit input was lost: " + str(over))
    fixtures.check(over["zones"] == ["c"], "A list is replaced under the default list merge strategy: " + str(over))
    fixtures.check(over["workspace"] == "blue", "metadata.terraform_workspace was ignored: " + str(over))
    again = apply_outputs(root, "vpc", "plain")["values"]
    fixtures.check(again["name"] == "from-hcl" and again["workspace"] == "default", "Another stack imports the same unit without the override: " + str(again))
    varfiles = fs.glob(root + "/.workdir/terraform/override-vpc-*/atmos-*.terraform.tfvars.json")
    fixtures.check(len(varfiles) == 1, "Expected one generated var file: " + str(varfiles))
    fixtures.check(json.decode(fs.read_file(varfiles[0])) == {"name": "from-stack", "extra": "from-hcl", "zones": ["c"], "tags": {"team": "stack", "env": "dev"}}, "The generated var file is not the merged result: " + fs.read_file(varfiles[0]))
    envu = apply_outputs(root, "envu", "override")
    fixtures.check(envu["values"]["extra"] == "stack-env", "Stack env must win over the unit's environment: " + str(envu["values"]))
    fixtures.check("stack-hook-ran" in envu["result"].stderr + envu["result"].stdout, "The stack's Atmos hook did not run for the unit:\n" + envu["result"].stderr)
    fixtures.check(len(fs.glob(root + "/.workdir/terraform/override-envu-*/providers_override.tf.json")) == 1, "The stack's providers section was not generated")
    component = json.decode(fixtures.run(root, ["describe", "component", "vpc", "-s", "override", "--format=json"]).stdout)
    fixtures.check(component["vars"] == {"name": "from-stack", "zones": ["c"], "tags": {"team": "stack"}}, "describe shows the stack layer: " + str(component["vars"]))
    fixtures.check(component["settings"]["owner"] == "platform" and component["settings"]["configuration"]["source"] == "live/over/vpc/terragrunt.hcl", "Settings must merge with the unit marker: " + str(component["settings"]))

    # Abstract components feed a unit-named component like they feed any other; !unset removes a
    # value of the stack layers, so the unit's own input shows again.
    fixtures.write(root, "stacks/inherits.yaml", "import:\n  - terragrunt://live/over/vpc\n" + """
components:
  terraform:
    base:
      metadata:
        type: abstract
      vars:
        name: from-base
        extra: from-base
        tags:
          base: b
          team: base
    vpc:
      metadata:
        inherits: [base]
        terraform_workspace: inherited
      vars:
        extra: !unset
        tags:
          added: c
""")
    inherited = apply_outputs(root, "vpc", "inherits")["values"]
    fixtures.check(inherited["name"] == "from-base", "An abstract base must override the unit input: " + str(inherited))
    fixtures.check(inherited["extra"] == "from-hcl", "!unset removes the base value, the unit input remains: " + str(inherited))
    fixtures.check(inherited["tags"] == {"base": "b", "team": "base", "added": "c", "env": "dev"}, "Unexpected inherited tags: " + str(inherited))

    # A stack map merges over the unit input map, and null replaces a unit input with null.
    fixtures.write(root, "stacks/replaced.yaml", "import:\n  - terragrunt://live/over/vpc\n" + """
components:
  terraform:
    base:
      metadata:
        type: abstract
      vars:
        tags:
          base: b
          team: base
    vpc:
      metadata:
        inherits: [base]
        terraform_workspace: replaced
      vars:
        extra: null
        tags:
          only: stack
""")
    replaced = apply_outputs(root, "vpc", "replaced")["values"]
    fixtures.check(replaced["tags"] == {"only": "stack", "base": "b", "team": "base", "env": "dev"}, "Stack layers merge over the unit input: " + str(replaced))
    fixtures.check("extra" not in replaced, "null must replace the unit input: " + str(replaced))

    # A YAML component in an import that comes before the unit's import still overrides the unit.
    fixtures.write(root, "stacks/mixins/catalog.yaml", "components:\n  terraform:\n    vpc:\n      metadata:\n        terraform_workspace: early\n      vars:\n        name: catalog-name\n")
    fixtures.write(root, "stacks/early.yaml", "import:\n  - mixins/catalog\n  - terragrunt://live/over/vpc\n")
    early = apply_outputs(root, "vpc", "early")["values"]
    fixtures.check(early["name"] == "catalog-name" and early["workspace"] == "early", "An earlier import must override the unit: " + str(early))

    # Locking a unit from the stack blocks changes, like any component.
    fixtures.write(root, "stacks/locked.yaml", "import:\n  - terragrunt://live/over/vpc\ncomponents:\n  terraform:\n    vpc:\n      metadata:\n        locked: true\n")
    locked = fixtures.run_failing(root, ["terraform", "apply", "vpc", "-s", "locked", "--", "-auto-approve", "-no-color"])
    fixtures.check("locked" in locked.stderr + locked.stdout, "A locked unit must refuse apply:\n" + locked.stderr)

    # Settings the unit owns cannot be overridden, and the error says why.
    fixtures.write(root, "stacks/backend.yaml", "import:\n  - terragrunt://live/over/vpc\ncomponents:\n  terraform:\n    vpc:\n      backend_type: s3\n      backend:\n        s3:\n          bucket: other\n")
    check_rejected(root, "backend", "vpc", ["backend_type", "remote_state", "owned"])
    fixtures.write(root, "stacks/module.yaml", "import:\n  - terragrunt://live/over/vpc\ncomponents:\n  terraform:\n    vpc:\n      metadata:\n        component: other\n")
    check_rejected(root, "module", "vpc", ["metadata.component", "other", "owned"])
    fixtures.write(root, "stacks/source.yaml", "import:\n  - terragrunt://live/over/vpc\ncomponents:\n  terraform:\n    vpc:\n      source:\n        uri: ./modules/other\n")
    check_rejected(root, "source", "vpc", ["source.uri", "modules/other", "terragrunt.hcl"])
    # A global backend in the stack is not a component override: the unit keeps its own state.
    fixtures.write(root, "stacks/global-backend.yaml", "import:\n  - terragrunt://live/over/vpc\nterraform:\n  backend_type: s3\n  backend:\n    s3:\n      bucket: global\ncomponents:\n  terraform:\n    vpc:\n      metadata:\n        terraform_workspace: shielded\n")
    shielded = apply_outputs(root, "vpc", "global-backend")["values"]
    fixtures.check(shielded["workspace"] == "shielded", "A global backend must not capture a unit: " + str(shielded))
    ui.success("Stack overrides: vars, env, settings, metadata, hooks, providers, inherits, !unset, null, lists, earlier imports; owned settings rejected")


def stack_pin(version):
    return "dependencies:\n  tools:\n    opentofu: \"" + version + "\"\n"


def test_stack_pinned_tools(work, package, binary):
    if "tofu" not in binary.replace("\\", "/").rsplit("/", 1)[-1]:
        ui.warning("Stack pinned tool scenario needs OpenTofu; skipped")
        return
    suffix = exec.run(["go", "env", "GOEXE"], output="capture").stdout.strip()
    tools = work + "/pin-tools"
    versions = ["9.9.9", "9.9.8", "9.9.7", "9.9.6"]
    probes = {version: hardening.install_probe(tools, version, binary, suffix) for version in versions}
    config = fixtures.configuration(package, binary="tofu")
    config["toolchain"] = {"install": "never", "install_path": tools}
    config["stacks"] = {"base_path": "stacks", "included_paths": ["*.yaml"]}
    root = work + "/pin"
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, ".tool-versions", "opentofu 9.9.9\n")
    fixtures.write(root, "modules/unit/main.tf", lifecycle.UNIT_MODULE)
    lifecycle.write_unit(root, "producer", lifecycle.PLAN_UNIT.replace("@GREETING@", "from-producer"))
    lifecycle.write_unit(root, "consumer", hardening.CONSUMER_UNIT)
    fixtures.write(root, "live/solo/lone/terragrunt.hcl", lifecycle.PLAN_UNIT.replace("@GREETING@", "lone") + lifecycle.BACKEND)
    imports = "import:\n  - terragrunt://live/dev\n"
    terraform = "terraform:\n  dependencies:\n    tools:\n      opentofu: \"9.9.7\"\n"
    component = "components:\n  terraform:\n" + "".join(["    " + unit + ":\n      dependencies:\n        tools:\n          opentofu: \"9.9.6\"\n" for unit in ["producer", "consumer"]])
    fixtures.write(root, "stacks/pin-global.yaml", imports + stack_pin("9.9.8"))
    fixtures.write(root, "stacks/pin-terraform.yaml", imports + stack_pin("9.9.8") + terraform)
    fixtures.write(root, "stacks/pin-component.yaml", imports + stack_pin("9.9.8") + terraform + component)
    ambient = work + "/pin-empty-path"
    fs.mkdir(ambient, parents=True)

    def used(version):
        log = probes[version] + "/invocations.log"
        return fs.read_file(log).splitlines() if fs.exists(log) else []

    # The pin from the stack manifest reaches an imported unit at global, terraform, and component
    # level, nearest wins, and every consumer of the tool uses it with no tool on the ambient PATH.
    for stack, version in [("pin-global", "9.9.8"), ("pin-terraform", "9.9.7"), ("pin-component", "9.9.6")]:
        for probe in versions:
            if fs.exists(probes[probe] + "/invocations.log"):
                fs.remove(probes[probe] + "/invocations.log")
        applied = hardening.pinned_invoke(root, ["terraform", "apply", "producer", "-s", stack, "--", "-auto-approve", "-no-color"], ambient)
        fixtures.check(applied.exit_code == 0, stack + ": apply failed:\n" + hardening.output_of(applied))
        planned = hardening.pinned_invoke(root, ["terraform", "plan", "consumer", "-s", stack, "--", "-no-color"], ambient)
        fixtures.check(planned.exit_code == 0, stack + ": plan failed:\n" + hardening.output_of(planned))
        commands = [line.split(" ")[0] for line in used(version)]
        for command in ["init", "apply", "plan", "output"]:
            fixtures.check(command in commands, stack + ": " + version + " never ran '" + command + "': " + str(commands))
        fixtures.check(len([command for command in commands if command == "version"]) >= 2, stack + ": run_cmd and the hook should use " + version + ": " + str(commands))
        for other in versions:
            if other != version:
                fixtures.check(used(other) == [], stack + ": " + other + " must not run: " + str(used(other)))

    # A unit that is only discovered has no stack manifest and takes .tool-versions.
    for probe in versions:
        if fs.exists(probes[probe] + "/invocations.log"):
            fs.remove(probes[probe] + "/invocations.log")
    lone = hardening.pinned_invoke(root, ["terraform", "plan", "lone", "-s", "live/solo", "--", "-no-color"], ambient)
    fixtures.check(lone.exit_code == 0, "plan of a discovered unit failed:\n" + hardening.output_of(lone))
    fixtures.check("plan" in [line.split(" ")[0] for line in used("9.9.9")], "A discovered unit must use .tool-versions: " + str(used("9.9.9")))
    fixtures.check(used("9.9.8") == [] and used("9.9.7") == [] and used("9.9.6") == [], "A discovered unit must not use stack pins")
    ui.success("Stack pinned tools: global, terraform, and component pins reach imported units; discovered units use .tool-versions")


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


def stack_overrides():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    test_stack_overrides(fixtures.workspace("atmos-terragrunt-overrides-"), fs.resolve("."), binary)


def stack_pinned_tools():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    test_stack_pinned_tools(fixtures.workspace("atmos-terragrunt-stack-pins-"), fs.resolve("."), binary)
