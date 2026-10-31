# Opt-in CLI acceptance tests for the Terragrunt settings that choose how a unit runs: the
# `terraform_binary` executable, the `errors` block, the `exclude` block, `feature` flags, and the
# `read-config` / `init-from-module` hook stages with the built-in `tflint` hook. Every fixture is
# generated here, uses only built-in terraform_data resources and real OpenTofu or Terraform with
# local state, and never touches a cloud. Importing this module has no effects.
fixtures = module.load("fixtures.star")
lifecycle = module.load("lifecycle_extended.star")
hardening = module.load("hardening.star")

STACK = lifecycle.STACK

ALT_MODULE = lifecycle.UNIT_MODULE.replace('input = "${var.greeting}|${var.suffix}|${var.tag}|${var.envvar}"', 'input = "alt-module"')


def engine():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    return binary


def is_opentofu(binary):
    return "tofu" in binary.replace("\\", "/").rsplit("/", 1)[-1]


def new_project(prefix, binary=None):
    root = lifecycle.project(fixtures.workspace(prefix), "project", fs.resolve("."), binary or engine())
    fixtures.write(root, "modules/alt/main.tf", ALT_MODULE)
    return root


def output_of(result):
    return result.stdout + result.stderr


def expect(result, text, context):
    fixtures.check(text in output_of(result), "Expected '" + text + "' in " + context + ":\n" + output_of(result))


def expect_absent(result, text, context):
    fixtures.check(text not in output_of(result), "Did not expect '" + text + "' in " + context + ":\n" + output_of(result))


def invoke(root, arguments, env={}, path=None):
    # Extra environment variables reach Atmos exactly like a shell would pass them.
    variables = {"NO_COLOR": "1", "ATMOS_TELEMETRY_ENABLED": "false", "TF_IN_AUTOMATION": "true"}
    variables.update(env)
    if path != None:
        variables["PATH"] = path
    return atmos.run(arguments, working_directory=root, env=variables, output="capture", check=False)


def run(root, arguments, env={}, path=None, failing=False):
    result = invoke(root, arguments, env, path)
    if failing:
        fixtures.check(result.exit_code != 0, " ".join(arguments) + " unexpectedly succeeded:\n" + output_of(result))
    else:
        fixtures.check(result.exit_code == 0, " ".join(arguments) + ":\n" + output_of(result))
    return result


def terraform(root, operation, unit, arguments=[], flags=[], env={}, path=None, failing=False, stack=STACK):
    command = ["terraform", operation, unit, "-s", stack] + flags + ["--"] + arguments + ["-no-color"]
    return run(root, command, env, path, failing)


def configure(root, binary, features=None, extra=None):
    config = fixtures.configuration(fs.resolve("."), binary=binary)
    if features != None:
        config["components"]["terraform"]["terragrunt"]["features"] = features
    if extra != None:
        config.update(extra)
    fixtures.write(root, "atmos.yaml", json.encode(config))


def value_of(root, unit):
    return lifecycle.state_output(root, unit)


# ---------------------------------------------------------------------------------------------
# Feature flags
# ---------------------------------------------------------------------------------------------

FEATURE_UNIT = """
terraform {
  source = feature.alt.value ? "../../../modules/alt" : "../../../modules/unit"
}

feature "alt" {
  default = false
}
feature "wide" {
  default = false
}
feature "size" {
  default = 2
}
feature "tier" {
  default = "basic"
}

inputs = {
  greeting = feature.tier.value
  suffix   = tostring(feature.size.value)
  tag      = feature.wide.value ? "wide" : "narrow"
}
"""

FEATURE_DEPENDENCY_UNIT = """
terraform {
  source = "../../../modules/unit"
}

feature "link" {
  default = false
}

dependency "base" {
  config_path  = "../base"
  skip_outputs = !feature.link.value
  mock_outputs = { value = "no-link" }
}

inputs = {
  tag = dependency.base.outputs.value
}
"""


