# Opt-in CLI acceptance tests for the typed provider flags and for executable mode.
#
# Provider flags: the options the package declares in plugin.yaml appear on `atmos terraform` commands
# and in help, follow command line > environment > configuration > declared default, are validated
# before anything runs, and are part of a saved plan's identity.
#
# Executable mode (`components.terraform.terragrunt.mode: executable`): Atmos discovers the units,
# orders them, authenticates, resolves the toolchain, and then runs one pinned `terragrunt run` command
# per unit instead of OpenTofu. Two kinds of executable are used:
#   * a stand-in built from fixtures/toolprobe that records how Atmos invoked it and then runs the real
#     OpenTofu in the unit directory. It proves the invocation (arguments, directory, environment,
#     order); it does not prove Terragrunt behavior.
#   * the real Terragrunt, installed by the Atmos toolchain, in one scenario. It needs network access and is
#     skipped with a warning when the install fails.
# Every fixture uses only built-in terraform_data resources and local state. Importing this module has no effects.
fixtures = module.load("fixtures.star")
lifecycle = module.load("lifecycle_extended.star")
hardening = module.load("hardening.star")
semantics = module.load("semantics.star")

STACK = lifecycle.STACK
TERRAGRUNT_VERSION = "1.2.1"


def contains(result, text):
    return text in semantics.output_of(result)


def expect(result, text, context):
    semantics.expect(result, text, context)


def expect_absent(result, text, context):
    semantics.expect_absent(result, text, context)


# ---------------------------------------------------------------------------------------------
# Provider flags
# ---------------------------------------------------------------------------------------------

def help_lists_provider_flags():
    binary = semantics.engine()
    root = semantics.new_project("atmos-terragrunt-flags-help-", binary)
    semantics.configure(root, binary)
    plan = semantics.run(root, ["terraform", "plan", "--help"])
    for text in ["TERRAGRUNT FLAGS", "--terragrunt-feature", "--terragrunt-iam-assume-role", "--terragrunt-iam-assume-role-duration", "--terragrunt-download-dir"]:
        expect(plan, text, "terraform plan --help")
    # Commands that run no unit do not get the flags.
    backend = semantics.run(root, ["terraform", "backend", "--help"])
    expect_absent(backend, "--terragrunt-feature", "terraform backend --help")

    # Without the provider enabled, nothing is registered.
    plain = semantics.new_project("atmos-terragrunt-flags-off-", binary)
    fixtures.write(plain, "atmos.yaml", json.encode({"base_path": ".", "components": {"terraform": {"command": binary}}}))
    expect_absent(semantics.run(plain, ["terraform", "plan", "--help"]), "TERRAGRUNT FLAGS", "terraform plan --help without the provider")
    ui.success("Provider flags: declared options appear in help under a provider section, only where they apply")


def feature_flags():
    binary = semantics.engine()
    root = semantics.new_project("atmos-terragrunt-flags-feature-", binary)
    lifecycle.write_unit(root, "featured", semantics.FEATURE_UNIT)

    # A declared default applies when nothing sets the feature.
    semantics.configure(root, binary)
    semantics.terraform(root, "apply", "featured", ["-auto-approve"])
    fixtures.check(semantics.value_of(root, "featured") == "basic|2|narrow|", "Default was not used: " + semantics.value_of(root, "featured"))

    # The flag outranks the environment, and the environment outranks the configuration, per name.
    semantics.configure(root, binary, features={"tier": "config", "size": 7})
    semantics.terraform(root, "apply", "featured", ["-auto-approve"])
    fixtures.check(semantics.value_of(root, "featured") == "config|7|narrow|", "Configuration was not used: " + semantics.value_of(root, "featured"))
    semantics.terraform(root, "apply", "featured", ["-auto-approve"], env={"TG_FEATURE": "tier=environment"})
    fixtures.check(semantics.value_of(root, "featured") == "environment|7|narrow|", "TG_FEATURE did not beat the configuration: " + semantics.value_of(root, "featured"))
    semantics.terraform(root, "apply", "featured", ["-auto-approve"], env={"ATMOS_TERRAGRUNT_FEATURE": "tier=atmos-environment"})
    fixtures.check(semantics.value_of(root, "featured") == "atmos-environment|7|narrow|", "ATMOS_TERRAGRUNT_FEATURE was ignored: " + semantics.value_of(root, "featured"))
    semantics.terraform(root, "apply", "featured", ["-auto-approve"], flags=["--terragrunt-feature", "tier=cli", "--terragrunt-feature=wide=true"], env={"TG_FEATURE": "tier=environment"})
    fixtures.check(semantics.value_of(root, "featured") == "cli|7|wide|", "The flag did not beat the environment: " + semantics.value_of(root, "featured"))

    # An explicit false is a value, not an absent flag.
    semantics.terraform(root, "apply", "featured", ["-auto-approve"], flags=["--terragrunt-feature=wide=false"], env={"TG_FEATURE": "wide=true"})
    fixtures.check(semantics.value_of(root, "featured") == "config|7|narrow|", "An explicit false was dropped: " + semantics.value_of(root, "featured"))

    # A malformed value is rejected before any discovery or infrastructure effect.
    fresh = "fresh"
    lifecycle.write_unit(root, fresh, semantics.FEATURE_UNIT)
    bad = semantics.terraform(root, "apply", fresh, ["-auto-approve"], flags=["--terragrunt-feature", "novalue"], failing=True)
    expect(bad, "name=value", "malformed --terragrunt-feature")
    fixtures.check(not fs.exists(lifecycle.state_path(root, fresh)), "A rejected flag still changed state")
    ui.success("Provider flags: feature flag precedence (flag > environment > configuration > default), explicit false, validation before effects")


