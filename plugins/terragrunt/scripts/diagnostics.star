# Opt-in CLI acceptance tests for plugin diagnostics: every rejection names the unit, the file, and
# the setting; one bad unit never breaks the project; bulk runs validate every selected unit before
# any infrastructure command runs; and `atmos config set` can always repair plugin settings.
# Rendered messages drop Markdown backticks, so expectations name plain text.
# Fixtures are generated here and use only built-in terraform_data resources.
fixtures = module.load("fixtures.star")

STACK = "live/dev"

MODULE = """
resource "terraform_data" "unit" {
  input = "ok"
}
"""

BACKEND = """
remote_state {
  backend  = "local"
  config   = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}
"""

GOOD = 'terraform {\n  source = "../../../modules/unit"\n}\n' + BACKEND


def unit(source_block):
    return 'terraform {\n  source = "../../../modules/unit"\n' + source_block + "}\n" + BACKEND


def project(prefix, binary, units):
    root = fixtures.workspace(prefix)
    config = fixtures.configuration(fs.resolve("."), binary=binary)
    config["stacks"] = {"base_path": "stacks", "included_paths": ["**/*.yaml"]}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "modules/unit/main.tf", MODULE)
    # An ordinary YAML stack proves non-Terragrunt components keep working beside a broken unit.
    fixtures.write(root, "stacks/plain.yaml", "components:\n  terraform:\n    plain:\n      vars: {}\n")
    for name in units:
        fixtures.write(root, "live/dev/" + name + "/terragrunt.hcl", units[name])
    return root


def output_of(result):
    return result.stdout + result.stderr


def expect(output, fragments, context):
    for fragment in fragments:
        fixtures.check(fragment in output, context + " is missing '" + fragment + "':\n" + output)


def forbid(output, fragments, context):
    for fragment in fragments:
        fixtures.check(fragment not in output, context + " must not contain '" + fragment + "':\n" + output)


def plan(root, name):
    return output_of(fixtures.run_failing(root, ["terraform", "plan", name, "-s", STACK, "--", "-no-color"]))


def verify_rejections_name_file_unit_and_setting(root):
    # An HCL syntax error reports the file with line and column.
    syntax = plan(root, "syntax")
    expect(syntax, ["Missing argument separator", "live/dev/syntax/terragrunt.hcl", "Component syntax in stack live/dev"], "syntax error")
    fixtures.check(syntax.count("live/dev/syntax/terragrunt.hcl:") > 0, "syntax error has no line and column:\n" + syntax)
    for name, setting, hint in [
        ("binary", "terraform_binary", "components.terraform.command"),
        ("errors", "errors", "retry"),
        ("exclude", "exclude", "excluded_paths"),
        ("iam", "iam_role", "identity"),
        ("feature", "feature", "stack variables"),
        ("lint", "tflint", "wrapper script"),
        ("lifecycle", "init-from-module", "Remove the lifecycle command"),
        ("escape", "generate", "relative"),
    ]:
        output = plan(root, name)
        expect(output, ["live/dev/" + name + "/terragrunt.hcl", "Component " + name + " in stack live/dev", setting, hint], name)
        forbid(output, ["plugin API operation failed", "unsupported plugin extension"], name)


def verify_secrets_stay_out_of_errors(root):
    output = plan(root, "secret")
    expect(output, ["Component secret in stack live/dev", "ATMOS_LOGS_LEVEL=Debug"], "secret-bearing evaluation error")
    forbid(output, ["hunter2"], "secret-bearing evaluation error")


def verify_one_bad_unit_does_not_break_the_project(root):
    instances = output_of(fixtures.run(root, ["list", "instances", "--format=json"]))
    expect(instances, ["good", "syntax", "plain"], "list instances")
    stacks = output_of(fixtures.run(root, ["list", "stacks"]))
    expect(stacks, ["live/dev", "plain"], "list stacks")
    fixtures.run(root, ["describe", "stacks", "--format=json"])
    # The healthy unit and the YAML component still work.
    fixtures.run(root, ["terraform", "plan", "good", "-s", STACK, "--", "-no-color"])