def features():
    binary = engine()
    root = new_project("atmos-terragrunt-features-", binary)
    lifecycle.write_unit(root, "featured", FEATURE_UNIT)
    lifecycle.write_unit(root, "base", lifecycle.PLAN_UNIT.replace("@GREETING@", "base-value"))
    lifecycle.write_unit(root, "linked", FEATURE_DEPENDENCY_UNIT)

    # Declared defaults apply when nothing overrides them: a string, a number, and a boolean.
    configure(root, binary)
    terraform(root, "apply", "featured", ["-auto-approve"])
    fixtures.check(value_of(root, "featured") == "basic|2|narrow|", "Defaults were not used: " + value_of(root, "featured"))

    # components.terraform.terragrunt.features overrides defaults, typed from the declaration.
    configure(root, binary, features={"wide": True, "size": 5, "tier": "gold"})
    terraform(root, "apply", "featured", ["-auto-approve"])
    fixtures.check(value_of(root, "featured") == "gold|5|wide|", "Configured features were not used: " + value_of(root, "featured"))

    # TG_FEATURE outranks the configuration, per name; other configured names stay.
    env = {"TG_FEATURE": "tier=premium,wide=false"}
    terraform(root, "apply", "featured", ["-auto-approve"], env=env)
    fixtures.check(value_of(root, "featured") == "premium|5|narrow|", "TG_FEATURE did not take precedence: " + value_of(root, "featured"))

    # A feature can choose the module source...
    configure(root, binary, features={"alt": True})
    terraform(root, "apply", "featured", ["-auto-approve"])
    fixtures.check(value_of(root, "featured") == "alt-module", "The source feature was ignored: " + value_of(root, "featured"))

    # ...and a dependency: off, the mock output is used and the dependency is not required.
    configure(root, binary)
    terraform(root, "apply", "linked", ["-auto-approve"])
    fixtures.check("no-link" in value_of(root, "linked"), "The disabled dependency was read: " + value_of(root, "linked"))
    terraform(root, "apply", "base", ["-auto-approve"])
    configure(root, binary, features={"link": True})
    terraform(root, "apply", "linked", ["-auto-approve"])
    fixtures.check("base-value" in value_of(root, "linked"), "The enabled dependency output was not used: " + value_of(root, "linked"))


    # A bad boolean override names the feature and the way to fix it.
    bad = terraform(root, "plan", "featured", env={"TG_FEATURE": "wide=sometimes"}, failing=True)
    expect(bad, "wide", "boolean feature with a non boolean override")
    expect(bad, "true", "boolean feature with a non boolean override")
    malformed = terraform(root, "plan", "featured", env={"TG_FEATURE": "novalue"}, failing=True)
    expect(malformed, "TG_FEATURE", "malformed TG_FEATURE")

    # An override no unit declares is reported by name and does not fail the command.
    configure(root, binary, features={"wdie": True})
    typo = run(root, ["list", "instances"])
    expect(typo, "wdie", "listing with an unknown feature override")
    ui.success("Feature flags: defaults, atmos.yaml and TG_FEATURE overrides, typed values, source and dependency toggles, clear errors")


# ---------------------------------------------------------------------------------------------
# terraform_binary
# ---------------------------------------------------------------------------------------------

def invocations(directory):
    log = directory + "/invocations.log"
    return [line.split(" ")[0] for line in fs.read_file(log).splitlines()] if fs.exists(log) else []


