# Opt-in CLI acceptance tests with real Terraform/OpenTofu and local state for native
# Terragrunt behavior that native.star does not cover: hooks and extra_arguments, saved
# plans, state and output consumers, and module sources. Every fixture is generated from
# this script, uses only built-in terraform_data resources, and never touches a cloud.
#
# Importing this module has no effects; each integration step calls one scenario function.
fixtures = module.load("fixtures.star")

STACK = "live/dev"

# Module whose output proves which arguments, variable files, and environment Terraform saw.
UNIT_MODULE = """
variable "greeting" {
  type    = string
  default = "unset"
}
variable "suffix" {
  type    = string
  default = ""
}
variable "tag" {
  type    = string
  default = ""
}
variable "envvar" {
  type    = string
  default = ""
}
resource "terraform_data" "unit" {
  input = "${var.greeting}|${var.suffix}|${var.tag}|${var.envvar}"
}
output "value" {
  value = terraform_data.unit.output
}
"""

# Terraform state is kept beside the unit so tests can read it without running Terraform.
BACKEND = """
remote_state {
  backend = "local"
  config  = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}
"""

# Hooks run the selected Terraform executable itself, so they work on every platform and
# need no shell. Their output is the only side effect the tests read.
HOOKED_UNIT = """
terraform {
  source = "../../../modules/unit"

  extra_arguments "inline" {
    commands  = ["plan", "apply"]
    arguments = ["-var", "greeting=@GREETING@"]
    env_vars  = { TF_VAR_envvar = "from-env" }
  }
  extra_arguments "files" {
    commands           = ["plan", "apply"]
    required_var_files = ["${get_terragrunt_dir()}/required.tfvars"]
    optional_var_files = ["${get_terragrunt_dir()}/optional.tfvars", "${get_terragrunt_dir()}/absent.tfvars"]
  }
  # Scoped to another command: it must never reach plan or apply.
  extra_arguments "destroy-only" {
    commands  = ["destroy"]
    arguments = ["-var", "greeting=destroy-only"]
  }

  before_hook "init-hook" {
    commands = ["init"]
    execute  = ["@TOOL@", "version"]
  }
  before_hook "plan-hook" {
    commands = ["plan"]
    execute  = ["@TOOL@", "version"]
  }
  before_hook "apply-before" {
    commands = ["apply"]
    execute  = ["@TOOL@", "output", "-no-color"]
  }
  after_hook "apply-after" {
    commands = ["apply"]
    execute  = ["@TOOL@", "output", "-no-color"]
  }
  # Nothing fails here, so no error hook may run.
  error_hook "never-fires" {
    commands  = ["apply"]
    execute   = ["@TOOL@", "version"]
    on_errors = [".*"]
  }
}
"""

# A before hook that exits with Terraform fmt's "needs formatting" status gates the apply.
GATED_UNIT = """
terraform {
  source = "../../../modules/unit"

  before_hook "format-gate" {
    commands = ["apply"]
    execute  = ["@TOOL@", "fmt", "-check", "${get_terragrunt_dir()}/unformatted.tf"]
  }
  after_hook "after-skipped" {
    commands = ["apply"]
    execute  = ["@TOOL@", "version"]
  }
  after_hook "after-always" {
    commands     = ["apply"]
    execute      = ["@TOOL@", "version"]
    run_on_error = true
  }
  error_hook "matching" {
    commands  = ["apply"]
    execute   = ["@TOOL@", "version"]
    on_errors = ["format-gate"]
  }
  error_hook "non-matching" {
    commands  = ["apply"]
    execute   = ["@TOOL@", "version"]
    on_errors = ["NO_SUCH_FAILURE_TEXT"]
  }
  error_hook "other-command" {
    commands  = ["plan"]
    execute   = ["@TOOL@", "version"]
    on_errors = ["format-gate"]
  }
}
"""


def hcl_path(path):
    # HCL strings treat backslashes as escapes; forward slashes work on every platform.
    return path.replace("\\", "/")


def project(work, name, package, binary, stacks=False):
    root = work + "/" + name
    config = fixtures.configuration(package, binary=binary)
    if stacks:
        config["stacks"] = {"base_path": "stacks", "included_paths": ["**/*.yaml"]}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "modules/unit/main.tf", UNIT_MODULE)
    return root