def saved_plan_identity():
    binary = semantics.engine()
    root = semantics.new_project("atmos-terragrunt-flags-plan-", binary)
    lifecycle.write_unit(root, "featured", semantics.FEATURE_UNIT)
    semantics.configure(root, binary)

    # The effective options are part of what a saved plan is bound to.
    semantics.terraform(root, "plan", "featured", flags=["--terragrunt-feature", "tier=reviewed"])
    other = semantics.terraform(root, "apply", "featured", ["-auto-approve"], flags=["--terragrunt-feature", "tier=different", "--from-plan"], failing=True)
    fixtures.check("plan verification failed" in semantics.output_of(other), "A plan reviewed with other options was accepted:\n" + semantics.output_of(other))
    fixtures.check(not fs.exists(lifecycle.state_path(root, "featured")), "A rejected plan created state")
    absent = semantics.terraform(root, "apply", "featured", ["-auto-approve"], flags=["--from-plan"], failing=True)
    fixtures.check("plan verification failed" in semantics.output_of(absent), "A plan reviewed with options was applied without them:\n" + semantics.output_of(absent))

    # The same options apply exactly what was reviewed.
    semantics.terraform(root, "plan", "featured", flags=["--terragrunt-feature", "tier=reviewed"])
    applied = semantics.terraform(root, "apply", "featured", ["-auto-approve"], flags=["--terragrunt-feature", "tier=reviewed", "--from-plan"])
    fixtures.check("Apply complete! Resources: 1 added" in applied.stdout, applied.stdout)
    fixtures.check(semantics.value_of(root, "featured") == "reviewed|2|narrow|", "The applied value differs from the reviewed plan: " + semantics.value_of(root, "featured"))
    ui.success("Provider flags: effective options are part of the saved-plan identity")


# ---------------------------------------------------------------------------------------------
# Executable mode
# ---------------------------------------------------------------------------------------------

UNIT_MODULE = """
resource "terraform_data" "unit" {
  input = "@NAME@"
}
output "value" {
  value = terraform_data.unit.output
}
"""

BACKEND_FILE = """
terraform {
  backend "local" {
    path = "state/terraform.tfstate"
  }
}
"""

CONSUMER_STACK = """
components:
  terraform:
    consumer:
      vars:
        from_output: !terraform.output producer live/dev value
        from_state: !terraform.state producer live/dev value
"""

FAILING_MODULE = """
output "value" {
  value = tonumber("not-a-number")
}
"""


def install_terragrunt_stand_in(tools, version, real, suffix, reported=None):
    directory = tools + "/bin/gruntwork-io/terragrunt/" + version
    fs.mkdir(directory, parents=True)
    exec.run(["go", "build", "-o", directory + "/terragrunt" + suffix, "./fixtures/toolprobe"], working_directory=fs.resolve("."), output="capture")
    fs.write_file(directory + "/real-tool.txt", real)
    if reported != None:
        fs.write_file(directory + "/version.txt", reported)
    return directory