def terraform_binary():
    real = engine()
    if not is_opentofu(real):
        ui.warning("terraform_binary scenario installs OpenTofu probes; skipped without OpenTofu")
        return
    work = fixtures.workspace("atmos-terragrunt-binary-")
    suffix = exec.run(["go", "env", "GOEXE"], output="capture").stdout.strip()
    tools = work + "/tools"
    pinned = hardening.install_probe(tools, "9.9.9", real, suffix)
    absolute = hardening.install_probe(work + "/absolute-tools", "1.0.0", real, suffix)
    root = work + "/project"
    config = fixtures.configuration(fs.resolve("."), binary=real)
    config["toolchain"] = {"install": "never", "install_path": tools}
    config["stacks"] = {"base_path": "stacks", "included_paths": ["*.yaml"]}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, ".tool-versions", "opentofu 9.9.9\n")
    fixtures.write(root, "modules/unit/main.tf", lifecycle.UNIT_MODULE)
    ambient = work + "/empty-path"
    fs.mkdir(ambient, parents=True)

    def clear(directory):
        if fs.exists(directory + "/invocations.log"):
            fs.remove(directory + "/invocations.log")

    plain = lifecycle.PLAN_UNIT.replace("@GREETING@", "plain")
    named = 'terraform_binary = "tofu"\n' + lifecycle.PLAN_UNIT.replace("@GREETING@", "named")
    pathed = 'terraform_binary = "' + lifecycle.hcl_path(absolute) + '/tofu' + suffix + '"\n' + lifecycle.PLAN_UNIT.replace("@GREETING@", "pathed")
    consumer = 'terraform_binary = "tofu"\ndependency "named" {\n  config_path = "../named"\n}\ninputs = {\n  tag = dependency.named.outputs.value\n}\n' + lifecycle.PLAN_UNIT.replace("@GREETING@", "consumer")
    lifecycle.write_unit(root, "plain", plain)
    lifecycle.write_unit(root, "named", named)
    lifecycle.write_unit(root, "pathed", pathed)
    lifecycle.write_unit(root, "consumer", consumer)

    # The real executable is the default; a unit that names an executable runs through the toolchain.
    terraform(root, "apply", "plain", ["-auto-approve"], path=ambient)
    fixtures.check(invocations(pinned) == [] and invocations(absolute) == [], "A unit without terraform_binary used a probe")
    terraform(root, "apply", "named", ["-auto-approve"], path=ambient)
    commands = invocations(pinned)
    for command in ["init", "apply"]:
        fixtures.check(command in commands, "The toolchain 'tofu' never ran '" + command + "' for terraform_binary: " + str(commands))
    fixtures.check(invocations(absolute) == [], "The wrong executable ran: " + str(invocations(absolute)))

    # An absolute path selects that exact executable.
    terraform(root, "apply", "pathed", ["-auto-approve"], path=ambient)
    fixtures.check("apply" in invocations(absolute), "The absolute terraform_binary never ran apply: " + str(invocations(absolute)))

    # Reading a dependency's outputs and the implicit init use the unit's executable.
    clear(pinned)
    terraform(root, "plan", "consumer", path=ambient)
    commands = invocations(pinned)
    for command in ["init", "output", "plan"]:
        fixtures.check(command in commands, "The consumer did not use the selected executable for '" + command + "': " + str(commands))

    # A stack's explicit `command` is the stronger override: the unit's executable is not used.
    fixtures.write(root, "stacks/override.yaml", "import:\n  - terragrunt://live/dev/named\ncomponents:\n  terraform:\n    named:\n      command: " + real + "\n")
    clear(pinned)
    terraform(root, "plan", "named", stack="override", path=ambient)
    fixtures.check(invocations(pinned) == [], "A stack command did not override terraform_binary: " + str(invocations(pinned)))

    # A missing executable names the component, the stack, and the executable.
    lifecycle.write_unit(root, "missing", 'terraform_binary = "nxtfengine"\n' + plain)
    missing = terraform(root, "plan", "missing", path=ambient, failing=True)
    for text in ["missing", STACK, "nxtfengine", "not found"]:
        expect(missing, text, "plan with a missing terraform_binary")

    # An executable that is not Terraform or OpenTofu is rejected before any command runs.
    other = exec.which("go")
    lifecycle.write_unit(root, "wrong", 'terraform_binary = "' + lifecycle.hcl_path(other) + '"\n' + plain)
    wrong = terraform(root, "plan", "wrong", failing=True)
    expect(wrong, "terraform_binary", "plan with a non engine executable")
    expect(wrong, "not a Terraform or OpenTofu executable", "plan with a non engine executable")
    ui.success("terraform_binary: toolchain name, absolute path, dependency reads and init, stack precedence, clear errors")


