# Opt-in CLI acceptance tests for the hardening of native Terragrunt execution: the lifecycle of
# saved plans, local module refresh, generated files and backends, hook semantics, and pinned
# tools. Every fixture is generated here, uses only built-in terraform_data resources, and never
# touches a cloud. Importing this module has no effects; each integration step calls one scenario.
fixtures = module.load("fixtures.star")
lifecycle = module.load("lifecycle_extended.star")

STACK = lifecycle.STACK


def engine():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    return binary


def new_project(prefix):
    return lifecycle.project(fixtures.workspace(prefix), "project", fs.resolve("."), engine())


def output_of(result):
    return result.stdout + result.stderr


def expect(result, text, context):
    fixtures.check(text in output_of(result), "Expected '" + text + "' in " + context + ":\n" + output_of(result))


def expect_absent(result, text, context):
    fixtures.check(text not in output_of(result), "Did not expect '" + text + "' in " + context + ":\n" + output_of(result))


def workdir_files(root, unit, name):
    return fs.glob(root + "/.workdir/terraform/" + STACK + "-" + unit + "-*/" + name)


SECRET_TAG = "binding-secret-tag"


def saved_plan_lifecycle():
    root = new_project("atmos-terragrunt-planlife-")
    unit = lifecycle.PLAN_UNIT.replace("@GREETING@", "life") + 'inputs = { tag = "' + SECRET_TAG + '" }\n'
    lifecycle.write_unit(root, "planned", unit)

    # Commands that only read or check the unit must leave the reviewed plan in place.
    lifecycle.terraform(root, "plan", "planned")
    planfile = lifecycle.planfiles(root)[0]
    binding = planfile + ".atmos.json"
    for verb in ["validate", "init"]:
        lifecycle.terraform(root, verb, "planned")
        fixtures.check(fs.exists(planfile), "'" + verb + "' deleted the saved plan")
        fixtures.check(fs.exists(binding), "'" + verb + "' deleted the plan binding")

    # The binding proves what was reviewed without holding input values.
    record = fs.read_file(binding)
    fixtures.check(SECRET_TAG not in record, "The plan binding stores an input value:\n" + record)

    # An explicit absolute path verifies exactly like the canonical location.
    applied = lifecycle.terraform(root, "apply", "planned", ["-auto-approve"], flags=["--from-plan=" + planfile])
    fixtures.check("Apply complete! Resources: 1 added" in applied.stdout, applied.stdout)
    fixtures.check(not fs.exists(planfile), "A consumed plan file was kept")
    fixtures.check(not fs.exists(binding), "A consumed plan binding was kept")

    # Applying the same plan again says what happened.
    again = lifecycle.terraform(root, "apply", "planned", ["-auto-approve"], flags=["--from-plan"], failing=True)
    expect(again, "has no saved plan", "second apply of a consumed plan")
    expect_absent(again, "working directory changed", "second apply of a consumed plan")

    # A direct apply makes an earlier plan stale and removes it.
    lifecycle.terraform(root, "plan", "planned")
    fixtures.check(fs.exists(planfile), "The plan was not recreated")
    lifecycle.terraform(root, "apply", "planned", ["-auto-approve"])
    fixtures.check(not fs.exists(planfile), "A direct apply left a stale saved plan")
    stale = lifecycle.terraform(root, "apply", "planned", ["-auto-approve"], flags=["--from-plan"], failing=True)
    expect(stale, "has no saved plan", "apply of a plan made stale by a direct apply")

    # A unit that was never planned has no plan to apply.
    lifecycle.write_unit(root, "fresh", lifecycle.PLAN_UNIT.replace("@GREETING@", "fresh"))
    never = lifecycle.terraform(root, "apply", "fresh", ["-auto-approve"], flags=["--from-plan"], failing=True)
    expect(never, "has no saved plan", "apply of a never planned unit")
    expect_absent(never, "binding is missing", "apply of a never planned unit")
    ui.success("Saved plans: survive read-only commands, explicit paths, consumed on apply, clear errors, no input values stored")