def write_stand_in_unit(root, name, content=None, module=None, dependencies=[]):
    # The stand-in runs OpenTofu in the unit directory itself, so the unit carries its own module and backend.
    unit = "terraform {\n  source = \".\"\n}\n"
    if dependencies:
        unit += "dependencies {\n  paths = [" + ", ".join(["\"../" + d + "\"" for d in dependencies]) + "]\n}\n"
    unit += lifecycle.BACKEND
    fixtures.write(root, "live/dev/" + name + "/terragrunt.hcl", content or unit)
    fixtures.write(root, "live/dev/" + name + "/main.tf", (module or UNIT_MODULE).replace("@NAME@", name))
    fixtures.write(root, "live/dev/" + name + "/backend.tf", BACKEND_FILE)


def executable_config(binary, tools=None, features=None):
    config = fixtures.configuration(fs.resolve("."), binary=binary)
    config["components"]["terraform"]["terragrunt"]["mode"] = "executable"
    if features != None:
        config["components"]["terraform"]["terragrunt"]["features"] = features
    config["stacks"] = {"base_path": "stacks", "included_paths": ["**/*.yaml"]}
    if tools != None:
        config["toolchain"] = {"install": "never", "install_path": tools}
    return config


def stand_in_project(prefix, version="9.9.9", reported=None):
    real = semantics.engine()
    work = fixtures.workspace(prefix)
    suffix = exec.run(["go", "env", "GOEXE"], output="capture").stdout.strip()
    tools = work + "/tools"
    stand_in = install_terragrunt_stand_in(tools, version, real, suffix, reported)
    root = work + "/project"
    fixtures.write(root, "atmos.yaml", json.encode(executable_config(real, tools)))
    fixtures.write(root, ".tool-versions", "terragrunt " + version + "\n")
    return struct(root=root, stand_in=stand_in, real=real, work=work, tools=tools, suffix=suffix)


def calls(project):
    log = project.stand_in + "/terragrunt.log"
    if not fs.exists(log):
        return []
    return [json.decode(line) for line in fs.read_file(log).splitlines() if line.strip() != ""]


def engine_calls(project):
    # Every recorded terragrunt command splits at `--` into Terragrunt arguments and the engine command.
    result = []
    for call in calls(project):
        arguments = call["args"]
        index = arguments.index("--")
        result.append(struct(before=arguments[:index], engine=arguments[index + 1:], dir=call["dir"], env=call["env"]))
    return result


def executable_command(project, operation, unit, arguments=[], flags=[], env={}, failing=False):
    return semantics.terraform(project.root, operation, unit, arguments, flags=flags, env=env, failing=failing)


def listed_engines(root):
    rows = json.decode(semantics.run(root, ["list", "instances", "--format=json"]).stdout)
    return {row["Component"]: row for row in rows}