# ---------------------------------------------------------------------------------------------
# errors block
# ---------------------------------------------------------------------------------------------

# A resource whose provisioner fails the first time and succeeds the second, so a retry has
# something to recover from. The marker lives beside the module, outside Terraform state.
FLAKY_MODULE = """
variable "marker" {
  type    = string
  default = ""
}
resource "terraform_data" "flaky" {
  provisioner "local-exec" {
    command = "test -f '${var.marker}' || { touch '${var.marker}'; echo transient-failure >&2; exit 1; }"
  }
}
output "value" {
  value = terraform_data.flaky.id != "" ? "recovered" : ""
}
"""

FAILING_MODULE = """
resource "terraform_data" "broken" {
  provisioner "local-exec" {
    command = "echo known-problem >&2; exit 1"
  }
}
output "value" {
  value = "unreachable"
}
"""


def errors_unit(module, block, marker=None):
    inputs = ""
    if marker != None:
        inputs = 'inputs = { marker = "' + lifecycle.hcl_path(marker) + '" }\n'
    return 'terraform {\n  source = "../../../modules/' + module + '"\n}\n' + inputs + block


def errors_block():
    binary = engine()
    if exec.which("sh") == None:
        ui.warning("errors scenario needs a POSIX shell for its failing provisioners; skipped")
        return
    root = new_project("atmos-terragrunt-errors-", binary)
    fixtures.write(root, "modules/flaky/main.tf", FLAKY_MODULE)
    fixtures.write(root, "modules/failing/main.tf", FAILING_MODULE)
    marker = root + "/flaky.marker"

    retry = 'errors {\n  retry "transient" {\n    retryable_errors   = [".*transient-failure.*"]\n    max_attempts       = 3\n    sleep_interval_sec = 0\n  }\n}\n'
    lifecycle.write_unit(root, "retried", errors_unit("flaky", retry, marker))
    result = terraform(root, "apply", "retried", ["-auto-approve"])
    expect(result, "Retryable error matched policy transient", "apply with a retry policy")
    expect(result, "attempt 1 of 3", "apply with a retry policy")
    expect_absent(result, "attempt 2 of 3", "apply that recovered on the second attempt")
    fixtures.check(lifecycle.state_output(root, "retried") == "recovered", "The retried apply did not complete")

    # A failure the policy does not cover is not retried, and a policy that runs out of attempts says so.
    other = 'errors {\n  retry "other" {\n    retryable_errors   = ["something-else"]\n    max_attempts       = 3\n    sleep_interval_sec = 0\n  }\n}\n'
    lifecycle.write_unit(root, "unmatched", errors_unit("failing", other))
    unmatched = terraform(root, "apply", "unmatched", ["-auto-approve"], failing=True)
    expect_absent(unmatched, "Retryable error matched", "apply of an unmatched failure")
    always = 'errors {\n  retry "always" {\n    retryable_errors   = [".*known-problem.*"]\n    max_attempts       = 2\n    sleep_interval_sec = 0\n  }\n}\n'
    lifecycle.write_unit(root, "exhausted", errors_unit("failing", always))
    exhausted = terraform(root, "apply", "exhausted", ["-auto-approve"], failing=True)
    expect(exhausted, "attempt 1 of 2", "apply with a retry policy that never succeeds")
    expect(exhausted, "maximum retry attempts reached", "apply with a retry policy that never succeeds")
    lifecycle.expect_count(output_of(exhausted), "attempt 1 of 2", 1, "exhausted retry policy")

    # A negative pattern vetoes the policy.
    veto = 'errors {\n  retry "vetoed" {\n    retryable_errors   = [".*known-problem.*", "!.*known-problem.*"]\n    max_attempts       = 3\n    sleep_interval_sec = 0\n  }\n}\n'
    lifecycle.write_unit(root, "vetoed", errors_unit("failing", veto))
    vetoed = terraform(root, "apply", "vetoed", ["-auto-approve"], failing=True)
    expect_absent(vetoed, "Retryable error matched", "apply with a vetoed retry policy")

    # An ignore policy turns the failure into success, prints its message, and writes its signals.
    ignore = 'errors {\n  ignore "known" {\n    ignorable_errors = [".*known-problem.*"]\n    message          = "this failure is expected"\n    signals = {\n      alert = "none"\n      scope = "test"\n    }\n  }\n  retry "never" {\n    retryable_errors   = [".*"]\n    max_attempts       = 5\n    sleep_interval_sec = 0\n  }\n}\n'
    lifecycle.write_unit(root, "ignored", errors_unit("failing", ignore))
    ignored = terraform(root, "apply", "ignored", ["-auto-approve"])
    expect(ignored, "this failure is expected", "apply with an ignore policy")
    expect_absent(ignored, "Retryable error matched", "ignore wins over retry")
    signals = fs.glob(root + "/.workdir/terraform/" + STACK + "-ignored-*/error-signals.json")
    fixtures.check(len(signals) == 1, "Expected one signals file: " + str(signals))
    fixtures.check(json.decode(fs.read_file(signals[0])) == {"alert": "none", "scope": "test"}, "Unexpected signals: " + fs.read_file(signals[0]))

    # Invalid policies are rejected with the block named.
    invalid = 'errors {\n  retry "bad" {\n    retryable_errors   = ["x"]\n    max_attempts       = 0\n    sleep_interval_sec = 0\n  }\n}\n'
    lifecycle.write_unit(root, "invalid", errors_unit("failing", invalid))
    rejected = terraform(root, "plan", "invalid", failing=True)
    expect(rejected, "max_attempts", "plan with an invalid retry policy")
    ui.success("errors block: retry until success, max attempts, veto patterns, ignore with message and signals, validation")