def local_source_stability():
    root = new_project("atmos-terragrunt-localstable-")
    fixtures.write(root, "modules/local/main.tf", lifecycle.versioned_module("local-v1"))
    lifecycle.write_unit(root, "local", lifecycle.source_unit("../../../modules/local"))
    lifecycle.terraform(root, "plan", "local")

    # Nothing changed: the module is neither copied again nor announced as expired.
    quiet = lifecycle.terraform(root, "plan", "local")
    expect_absent(quiet, "Source cache expired", "plan of an unchanged local module")
    expect_absent(quiet, "Local source changed", "plan of an unchanged local module")
    expect_absent(quiet, "Auto-provisioned source", "plan of an unchanged local module")

    # An edit is detected and reaches the plan.
    fixtures.write(root, "modules/local/main.tf", lifecycle.versioned_module("local-v2"))
    edited = lifecycle.terraform(root, "plan", "local")
    expect(edited, "Local source changed", "plan after a module edit")
    expect(edited, "local-v2", "plan after a module edit")
    quiet = lifecycle.terraform(root, "plan", "local")
    expect_absent(quiet, "Local source changed", "second plan after the edit")
    ui.success("Local module: unchanged source is not re-copied, an edit is picked up once")


GENERATE_BLOCK = """
generate "extra" {
  path      = "extra.tf"
  if_exists = "overwrite_terragrunt"
  contents  = "variable \\"stale_marker\\" {\\n  default = \\"x\\"\\n}\\n"
}
"""

ERROR_POLICY_BLOCK = """
generate "provider" {
  path      = "provider.tf"
  if_exists = "error"
  contents  = "# generated provider file\\n"
}
"""

PLAIN_BACKEND_UNIT = """
terraform {
  source = "../../../modules/unit"
}

generate "backend" {
  path              = "backend.tf"
  if_exists         = "overwrite_terragrunt"
  disable_signature = false
  contents          = <<-EOT
    terraform {
      backend "local" {
        path = "@STATE@"
      }
    }
  EOT
}
"""

CACHE_STATE_UNIT = """
terraform {
  source = "../../../modules/unit"
}
"""


def generation_lifecycle():
    root = new_project("atmos-terragrunt-generation-")
    base = lifecycle.PLAN_UNIT.replace("@GREETING@", "gen")

    # A generated file exists while its block does, and stops applying once the block is removed.
    lifecycle.write_unit(root, "gen", base + GENERATE_BLOCK)
    lifecycle.terraform(root, "plan", "gen")
    fixtures.check(len(workdir_files(root, "gen", "extra.tf")) == 1, "The generated file was not written")
    lifecycle.write_unit(root, "gen", base)
    lifecycle.terraform(root, "plan", "gen")
    fixtures.check(len(workdir_files(root, "gen", "extra.tf")) == 0, "A removed generate block still applies its file")

    # if_exists = "error" accepts the file this unit generated on every rerun.
    lifecycle.write_unit(root, "errpolicy", base + ERROR_POLICY_BLOCK)
    for _ in range(3):
        lifecycle.terraform(root, "plan", "errpolicy")

    # A plain generate block that writes a backend replaces the backend Atmos would write.
    state = lifecycle.hcl_path(root) + "/live/dev/plainbackend/state/terraform.tfstate"
    fixtures.write(root, "live/dev/plainbackend/terragrunt.hcl", PLAIN_BACKEND_UNIT.replace("@STATE@", state))
    plain = lifecycle.terraform(root, "plan", "plainbackend")
    expect_absent(plain, "Duplicate backend configuration", "plan with a generated backend block")
    fixtures.check(len(workdir_files(root, "plainbackend", "backend.tf.json")) == 0, "Atmos wrote a second backend file")

    # A lock file kept beside terragrunt.hcl is copied into the working directory.
    lifecycle.write_unit(root, "locked", base)
    fixtures.write(root, "live/dev/locked/.terraform.lock.hcl", "# pinned beside the unit\n")
    lifecycle.terraform(root, "plan", "locked")
    locks = workdir_files(root, "locked", ".terraform.lock.hcl")
    fixtures.check(len(locks) == 1, "The lock file beside the unit was not copied")
    fixtures.check("pinned beside the unit" in fs.read_file(locks[0]), "The copied lock file lost its content")
    ui.success("Generated files: removed blocks stop applying, error policy reruns, generated backend, unit lock file")