def executable_invocation():
    project = stand_in_project("atmos-terragrunt-exec-")
    write_stand_in_unit(project.root, "app")

    # Units are listed with the engine that runs them.
    rows = listed_engines(project.root)
    fixtures.check(rows["app"]["Engine"] == "terragrunt", "list instances did not show the terragrunt engine: " + str(rows))
    fixtures.check(rows["app"]["Type"] == "terraform", "The component type changed: " + str(rows))

    # plan: one `terragrunt run` in the unit directory; the engine command and its arguments follow `--`.
    plan = executable_command(project, "plan", "app")
    expect(plan, "1 to add", "plan through terragrunt")
    recorded = engine_calls(project)
    fixtures.check(len(recorded) == 1, "Expected one terragrunt command, got " + str(recorded))
    first = recorded[0]
    unit_dir = project.root + "/live/dev/app"
    fixtures.check(first.engine == ["plan", "-no-color"], "Unexpected engine command: " + str(first.engine))
    fixtures.check(first.before[0] == "--working-dir" and first.before[1].endswith("live/dev/app") and first.before[2] == "run", "Unexpected terragrunt arguments: " + str(first.before))
    fixtures.check(first.env.get("TG_NON_INTERACTIVE") == "true", "Terragrunt was allowed to prompt: " + str(first.env))
    fixtures.check(first.env.get("TG_TF_FORWARD_STDOUT") == "true", "Engine output was not forwarded untouched: " + str(first.env))
    fixtures.check(first.env.get("TG_TF_PATH", "").endswith(semantics.engine().rsplit("/", 1)[-1]), "Terragrunt did not run the Atmos-selected engine: " + str(first.env))

    # Native machinery never ran: no workdir, no generated varfile, no backend file written by Atmos.
    fixtures.check(not fs.exists(project.root + "/.workdir"), "Native provisioning ran for an executable unit")
    fixtures.check(fs.glob(unit_dir + "/*.terraform.tfvars.json") == [], "A native varfile was generated")

    # apply with pass-through arguments; deploy is apply without approval.
    executable_command(project, "apply", "app", ["-auto-approve"])
    fixtures.check(lifecycle.state_output(project.root, "app") == "app", "The unit was not applied")
    fixtures.check(engine_calls(project)[-1].engine == ["apply", "-auto-approve", "-no-color"], "Unexpected apply command: " + str(engine_calls(project)[-1].engine))
    executable_command(project, "deploy", "app")
    fixtures.check(engine_calls(project)[-1].engine[:2] == ["apply", "-auto-approve"], "deploy did not apply without approval: " + str(engine_calls(project)[-1].engine))

    # output and compound subcommands are forwarded as written.
    out = semantics.terraform(project.root, "output", "app", ["-json"], flags=[])
    expect(out, "\"value\"", "output -json through terragrunt")
    fixtures.check(engine_calls(project)[-1].engine[0] == "output", "output was not forwarded")
    semantics.terraform(project.root, "validate", "app", flags=[])
    fixtures.check(engine_calls(project)[-1].engine[0] == "validate", "validate was not forwarded: " + str(engine_calls(project)[-1].engine))
    ui.success("Executable mode: engine listed as terragrunt; plan, apply, deploy, output and validate run one pinned terragrunt command per unit")


def executable_flags_and_config():
    project = stand_in_project("atmos-terragrunt-exec-flags-")
    write_stand_in_unit(project.root, "app")
    config = executable_config(project.real, project.tools, features={"from_config": True, "shared": "config"})
    fixtures.write(project.root, "atmos.yaml", json.encode(config))

    token = "tok-s3cret-value"
    result = executable_command(project, "plan", "app", flags=[
        "--terragrunt-feature", "shared=cli", "--terragrunt-feature=extra=1",
        "--terragrunt-iam-assume-role", "arn:aws:iam::123456789012:role/ci",
        "--terragrunt-iam-assume-role-duration=900", "--terragrunt-iam-assume-role-session-name", "atmos",
        "--terragrunt-download-dir", project.work + "/downloads",
        "--terragrunt-iam-assume-role-web-identity-token", token,
    ])
    recorded = engine_calls(project)[-1]
    before = " ".join(recorded.before)
    for text in ["--feature extra=1", "--feature from_config=true", "--feature shared=cli", "--iam-assume-role arn:aws:iam::123456789012:role/ci", "--iam-assume-role-duration 900", "--iam-assume-role-session-name atmos", "--download-dir " + project.work + "/downloads"]:
        fixtures.check(text in before, "Terragrunt was not given '" + text + "': " + before)
    fixtures.check("shared=config" not in before, "A configured feature overrode the flag: " + before)
    # The sensitive option travels through the environment, never through argv, and is never printed.
    fixtures.check(token not in before, "The sensitive option reached the arguments")
    fixtures.check(recorded.env.get("TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN") == token, "The sensitive option did not reach the environment")
    fixtures.check(token not in semantics.output_of(result), "The sensitive option was printed")

    # Absent options send nothing.
    executable_command(project, "plan", "app")
    quiet = " ".join(engine_calls(project)[-1].before)
    for text in ["--iam-assume-role", "--download-dir"]:
        fixtures.check(text not in quiet, "An absent option was sent: " + quiet)
    ui.success("Executable mode: provider flags and configured features become Terragrunt flags; secrets stay out of argv and output")


