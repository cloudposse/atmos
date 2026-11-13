# Opt-in CLI acceptance tests for the IAM, state encryption, and registry source features of native
# Terragrunt execution. Every fixture is generated here and uses local state and built-in terraform_data
# resources. State encryption needs OpenTofu; the other scenarios also run with Terraform. Nothing reaches a cloud: the IAM scenarios end at an unreachable STS endpoint,
# and the successful STS exchange is covered by Go tests against an STS test server.
#
# Importing this module has no effects; each integration step calls one scenario function.
fixtures = module.load("fixtures.star")
lifecycle = module.load("lifecycle_extended.star")

STACK = lifecycle.STACK
PASSPHRASE = "correct-horse-battery-staple"
ROLE = "arn:aws:iam::123456789012:role/terragrunt-deploy"
WEB_TOKEN = "web-identity-token-must-stay-secret"
UNREACHABLE_STS = "http://127.0.0.1:1"

MODULE_UNIT = """
terraform {
  source = "../../../modules/unit"
}
"""


def tofu():
    binary = exec.which("tofu")
    fixtures.check(binary != None, "Install OpenTofu for integration tests")
    return binary


def engine():
    binary = exec.which("tofu") or exec.which("terraform")
    fixtures.check(binary != None, "Install OpenTofu or Terraform for integration tests")
    return binary


def new_project(prefix, auth=False, binary=None):
    binary = binary or engine()
    work = fixtures.workspace(prefix)
    root = work + "/project"
    config = fixtures.configuration(fs.resolve("."), binary=binary)
    if auth:
        config["auth"] = {
            "providers": {"mock-provider": {"kind": "mock/aws"}},
            "identities": {
                "mock-identity": {"kind": "mock/aws", "default": True, "via": {"provider": "mock-provider"}, "principal": {"account": {"id": "123456789012"}}},
            },
        }
    fixtures.write(root, "atmos.yaml", json.encode(config))
    fixtures.write(root, "modules/unit/main.tf", lifecycle.UNIT_MODULE)
    return root, work


def write_hcl(root, name, content):
    fixtures.write(root, "live/dev/" + name + "/terragrunt.hcl", content)


def invoke(root, work, arguments, extra={}):
    env = {
        "NO_COLOR": "1",
        "ATMOS_TELEMETRY_ENABLED": "false",
        "TF_IN_AUTOMATION": "true",
        "ATMOS_KEYRING_TYPE": "memory",
        "XDG_CONFIG_HOME": work + "/xdg",
        "AWS_EC2_METADATA_DISABLED": "true",
    }
    env.update(extra)
    return atmos.run(arguments, working_directory=root, env=env, output="capture", check=False)


def output_of(result):
    return result.stdout + result.stderr


def terraform(root, work, operation, unit, extra={}, flags=[], failing=False):
    arguments = ["terraform", operation, unit, "-s", STACK] + flags + ["--"] + (["-auto-approve"] if operation == "apply" else []) + ["-no-color"]
    result = invoke(root, work, arguments, extra)
    if failing:
        fixtures.check(result.exit_code != 0, " ".join(arguments) + " unexpectedly succeeded:\n" + output_of(result))
    else:
        fixtures.check(result.exit_code == 0, " ".join(arguments) + ":\n" + output_of(result))
    return result


def expect(result, texts, context):
    for text in texts:
        fixtures.check(text in output_of(result), "Expected '" + text + "' in " + context + ":\n" + output_of(result))


def forbid(result, texts, context):
    for text in texts:
        fixtures.check(text not in output_of(result), "Did not expect '" + text + "' in " + context + ":\n" + output_of(result))


ENCRYPTED_BACKEND = """
remote_state {
  backend  = "local"
  generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }
  config   = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
  encryption = {
    key_provider = "pbkdf2"
    passphrase   = "@PASSPHRASE@"
  }
}
"""