def write_unit(root, name, content):
    fixtures.write(root, "live/dev/" + name + "/terragrunt.hcl", content + BACKEND)


def terraform(root, operation, unit, arguments=[], flags=[], failing=False):
    command = ["terraform", operation, unit, "-s", STACK] + flags + ["--"] + arguments + ["-no-color"]
    if failing:
        return fixtures.run_failing(root, command)
    return fixtures.run(root, command)


def state_path(root, unit):
    return root + "/live/dev/" + unit + "/state/terraform.tfstate"


def state_output(root, unit, name="value"):
    # Read the local state Terraform wrote, independent of any Atmos function under test.
    state = json.decode(fs.read_file(state_path(root, unit)))
    return state["outputs"][name]["value"]


def expect_order(text, markers, context):
    # Markers must appear in this order; every marker is required.
    position = -1
    for marker in markers:
        index = text.find(marker, position + 1)
        fixtures.check(index >= 0, "Missing or misordered '" + marker + "' in " + context + ":\n" + text)
        position = index


def expect_count(text, marker, count, context):
    actual = text.count(marker)
    fixtures.check(actual == count, "Expected " + str(count) + " x '" + marker + "', got " + str(actual) + " in " + context + ":\n" + text)


def tool_of(binary):
    return hcl_path(binary)


def hooked_unit(binary, greeting):
    return HOOKED_UNIT.replace("@TOOL@", tool_of(binary)).replace("@GREETING@", greeting)


def verify_hook_and_argument_ordering(root, binary):
    fixtures.write(root, "live/dev/hooked/required.tfvars", 'tag = "required"\n')
    fixtures.write(root, "live/dev/hooked/optional.tfvars", 'suffix = "optional"\n')
    write_unit(root, "hooked", hooked_unit(binary, "one"))

    # The first plan initializes implicitly: init hooks fire for that init, plan hooks only for plan.
    plan = terraform(root, "plan", "hooked")
    expect_count(plan.stderr, "Executing before hook: init-hook", 1, "plan stderr")
    expect_count(plan.stderr, "Executing before hook: plan-hook", 1, "plan stderr")
    expect_order(plan.stderr, ["Executing before hook: init-hook", "Executing before hook: plan-hook"], "plan stderr")
    expect_count(plan.stderr, "apply-before", 0, "plan stderr")
    # Extra arguments, both variable files, and the extra environment reached OpenTofu.
    fixtures.check("one|optional|required|from-env" in plan.stdout, "Extra arguments did not reach plan:\n" + plan.stdout)
    fixtures.check("destroy-only" not in plan.stdout, "A destroy-only argument reached plan:\n" + plan.stdout)
    fixtures.check(not fs.exists(state_path(root, "hooked")), "Plan created state")

    # Apply: init is already current, so no init hook; the plan-scoped hook must not fire.
    apply = terraform(root, "apply", "hooked", ["-auto-approve"])
    expect_count(apply.stderr, "Executing before hook: plan-hook", 0, "apply stderr")
    expect_count(apply.stderr, "Executing before hook: init-hook", 0, "apply stderr")
    expect_count(apply.stderr, "Executing error hook", 0, "apply stderr")
    # The before hook saw empty state; the after hook saw what the apply wrote.
    expect_order(apply.stderr, [
        "Executing before hook: apply-before",
        "No outputs found",
        "Executing after hook: apply-after",
        'value = "one|optional|required|from-env"',
    ], "apply stderr")
    fixtures.check(state_output(root, "hooked") == "one|optional|required|from-env", "Unexpected applied value")

    # A second apply with a new argument: the before hook reads the old value, the after hook the new one.
    write_unit(root, "hooked", hooked_unit(binary, "two"))
    apply = terraform(root, "apply", "hooked", ["-auto-approve"])
    expect_order(apply.stderr, [
        "Executing before hook: apply-before",
        'value = "one|optional|required|from-env"',
        "Executing after hook: apply-after",
        'value = "two|optional|required|from-env"',
    ], "second apply stderr")
    fixtures.check(state_output(root, "hooked") == "two|optional|required|from-env", "Second apply value is stale")