def verify_bulk_runs_validate_before_running(root):
    for operation in ["plan", "apply", "destroy"]:
        arguments = ["terraform", operation, "--all", "-s", STACK, "--"] + (["-auto-approve"] if operation != "plan" else []) + ["-no-color"]
        output = output_of(fixtures.run_failing(root, arguments))
        expect(output, ["no infrastructure command was run", "Component syntax", "Component binary"], operation + " --all")
        fixtures.check(not fs.exists(root + "/live/dev/good/state/terraform.tfstate"), operation + " --all ran a healthy unit before rejecting the broken ones")


def diagnostics():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    units = {
        "good": GOOD,
        "syntax": 'terraform {\n  source = "../../../modules/unit"\n}\ninputs = {\n  a = find_in_parent_folders("x"\n}\n',
        "binary": unit("") + 'terraform_binary = "terraform"\n',
        "errors": unit("") + 'errors {\n  retry "x" {\n    retryable_errors = [".*"]\n    max_attempts = 2\n    sleep_interval_sec = 1\n  }\n}\n',
        "exclude": unit("") + 'exclude {\n  if      = true\n  actions = ["plan"]\n}\n',
        "iam": unit("") + 'iam_role = "arn:aws:iam::123456789012:role/x"\n',
        "feature": unit("") + 'feature "flag" {\n  default = true\n}\n',
        "lint": unit('  before_hook "lint" {\n    commands = ["plan"]\n    execute  = ["tflint"]\n  }\n'),
        "lifecycle": unit('  before_hook "setup" {\n    commands = ["init-from-module"]\n    execute  = ["echo", "x"]\n  }\n'),
        "escape": unit("") + 'generate "x" {\n  path      = "../escape.tf"\n  if_exists = "overwrite"\n  contents  = ""\n}\n',
        "secret": unit("") + 'inputs = {\n  a = tonumber("hunter2")\n}\n',
    }
    root = project("atmos-terragrunt-diagnostics-", binary, units)
    verify_rejections_name_file_unit_and_setting(root)
    verify_secrets_stay_out_of_errors(root)
    verify_one_bad_unit_does_not_break_the_project(root)
    verify_bulk_runs_validate_before_running(root)
    ui.success("Diagnostics: rich unit errors, redaction, isolation, bulk pre-flight")


def config_set(root, key, value, kind=None):
    arguments = ["config", "set"] + (["--type=" + kind] if kind else []) + [key, value]
    return fixtures.run(root, arguments)


def settings_repair():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    root = fixtures.workspace("atmos-terragrunt-settings-")
    config = fixtures.configuration(fs.resolve("."), binary=binary)
    config["components"]["terraform"]["terragrunt"] = {"enabled": True}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "modules/unit/main.tf", MODULE)
    fixtures.write(root, "live/dev/good/terragrunt.hcl", GOOD)

    # Enabling discovery before included_paths must neither lock the project out nor stop `config set`.
    config_set(root, "components.terraform.terragrunt.discovery.enabled", "true")
    instances = output_of(fixtures.run(root, ["list", "instances"]))
    expect(instances, ["good"], "discovery with the default scope")
    config_set(root, "components.terraform.terragrunt.discovery.included_paths", '["live/**"]', kind="yaml")
    fixtures.run(root, ["list", "instances"])

    # A misspelled key and an unsupported mode fail loudly, name the key, and can be repaired.
    config_set(root, "components.terraform.terragrunt.discoverey.enabled", "true")
    output = output_of(fixtures.run_failing(root, ["list", "stacks"]))
    expect(output, ["discoverey"], "misspelled key")
    config_set(root, "components.terraform.terragrunt.mode", "nativ")
    output = output_of(fixtures.run_failing(root, ["list", "stacks"]))
    expect(output, ["components.terraform.terragrunt.mode"], "invalid mode")
    config_set(root, "components.terraform.terragrunt.mode", "native")
    ui.success("Settings: default scope, config set repair, key-naming validation")