def state_encryption():
    if exec.which("tofu") == None:
        ui.warning("State encryption scenario needs OpenTofu; skipped")
        return
    root, work = new_project("atmos-terragrunt-encryption-", binary=tofu())
    write_hcl(root, "secure", MODULE_UNIT + ENCRYPTED_BACKEND.replace("@PASSPHRASE@", PASSPHRASE))
    applied = terraform(root, work, "apply", "secure")
    expect(applied, ["Apply complete! Resources: 1 added"], "apply with state encryption")
    forbid(applied, [PASSPHRASE], "apply with state encryption")

    # The state OpenTofu wrote is an encrypted envelope, not readable JSON state.
    state = fs.read_file(lifecycle.state_path(root, "secure"))
    fixtures.check("encrypted_data" in state, "The state is not encrypted:\n" + state)
    fixtures.check("terraform_data" not in state and "\"resources\"" not in state, "The state holds plaintext resources:\n" + state)

    # The key configuration is stable: a later plan decrypts the state and finds nothing to do.
    planned = terraform(root, work, "plan", "secure")
    expect(planned, ["No changes."], "plan after an encrypted apply")
    forbid(planned, [PASSPHRASE], "plan after an encrypted apply")

    # Encryption without generate has no effect in Terragrunt, and the state stays plain.
    write_hcl(root, "plain", MODULE_UNIT + """
remote_state {
  backend    = "local"
  config     = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
  encryption = { key_provider = "pbkdf2", passphrase = "@PASSPHRASE@" }
}
""".replace("@PASSPHRASE@", PASSPHRASE))
    ignored = terraform(root, work, "apply", "plain")
    expect(ignored, ["no effect without remote_state.generate"], "encryption without generate")
    forbid(ignored, [PASSPHRASE], "encryption without generate")
    fixtures.check("encrypted_data" not in fs.read_file(lifecycle.state_path(root, "plain")), "State was encrypted without generate")

    # A module that already ships the backend file keeps it under if_exists = "skip", so the generated
    # encryption block is never written. Atmos warns before running, and only then.
    fixtures.write(root, "modules/kept/main.tf", lifecycle.UNIT_MODULE)
    fixtures.write(root, "modules/kept/backend.tf", "terraform {\n  backend \"local\" {}\n}\n")
    write_hcl(root, "kept", """
terraform {
  source = "../../../modules/kept"
}
""" + ENCRYPTED_BACKEND.replace("overwrite_terragrunt", "skip").replace("@PASSPHRASE@", PASSPHRASE))
    kept = terraform(root, work, "plan", "kept")
    expect(kept, ["configures state encryption in the generated file", "The state will not be encrypted"], "skipped encrypted backend")
    forbid(kept, [PASSPHRASE], "skipped encrypted backend")
    forbid(planned, ["will not be encrypted"], "generated encrypted backend")
    ui.success("State encryption: pbkdf2 encrypts the OpenTofu state, plans decrypt it, and the passphrase stays out of output")


def disable_init():
    root, work = new_project("atmos-terragrunt-disableinit-")
    write_hcl(root, "no-bootstrap", MODULE_UNIT + """
remote_state {
  backend      = "local"
  disable_init = true
  config       = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
}
""")
    # Atmos never creates backend resources, and OpenTofu initializes the backend as usual.
    applied = terraform(root, work, "apply", "no-bootstrap")
    expect(applied, ["Apply complete! Resources: 1 added"], "apply with disable_init")
    fixtures.check(fs.exists(lifecycle.state_path(root, "no-bootstrap")), "disable_init prevented the backend from being used")
    ui.success("disable_init: accepted, and the backend initializes and stores state normally")


def registry_sources():
    root, work = new_project("atmos-terragrunt-registry-")
    unit = """
terraform {
  source = "tfr://127.0.0.1:1/acme/vpc/aws//modules/nat?version=1.0.0"
}
"""
    write_hcl(root, "registry", unit)
    write_hcl(root, "plain", MODULE_UNIT)

    # Discovery never contacts the registry: the unit is listed and the plain unit still plans.
    instances = invoke(root, work, ["list", "instances", "--format=json"])
    fixtures.check(instances.exit_code == 0, output_of(instances))
    expect(instances, ["registry", "plain"], "list instances with a registry source")
    terraform(root, work, "plan", "plain")

    # Preparing the unit resolves the source through the registry protocol; an unreachable registry
    # is reported against the setting, with the credentials it would use.
    failed = terraform(root, work, "plan", "registry", failing=True)
    expect(failed, ["could not resolve the tfr:// source", "terraform.source", "TF_TOKEN_<host>"], "unreachable registry")
    forbid(failed, ["unsupported plugin extension", "Unsupported module source transport"], "unreachable registry")
    ui.success("Registry sources: discovery is offline, preparation resolves through the registry and reports failures against terraform.source")


IAM_UNIT = """
terraform {
  source = "../../../modules/unit"
}
iam_role                     = "@ROLE@"
iam_assume_role_session_name = "atmos-test"
iam_assume_role_duration     = 1800
iam_web_identity_token       = "@TOKEN@"
"""


def iam_role():
    root, work = new_project("atmos-terragrunt-iam-", auth=True)
    write_hcl(root, "assumed", IAM_UNIT.replace("@ROLE@", ROLE).replace("@TOKEN@", WEB_TOKEN))
    sts = {"AWS_ENDPOINT_URL_STS": UNREACHABLE_STS}

    # The settings are accepted at discovery: nothing is rejected before a command runs.
    instances = invoke(root, work, ["list", "instances", "--format=json"], sts)
    fixtures.check(instances.exit_code == 0, output_of(instances))
    expect(instances, ["assumed"], "list instances with iam_role")

    # With no identity selected, Atmos assumes the role itself, in process. The STS endpoint is
    # unreachable, so the failure names the role and the way out, and never prints the token.
    failed = terraform(root, work, "plan", "assumed", sts, failing=True)
    expect(failed, [ROLE, "--identity"], "iam_role assumption failure")
    forbid(failed, [WEB_TOKEN, "unsupported plugin extension", "Unsupported Terragrunt setting"], "iam_role assumption failure")

    # An Atmos identity selected explicitly decides who the component runs as: iam_role is not
    # assumed, no STS call is made, and the command succeeds even with STS unreachable.
    selected = terraform(root, work, "plan", "assumed", sts, flags=["--identity=mock-identity"])
    expect(selected, ["runs as Atmos identity mock-identity", ROLE], "explicit identity")
    forbid(selected, [WEB_TOKEN], "explicit identity")
    ui.success("iam_role: accepted at discovery, assumed in process otherwise, replaced by an explicit identity, token never printed")