# ---------------------------------------------------------------------------------------------
# exclude block
# ---------------------------------------------------------------------------------------------

def exclude_unit(block, dependency=None):
    text = 'terraform {\n  source = "../../../modules/unit"\n}\n' + block
    if dependency != None:
        text += 'dependency "' + dependency + '" {\n  config_path = "../' + dependency + '"\n}\ninputs = {\n  tag = dependency.' + dependency + '.outputs.value\n}\n'
    return text



def exclude_block():
    binary = engine()
    root = new_project("atmos-terragrunt-exclude-", binary)
    fixtures.write(root, "modules/broken/main.tf", 'resource "terraform_data" "broken" {\n  lifecycle {\n    precondition {\n      condition     = false\n      error_message = "this unit always fails"\n    }\n  }\n}\n')
    # core is app's dependency; app excludes its dependencies for apply, but can still be run by name.
    lifecycle.write_unit(root, "core", exclude_unit(""))
    lifecycle.write_unit(root, "app", exclude_unit('exclude {\n  if                   = true\n  actions              = ["apply"]\n  exclude_dependencies = true\n  no_run               = false\n}\n', "core"))
    # With no_run unset only an exact action prevents even a direct run.
    lifecycle.write_unit(root, "stopped", exclude_unit('exclude {\n  if      = true\n  actions = ["apply"]\n}\n'))
    # no_run = true also honors the "all" vocabulary.
    lifecycle.write_unit(root, "frozen", exclude_unit('exclude {\n  if      = true\n  actions = ["all"]\n  no_run  = true\n}\n'))
    # A false condition excludes nothing.
    lifecycle.write_unit(root, "off", exclude_unit('exclude {\n  if      = false\n  actions = ["all"]\n}\n'))

    listed = output_of(run(root, ["list", "instances"]))
    for unit in ["core", "app", "stopped", "frozen", "off"]:
        fixtures.check(unit in listed, "list instances must show every unit including " + unit + ":\n" + listed)

    # A bulk apply leaves out every unit whose exclude block lists apply (here also through
    # "all"), and app's dependency because app sets exclude_dependencies.
    bulk = run(root, ["terraform", "apply", "--all", "-s", STACK, "--", "-auto-approve", "-no-color"])
    for unit in ["app", "stopped", "core", "frozen"]:
        expect(bulk, "Skipped " + unit + " in " + STACK + ": excluded by the Terragrunt exclude block", "bulk apply")
        fixtures.check(not fs.exists(lifecycle.state_path(root, unit)), unit + " was applied by a bulk run that excludes it")
    expect_absent(bulk, "Skipped off", "bulk apply")
    fixtures.check(fs.exists(lifecycle.state_path(root, "off")), "A false exclude condition still excluded the unit")

    # Naming a unit runs it unless no_run prevents the command. app reads core's outputs.
    terraform(root, "apply", "core", ["-auto-approve"])
    terraform(root, "apply", "app", ["-auto-approve"])
    fixtures.check(fs.exists(lifecycle.state_path(root, "app")), "no_run = false must let an explicit apply run")
    stopped = terraform(root, "apply", "stopped", ["-auto-approve"])
    expect(stopped, "no_run", "explicit apply of a unit that prevents it")
    fixtures.check(not fs.exists(lifecycle.state_path(root, "stopped")), "A prevented apply still ran")
    frozen = terraform(root, "plan", "frozen")
    expect(frozen, "no_run", "explicit plan of a frozen unit")

    # Bulk commands the block does not list still include the unit.
    planned = run(root, ["terraform", "plan", "--all", "-s", STACK, "--", "-no-color"])
    expect_absent(planned, "Skipped stopped", "bulk plan")

    # Components skipped after a failure are named.
    lifecycle.write_unit(root, "broken", 'terraform {\n  source = "../../../modules/broken"\n}\n')
    lifecycle.write_unit(root, "after", exclude_unit("", "broken"))
    failed = run(root, ["terraform", "plan", "--all", "-s", STACK, "--", "-no-color"], failing=True)
    expect(failed, "skipped after an earlier failure", "bulk plan with a failing unit")
    expect(failed, "after in " + STACK, "bulk plan with a failing unit")
    ui.success("exclude block: bulk skip with named lines, dependencies, no_run semantics, explicit runs, unaffected listing")