def cache_state_warning():
    root = new_project("atmos-terragrunt-cachestate-")
    fixtures.write(root, "live/dev/cache/terragrunt.hcl", CACHE_STATE_UNIT)
    fixtures.write(root, "modules/unit/main.tf", lifecycle.UNIT_MODULE)
    warned = lifecycle.terraform(root, "plan", "cache")
    expect(warned, "keeps its local state", "plan of a unit without remote_state")
    expect(warned, ".terragrunt-cache", "plan of a unit without remote_state")
    expect(warned, "remote_state", "plan of a unit without remote_state")

    # A stable explicit path is not warned about.
    lifecycle.write_unit(root, "stable", lifecycle.PLAN_UNIT.replace("@GREETING@", "stable"))
    quiet = lifecycle.terraform(root, "plan", "stable")
    expect_absent(quiet, "keeps its local state", "plan of a unit with a stable local backend")
    ui.success("State identity: a unit whose state lives in .terragrunt-cache is warned about")


DETAILED_UNIT = """
terraform {
  source = "../../../modules/unit"

  before_hook "state-hook" {
    commands = ["state"]
    execute  = ["@TOOL@", "version"]
  }
  after_hook "after" {
    commands = ["plan"]
    execute  = ["@TOOL@", "version"]
  }
  error_hook "alert" {
    commands  = ["plan"]
    execute   = ["@TOOL@", "version"]
    on_errors = [".*"]
  }
}
"""

BAD_DIRECTORY_UNIT = """
terraform {
  source = "../../../modules/unit"

  before_hook "bad-directory" {
    commands    = ["plan"]
    execute     = ["@TOOL@", "version"]
    working_dir = "${get_terragrunt_dir()}/nodir"
  }
}
"""


def hook_semantics():
    binary = engine()
    root = lifecycle.project(fixtures.workspace("atmos-terragrunt-hooksem-"), "project", fs.resolve("."), binary)
    tool = lifecycle.tool_of(binary)

    # plan -detailed-exitcode returns 2 when changes are present. That is success for hooks.
    lifecycle.write_unit(root, "dx", DETAILED_UNIT.replace("@TOOL@", tool))
    changed = lifecycle.terraform(root, "plan", "dx", ["-detailed-exitcode"], failing=True)
    fixtures.check(changed.exit_code == 2, "Expected exit status 2 for a plan with changes, got " + str(changed.exit_code) + "\n" + output_of(changed))
    expect(changed, "Executing after hook: after", "plan with changes")
    expect_absent(changed, "Executing error hook", "plan with changes")
    # Hook lines name the unit, so bulk output can be attributed.
    expect(changed, "(dx in " + STACK + ")", "plan with changes")

    # A hook that lists "state" runs for every state subcommand.
    lifecycle.terraform(root, "apply", "dx", ["-auto-approve"])
    listed = fixtures.run(root, ["terraform", "state", "list", "dx", "-s", STACK])
    expect(listed, "Executing before hook: state-hook", "state list")

    # A hook working directory that does not exist is reported as such.
    lifecycle.write_unit(root, "baddir", BAD_DIRECTORY_UNIT.replace("@TOOL@", tool))
    failed = lifecycle.terraform(root, "plan", "baddir", failing=True)
    expect(failed, "nodir", "hook with a missing working_dir")
    expect(failed, "does not exist", "hook with a missing working_dir")
    ui.success("Hooks: detailed exit code is not a failure, command matching, unit in announce lines, missing directory named")


CONSUMER_UNIT = """
terraform {
  source = "../../../modules/unit"

  before_hook "pinned" {
    commands = ["plan"]
    execute  = ["tofu", "version"]
  }
}

dependency "producer" {
  config_path = "../producer"
}

inputs = {
  tag    = dependency.producer.outputs.value
  suffix = split("\\n", run_cmd("--terragrunt-quiet", "tofu", "version"))[0]
}
"""


def pinned_invoke(root, arguments, path):
    # The ambient PATH holds no OpenTofu, so only the pinned copy can satisfy a command.
    return atmos.run(
        arguments,
        working_directory=root,
        env={"PATH": path, "NO_COLOR": "1", "ATMOS_TELEMETRY_ENABLED": "false", "TF_IN_AUTOMATION": "true"},
        output="capture",
        check=False,
    )