def executable_ordering():
    project = stand_in_project("atmos-terragrunt-exec-all-")
    write_stand_in_unit(project.root, "base")
    write_stand_in_unit(project.root, "app", dependencies=["base"])
    result = semantics.run(project.root, ["terraform", "apply", "--all", "-s", STACK, "--", "-auto-approve", "-no-color"])
    expect(result, "Apply complete", "apply --all")
    order = [c.dir.rsplit("/", 1)[-1] for c in engine_calls(project) if c.engine[0] == "apply"]
    fixtures.check(order == ["base", "app"], "Dependencies were not applied first: " + str(order))
    # Atmos schedules the units; Terragrunt never receives --all.
    for call in engine_calls(project):
        fixtures.check("--all" not in call.before and "--all" not in call.engine, "terragrunt run --all was used: " + str(call.before))
    semantics.run(project.root, ["terraform", "destroy", "--all", "-s", STACK, "--", "-auto-approve", "-no-color"])
    order = [c.dir.rsplit("/", 1)[-1] for c in engine_calls(project) if c.engine[0] == "destroy"]
    fixtures.check(order == ["app", "base"], "Destroy did not run in reverse dependency order: " + str(order))
    ui.success("Executable mode: Atmos schedules units in dependency order, one terragrunt command each, never terragrunt run --all")


def executable_consumers():
    project = stand_in_project("atmos-terragrunt-exec-consumer-")
    write_stand_in_unit(project.root, "producer")
    fixtures.write(project.root, "components/terraform/consumer/main.tf", lifecycle.CONSUMER_MODULE)
    fixtures.write(project.root, "stacks/consumers.yaml", CONSUMER_STACK)
    executable_command(project, "apply", "producer", ["-auto-approve"])
    # An ordinary component reads the executable unit's state and outputs through the discovered backend.
    values = lifecycle.described_vars(project.root, "consumer")
    fixtures.check(values["from_state"] == "producer" and values["from_output"] == "producer", "Consumers did not read the unit: " + str(values))
    ui.success("Executable mode: !terraform.state and !terraform.output read executable units through the discovered backend")


def executable_failures():
    project = stand_in_project("atmos-terragrunt-exec-fail-")
    write_stand_in_unit(project.root, "app")
    write_stand_in_unit(project.root, "broken", module=FAILING_MODULE)

    # A failing unit fails the command with its own error and a failing exit code.
    failed = semantics.invoke(project.root, ["terraform", "plan", "broken", "-s", STACK, "--", "-no-color"])
    fixtures.check(failed.exit_code != 0, "A failing unit reported success:\n" + semantics.output_of(failed))
    expect(failed, "tonumber", "failing unit output")

    # Unsupported commands and stack overrides are rejected before anything runs.
    before = len(calls(project))
    shell = semantics.invoke(project.root, ["terraform", "shell", "app", "-s", STACK])
    fixtures.check(shell.exit_code != 0, "shell was accepted in executable mode")
    expect(shell, "no Terragrunt equivalent", "terraform shell")
    saved = semantics.invoke(project.root, ["terraform", "apply", "app", "-s", STACK, "--from-plan"])
    fixtures.check(saved.exit_code != 0, "--from-plan was accepted in executable mode")
    expect(saved, "saved plans", "apply --from-plan")
    fixtures.write(project.root, "stacks/override.yaml", "import:\n  - terragrunt://live/dev/app\ncomponents:\n  terraform:\n    app:\n      vars:\n        region: us-east-1\n")
    overridden = semantics.invoke(project.root, ["terraform", "plan", "app", "-s", "override", "--", "-no-color"])
    fixtures.check(overridden.exit_code != 0, "Stack vars were accepted for an executable unit")
    expect(overridden, "stack vars", "stack vars override")
    fixtures.check(len(calls(project)) == before, "Rejected commands ran terragrunt")

    # Environment given to the unit's component reaches terragrunt.
    fixtures.write(project.root, "stacks/env.yaml", "import:\n  - terragrunt://live/dev/app\ncomponents:\n  terraform:\n    app:\n      env:\n        TG_TF_PATH: " + project.real + "\n        MARKER: from-stack\n")
    semantics.run(project.root, ["terraform", "plan", "app", "-s", "env", "--", "-no-color"])
    fixtures.check(engine_calls(project)[-1].env.get("TG_TF_PATH") == project.real, "A component env entry did not win over the default")
    ui.success("Executable mode: failing units fail the command; unsupported commands and stack overrides are rejected before anything runs")