def verify_missing_required_var_file(root):
    # A required variable file that is absent is rejected before OpenTofu runs; optional ones are not.
    path = root + "/live/dev/hooked/required.tfvars"
    content = fs.read_file(path)
    fs.remove(path)
    result = terraform(root, "plan", "hooked", failing=True)
    fixtures.check("required.tfvars" in result.stderr + result.stdout, "Missing var file was not named:\n" + result.stderr)
    fs.write_file(path, content)


def verify_failing_before_hook(root, binary):
    write_unit(root, "gated", GATED_UNIT.replace("@TOOL@", tool_of(binary)))
    fixtures.write(root, "live/dev/gated/unformatted.tf", 'variable   "x"   {}\n')
    result = terraform(root, "apply", "gated", ["-auto-approve"], failing=True)
    err = result.stderr
    # OpenTofu fmt -check exits 3 when files need formatting; Atmos must preserve that status.
    fixtures.check(result.exit_code == 3, "Hook exit code was not preserved: " + str(result.exit_code) + "\n" + err)
    expect_count(err, "Executing before hook: format-gate", 1, "gated stderr")
    # The failed before hook stopped the action; no apply output, no state.
    fixtures.check("Apply complete" not in result.stdout, "Apply ran after a failing before hook")
    fixtures.check(not fs.exists(state_path(root, "gated")), "Failing before hook still created state")
    # After hooks run after a failure only with run_on_error.
    expect_count(err, "Executing after hook: after-skipped", 0, "gated stderr")
    expect_count(err, "Executing after hook: after-always", 1, "gated stderr")
    # Error hooks run only for a matching error expression and a listed command.
    expect_count(err, "Executing error hook: matching", 1, "gated stderr")
    expect_count(err, "Executing error hook: non-matching", 0, "gated stderr")
    expect_count(err, "Executing error hook: other-command", 0, "gated stderr")
    expect_order(err, [
        "Executing before hook: format-gate",
        "Executing after hook: after-always",
        "Executing error hook: matching",
    ], "gated stderr")


def hooks():
    package = fs.resolve(".")
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    root = project(fixtures.workspace("atmos-terragrunt-hooks-"), "project", package, binary)
    verify_hook_and_argument_ordering(root, binary)
    verify_missing_required_var_file(root)
    verify_failing_before_hook(root, binary)
    ui.success("Hooks and extra_arguments: ordering, scoping, exit codes, error hooks, var files, env")


PLAN_UNIT = """
terraform {
  source = "../../../modules/unit"

  extra_arguments "inline" {
    commands  = ["plan", "apply"]
    arguments = ["-var", "greeting=@GREETING@"]
  }
}
"""


def planfiles(root):
    # Saved plans live beside the module Terraform ran in, next to their private binding.
    return fs.glob(root + "/.workdir/terraform/" + STACK + "-planned-*/*.planfile")


def plan_unit(root, greeting):
    write_unit(root, "planned", PLAN_UNIT.replace("@GREETING@", greeting))
    result = terraform(root, "plan", "planned")
    files = planfiles(root)
    fixtures.check(len(files) == 1, "Expected one saved plan, found " + str(files))
    fixtures.check(fs.exists(files[0] + ".atmos.json"), "Saved plan has no Atmos binding")
    return struct(result=result, planfile=files[0])


def apply_saved(root, failing=False):
    return terraform(root, "apply", "planned", flags=["--from-plan"], failing=failing)


def verify_rejected(root, result, reason):
    output = result.stdout + result.stderr
    fixtures.check("plan verification failed" in output, "Rejection was not a plan verification failure:\n" + output)
    fixtures.check(reason in output, "Rejection did not explain '" + reason + "':\n" + output)
    fixtures.check("Apply complete" not in output, "A rejected plan was applied")
    fixtures.check(not fs.exists(state_path(root, "planned")), "A rejected plan created state")