ORDINARY_STACK = """
dependencies:
  tools:
    opentofu: "9.9.8"
components:
  terraform:
    ordinary:
      command: tofu
"""


def install_probe(tools, version, real, suffix):
    directory = tools + "/bin/opentofu/opentofu/" + version
    fs.mkdir(directory, parents=True)
    exec.run(["go", "build", "-o", directory + "/tofu" + suffix, "./fixtures/toolprobe"], working_directory=fs.resolve("."), output="capture")
    fs.write_file(directory + "/real-tool.txt", real)
    return directory


def pinned_tools():
    real = engine()
    if "tofu" not in real.replace("\\", "/").rsplit("/", 1)[-1]:
        ui.warning("Pinned tool scenario needs OpenTofu; skipped")
        return
    work = fixtures.workspace("atmos-terragrunt-pinned-")
    suffix = exec.run(["go", "env", "GOEXE"], output="capture").stdout.strip()
    tools = work + "/tools"
    pinned = install_probe(tools, "9.9.9", real, suffix)
    stack_pinned = install_probe(tools, "9.9.8", real, suffix)

    # The project pins OpenTofu in .tool-versions, the baseline for units (which have no stack
    # manifest); nothing is downloaded because the pinned copy is already installed.
    config = fixtures.configuration(fs.resolve("."), binary="tofu")
    config["toolchain"] = {"install": "never", "install_path": tools}
    config["stacks"] = {"base_path": "stacks", "included_paths": ["**/*.yaml"]}
    root = work + "/project"
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, ".tool-versions", "opentofu 9.9.9\n")
    fixtures.write(root, "modules/unit/main.tf", lifecycle.UNIT_MODULE)
    fixtures.write(root, "components/terraform/ordinary/main.tf", lifecycle.UNIT_MODULE)
    fixtures.write(root, "stacks/ordinary.yaml", ORDINARY_STACK)
    lifecycle.write_unit(root, "producer", lifecycle.PLAN_UNIT.replace("@GREETING@", "from-producer"))
    lifecycle.write_unit(root, "consumer", CONSUMER_UNIT)
    ambient = work + "/empty-path"
    fs.mkdir(ambient, parents=True)

    applied = pinned_invoke(root, ["terraform", "apply", "producer", "-s", STACK, "--", "-auto-approve", "-no-color"], ambient)
    fixtures.check(applied.exit_code == 0, "apply with a pinned tool failed:\n" + output_of(applied))
    planned = pinned_invoke(root, ["terraform", "plan", "consumer", "-s", STACK, "--", "-no-color"], ambient)
    fixtures.check(planned.exit_code == 0, "plan with a pinned tool failed:\n" + output_of(planned))
    expect(planned, "Executing before hook: pinned", "plan of the consumer")

    # Every consumer of the tool went through the pinned copy: the Terraform commands, the
    # dependency output read, run_cmd in HCL, and the hook.
    log = fs.read_file(pinned + "/invocations.log").splitlines()
    commands = [line.split(" ")[0] for line in log]
    for command in ["init", "apply", "plan", "output"]:
        fixtures.check(command in commands, "The pinned tool never ran '" + command + "': " + str(log))
    versions = len([command for command in commands if command == "version"])
    fixtures.check(versions >= 2, "run_cmd and the hook should both run the pinned tool: " + str(log))

    # An ordinary component takes its pin from dependencies.tools in the stack manifest, which wins
    # over the project's .tool-versions. A top-level dependencies key in atmos.yaml is not read.
    ordinary = pinned_invoke(root, ["terraform", "plan", "ordinary", "-s", "ordinary", "--", "-no-color"], ambient)
    fixtures.check(ordinary.exit_code == 0, "plan of an ordinary component with a stack pin failed:\n" + output_of(ordinary))
    stack_log = fs.read_file(stack_pinned + "/invocations.log").splitlines()
    fixtures.check("plan" in [line.split(" ")[0] for line in stack_log], "The stack manifest pin was not used: " + str(stack_log))
    ui.success("Pinned tools: terraform commands, dependency output reads, run_cmd, and hooks use the pinned copy without an ambient PATH entry")