def executable_requirements():
    # The executable is not pinned and not on PATH: a clear error that says how to pin it.
    project = stand_in_project("atmos-terragrunt-exec-missing-")
    write_stand_in_unit(project.root, "app")
    fs.remove(project.root + "/.tool-versions")
    missing = semantics.invoke(project.root, ["terraform", "plan", "app", "-s", STACK, "--", "-no-color"], path=project.work)
    fixtures.check(missing.exit_code != 0, "A missing terragrunt was accepted")
    expect(missing, "terragrunt executable not found", "missing terragrunt")
    expect(missing, ".tool-versions", "missing terragrunt")

    # A pinned version older than the minimum is rejected before the unit runs.
    old = stand_in_project("atmos-terragrunt-exec-old-", reported="v0.99.0")
    write_stand_in_unit(old.root, "app")
    rejected = semantics.invoke(old.root, ["terraform", "plan", "app", "-s", STACK, "--", "-no-color"])
    fixtures.check(rejected.exit_code != 0, "An old terragrunt was accepted")
    expect(rejected, "older than the minimum 1.0.0", "old terragrunt")
    fixtures.check(engine_calls(old) == [], "The unit ran with an unsupported terragrunt")
    ui.success("Executable mode: a missing or too old terragrunt is a clear error, and nothing runs")


# ---------------------------------------------------------------------------------------------
# Real Terragrunt
# ---------------------------------------------------------------------------------------------

REAL_UNIT = """
terraform {
  source = "../../../modules/unit"
}

feature "tier" {
  default = "basic"
}

inputs = {
  greeting = feature.tier.value
}
"""


def real_terragrunt():
    real = semantics.engine()
    work = fixtures.workspace("atmos-terragrunt-real-")
    root = work + "/project"
    tools = work + "/tools"
    config = executable_config(real, tools)
    config["toolchain"]["install"] = "auto"
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, ".tool-versions", "terragrunt " + TERRAGRUNT_VERSION + "\n")
    fixtures.write(root, "modules/unit/main.tf", lifecycle.UNIT_MODULE)
    fixtures.write(root, "components/terraform/consumer/main.tf", lifecycle.CONSUMER_MODULE)
    fixtures.write(root, "stacks/consumers.yaml", CONSUMER_STACK)
    lifecycle.write_unit(root, "producer", REAL_UNIT)

    # Installing the pinned executable needs network access; without it the scenario cannot run.
    installed = atmos.run(["toolchain", "install", "gruntwork-io/terragrunt@" + TERRAGRUNT_VERSION], working_directory=root, env={"ATMOS_TELEMETRY_ENABLED": "false", "NO_COLOR": "1"}, output="capture", check=False)
    if installed.exit_code != 0:
        ui.warning("Real Terragrunt could not be installed (no network?); skipped")
        return

    plan = semantics.terraform(root, "plan", "producer")
    expect(plan, "1 to add", "plan through the real terragrunt")
    # The real executable ran: Terragrunt creates its working copy of the module.
    fixtures.check(fs.glob(root + "/live/dev/producer/.terragrunt-cache/*") != [], "Terragrunt did not run in the unit")

    semantics.terraform(root, "apply", "producer", ["-auto-approve"])
    fixtures.check(lifecycle.state_output(root, "producer") == "basic|||", "The real terragrunt did not apply the unit: " + str(lifecycle.state_output(root, "producer")))

    # A feature flag reaches the real executable and changes the unit.
    semantics.terraform(root, "apply", "producer", ["-auto-approve"], flags=["--terragrunt-feature", "tier=premium"])
    fixtures.check(lifecycle.state_output(root, "producer") == "premium|||", "The feature flag did not reach terragrunt: " + str(lifecycle.state_output(root, "producer")))
    semantics.terraform(root, "apply", "producer", ["-auto-approve"], env={"TG_FEATURE": "tier=environment"})
    fixtures.check(lifecycle.state_output(root, "producer") == "environment|||", "TG_FEATURE did not reach terragrunt: " + str(lifecycle.state_output(root, "producer")))

    # Consumers read the unit written by the real executable.
    values = lifecycle.described_vars(root, "consumer")
    fixtures.check(values["from_state"] == "environment|||" and values["from_output"] == "environment|||", "Consumers did not read the unit: " + str(values))

    out = semantics.terraform(root, "output", "producer", ["-json"])
    expect(out, "environment", "output -json through the real terragrunt")
    semantics.terraform(root, "destroy", "producer", ["-auto-approve"])
    state = json.decode(fs.read_file(lifecycle.state_path(root, "producer")))
    fixtures.check(not state["resources"], "Destroy left resources")
    ui.success("Executable mode with the real Terragrunt " + TERRAGRUNT_VERSION + ": plan, apply, feature flags, outputs, consumers, destroy")