def saved_plan():
    package = fs.resolve(".")
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    root = project(fixtures.workspace("atmos-terragrunt-plan-"), "project", package, binary)

    # Tampering with the reviewed plan artifact is rejected before anything is applied.
    saved = plan_unit(root, "reviewed")
    fixtures.check("reviewed" in saved.result.stdout, "Plan did not show the reviewed value")
    fixtures.check(not fs.exists(state_path(root, "planned")), "Planning created state")
    fs.write_file(saved.planfile, "tampered", overwrite=True)
    verify_rejected(root, apply_saved(root, failing=True), "create a new plan")

    # So is a changed working directory, even when the plan bytes are intact.
    saved = plan_unit(root, "reviewed")
    injected = saved.planfile.rsplit("/", 1)[0] + "/injected.tf"
    fs.write_file(injected, 'output "injected" { value = "unreviewed" }\n')
    verify_rejected(root, apply_saved(root, failing=True), "working directory changed")
    fs.remove(injected)

    # Editing the unit after review changes the plan's identity, so the plan is rejected too.
    saved = plan_unit(root, "reviewed")
    write_unit(root, "planned", PLAN_UNIT.replace("@GREETING@", "edited-after-review"))
    verify_rejected(root, apply_saved(root, failing=True), "create a new plan")

    # With the reviewed definition back, the plan applies exactly what was reviewed and does
    # not plan again.
    write_unit(root, "planned", PLAN_UNIT.replace("@GREETING@", "reviewed"))
    applied = apply_saved(root)
    fixtures.check("Plan:" not in applied.stdout, "Applying a saved plan planned again:\n" + applied.stdout)
    fixtures.check("Apply complete! Resources: 1 added" in applied.stdout, applied.stdout)
    fixtures.check(state_output(root, "planned") == "reviewed|||", "Applied value differs from the reviewed plan")
    ui.success("Saved plans: exact apply without re-planning; tampered plan and workdir rejected")


CONSUMER_MODULE = """
variable "from_output" {
  type = string
}
variable "from_state" {
  type = string
}
resource "terraform_data" "consumer" {
  input = { output = var.from_output, state = var.from_state }
}
"""

# An ordinary Atmos component reads a Terragrunt unit through both YAML functions. The stack
# is YAML text because the functions are YAML tags, which a JSON document cannot express.
CONSUMER_STACK = """
components:
  terraform:
    consumer:
      vars:
        from_output: !terraform.output producer live/dev value
        from_state: !terraform.state producer live/dev value
    orphan-consumer:
      metadata:
        component: consumer
      vars:
        from_output: !terraform.output orphan live/dev value
        from_state: !terraform.state orphan live/dev value
"""


def described_vars(root, component):
    result = fixtures.run(root, ["describe", "component", component, "-s", "consumers", "--format=json"])
    return json.decode(result.stdout)["vars"]


def project_files(root):
    # Every project file, including dot directories such as .workdir.
    return sorted([path for path in fs.glob(root + "/**/*") if fs.stat(path).is_file] + fs.glob(root + "/.workdir/**/*"))


def verify_unprovisioned_state_is_rejected(root):
    before = project_files(root)
    fixtures.check(not fs.exists(root + "/.workdir"), "Fixture started with a workdir")
    result = fixtures.run_failing(root, ["terraform", "plan", "orphan-consumer", "-s", "consumers", "--", "-no-color"])
    fixtures.check("terraform state not provisioned for component orphan in stack live/dev" in result.stderr + result.stdout, "Unclear error:\n" + result.stderr)
    # Reading must not create workdirs, workspaces, or state for the unit that was never applied.
    fixtures.check(project_files(root) == before, "Reading unprovisioned state changed project files")
    fixtures.check(not fs.exists(root + "/.workdir"), "Reading unprovisioned state created a workdir")
    fixtures.check(not fs.glob(root + "/**/terraform.tfstate*"), "Reading unprovisioned state created state")
    fixtures.check(not fs.exists(state_path(root, "orphan")), "Reading unprovisioned state created unit state")