# ---------------------------------------------------------------------------------------------
# read-config and init-from-module hooks, and the built-in tflint hook
# ---------------------------------------------------------------------------------------------

LIFECYCLE_UNIT = """
terraform {
  source = "../../../modules/unit"

  # The relative path only exists in the unit directory, which is where these hooks run.
  after_hook "read" {
    commands = ["terragrunt-read-config"]
    execute  = ["@TOOL@", "fmt", "-check", "marker.tf"]
  }
  after_hook "read-alias" {
    commands = ["read-config"]
    execute  = ["@TOOL@", "version"]
  }
  before_hook "never-runs" {
    commands = ["terragrunt-read-config"]
    execute  = ["@TOOL@", "version"]
  }
  before_hook "fetch-before" {
    commands = ["init-from-module"]
    execute  = ["@TOOL@", "fmt", "-check", "marker.tf"]
  }
  after_hook "fetch-after" {
    commands = ["init-from-module"]
    execute  = ["@TOOL@", "version"]
  }
  error_hook "fetch-error" {
    commands  = ["init-from-module"]
    execute   = ["@TOOL@", "version"]
    on_errors = [".*"]
  }
}
"""


def lifecycle_hooks():
    binary = engine()
    root = new_project("atmos-terragrunt-lifecycle-", binary)
    tool = lifecycle.tool_of(binary)
    fixtures.write(root, "modules/local/main.tf", lifecycle.versioned_module("v1"))
    fixtures.write(root, "live/dev/staged/marker.tf", 'variable "marker" {}\n')
    unit = LIFECYCLE_UNIT.replace("@TOOL@", tool).replace("../../../modules/unit", "../../../modules/local")
    lifecycle.write_unit(root, "staged", unit)

    # The first plan fetches the module: read-config runs, then the fetch hooks, then the plan.
    first = terraform(root, "plan", "staged")
    err = first.stderr
    lifecycle.expect_count(err, "Executing after hook: read (", 1, "first plan")
    lifecycle.expect_count(err, "Executing after hook: read-alias", 1, "first plan")
    lifecycle.expect_count(err, "Executing before hook: fetch-before", 1, "first plan")
    lifecycle.expect_count(err, "Executing after hook: fetch-after", 1, "first plan")
    lifecycle.expect_count(err, "Executing error hook", 0, "first plan")
    lifecycle.expect_order(err, ["Executing after hook: read", "Executing before hook: fetch-before", "Executing after hook: fetch-after"], "first plan")
    # A before hook that lists read-config can never run; the user is told once.
    expect(first, "never-runs", "plan of a unit with a read-config before hook")
    lifecycle.expect_count(err, "Executing before hook: never-runs", 0, "first plan")

    # An unchanged module is not fetched again, but read-config runs on every command.
    second = terraform(root, "plan", "staged")
    lifecycle.expect_count(second.stderr, "Executing after hook: read (", 1, "second plan")
    lifecycle.expect_count(second.stderr, "Executing before hook: fetch-before", 0, "second plan")
    lifecycle.expect_count(second.stderr, "Executing after hook: fetch-after", 0, "second plan")

    # An edited module is fetched again, so the fetch hooks run again.
    fixtures.write(root, "modules/local/main.tf", lifecycle.versioned_module("v2"))
    third = terraform(root, "plan", "staged")
    lifecycle.expect_count(third.stderr, "Executing before hook: fetch-before", 1, "plan after an edit")
    lifecycle.expect_count(third.stderr, "Executing after hook: fetch-after", 1, "plan after an edit")
    ui.success("Lifecycle hooks: read-config on every command in the unit directory, init-from-module only when the source is fetched")


def tflint_hook():
    binary = engine()
    echo = exec.which("echo")
    if echo == None:
        ui.warning("tflint scenario needs an echo executable to stand in for tflint; skipped")
        return
    work = fixtures.workspace("atmos-terragrunt-tflint-")
    suffix = exec.run(["go", "env", "GOEXE"], output="capture").stdout.strip()
    path_dir = work + "/path"
    fs.mkdir(path_dir, parents=True)
    exec.run(["go", "build", "-o", path_dir + "/tflint" + suffix, "./fixtures/toolprobe"], working_directory=fs.resolve("."), output="capture")
    fs.write_file(path_dir + "/real-tool.txt", echo)
    root = new_project("atmos-terragrunt-tflint-", binary)
    fixtures.write(root, ".tflint.hcl", "# test configuration\n")
    unit = 'terraform {\n  source = "../../../modules/unit"\n\n  before_hook "lint" {\n    commands = ["plan"]\n    execute  = ["tflint", "--terragrunt-external-tflint", "--minimum-failure-severity=error"]\n  }\n}\ninputs = { greeting = "hi" }\n'
    lifecycle.write_unit(root, "linted", unit)
    result = terraform(root, "plan", "linted", path=path_dir + ":/usr/bin:/bin")
    lifecycle.expect_count(result.stderr, "Executing before hook: lint", 2, "plan with a tflint hook")
    lines = fs.read_file(path_dir + "/invocations.log").splitlines()
    fixtures.check(len(lines) == 2, "tflint should run twice (init and lint): " + str(lines))
    fixtures.check(lines[0].startswith("--init --config ") and lines[0].endswith("--chdir ."), "Unexpected tflint init: " + lines[0])
    fixtures.check(".tflint.hcl" in lines[1] and "--var=greeting=hi" in lines[1] and "--minimum-failure-severity=error" in lines[1], "Unexpected tflint run: " + lines[1])
    fixtures.check("--terragrunt-external-tflint" not in lines[1], "The external flag must not reach tflint: " + lines[1])
    ui.success("tflint hook: runs tflint --init and tflint with the unit's configuration and inputs")


def main():
    features()
    terraform_binary()
    errors_block()
    exclude_block()
    lifecycle_hooks()
    tflint_hook()