def state_and_outputs():
    package = fs.resolve(".")
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    root = project(fixtures.workspace("atmos-terragrunt-state-"), "project", package, binary, stacks=True)
    fixtures.write(root, "components/terraform/consumer/main.tf", CONSUMER_MODULE)
    fixtures.write(root, "stacks/consumers.yaml", CONSUMER_STACK)
    write_unit(root, "producer", PLAN_UNIT.replace("@GREETING@", "first"))
    write_unit(root, "orphan", PLAN_UNIT.replace("@GREETING@", "never-applied"))

    verify_unprovisioned_state_is_rejected(root)

    # After the unit is applied, both functions resolve to what Terraform stored.
    terraform(root, "apply", "producer", ["-auto-approve"])
    resolved = described_vars(root, "consumer")
    fixtures.check(resolved["from_output"] == "first|||", "Output function returned " + str(resolved))
    fixtures.check(resolved["from_state"] == "first|||", "State function returned " + str(resolved))

    # The ordinary component consumes the values when it is applied.
    fixtures.run(root, ["terraform", "apply", "consumer", "-s", "consumers", "--", "-auto-approve", "-no-color"])
    consumer_state = fs.glob(root + "/components/terraform/consumer/terraform.tfstate.d/*/terraform.tfstate")
    fixtures.check(len(consumer_state) == 1, "Expected one consumer state file")
    attributes = json.decode(fs.read_file(consumer_state[0]))["resources"][0]["instances"][0]["attributes"]
    fixtures.check(attributes["input"]["value"] == {"output": "first|||", "state": "first|||"}, "Consumer received " + str(attributes))

    # A later change to the unit is visible to the next resolution; nothing is cached on disk.
    write_unit(root, "producer", PLAN_UNIT.replace("@GREETING@", "second"))
    terraform(root, "apply", "producer", ["-auto-approve"])
    resolved = described_vars(root, "consumer")
    fixtures.check(resolved["from_output"] == "second|||" and resolved["from_state"] == "second|||", "Stale value: " + str(resolved))
    ui.success("State and output integration: resolved values, consumer apply, fresh reads, unprovisioned state rejected")


def versioned_module(version):
    return 'resource "terraform_data" "module" {\n  input = "' + version + '"\n}\noutput "version" {\n  value = terraform_data.module.output\n}\n'


def source_unit(source):
    return 'terraform {\n  source = "' + source + '"\n}\n'


def applied_version(root, unit):
    return state_output(root, unit, "version")


def create_module_repository(work):
    # A local repository with two tagged module revisions replaces any network module source.
    repo = work + "/module-repository"
    fs.mkdir(repo, parents=True)
    fixtures.git(repo, "init", "-b", "main")
    fixtures.git(repo, "config", "user.name", "Atmos fixture")
    fixtures.git(repo, "config", "user.email", "fixture@example.invalid")
    fixtures.write(repo, "module/main.tf", versioned_module("module-v1"))
    fixtures.commit(repo, "module v1")
    fixtures.git(repo, "tag", "v1")
    fixtures.write(repo, "module/main.tf", versioned_module("module-v2"))
    fixtures.commit(repo, "module v2")
    fixtures.git(repo, "tag", "v2")
    url = hcl_path(repo)
    return "git::file://" + ("" if url.startswith("/") else "/") + url + "//module?ref="


def git_source_refresh():
    package = fs.resolve(".")
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    work = fixtures.workspace("atmos-terragrunt-git-source-")
    source = create_module_repository(work)
    root = project(work, "project", package, binary)
    write_unit(root, "remote", source_unit(source + "v1"))
    terraform(root, "apply", "remote", ["-auto-approve"])
    fixtures.check(applied_version(root, "remote") == "module-v1", "The tagged module was not fetched and applied")

    # Moving the reference to a newer tag must replace the module code provisioned earlier.
    write_unit(root, "remote", source_unit(source + "v2"))
    terraform(root, "apply", "remote", ["-auto-approve"])
    fixtures.check(applied_version(root, "remote") == "module-v2", "A changed Git ref kept the previously provisioned module")
    ui.success("Local Git module source: fetched at v1, refreshed at v2")


def local_source_refresh():
    package = fs.resolve(".")
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    root = project(fixtures.workspace("atmos-terragrunt-local-source-"), "project", package, binary)
    fixtures.write(root, "modules/local/main.tf", versioned_module("local-v1"))
    write_unit(root, "local", source_unit("../../../modules/local"))
    terraform(root, "apply", "local", ["-auto-approve"])
    fixtures.check(applied_version(root, "local") == "local-v1", "The local module was not applied")

    # Editing the module in place must reach the next run through the retained workdir.
    fixtures.write(root, "modules/local/main.tf", versioned_module("local-v2"))
    terraform(root, "apply", "local", ["-auto-approve"])
    fixtures.check(applied_version(root, "local") == "local-v2", "An edited local module kept the previously provisioned code")
    ui.success("Local path module source: applied at v1, refreshed at v2")
