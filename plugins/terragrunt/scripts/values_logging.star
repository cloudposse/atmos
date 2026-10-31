# Opt-in CLI acceptance tests for how native Terragrunt units exchange values and messages with Atmos:
# exact input values and sensitive inputs, Terragrunt's own log events shown through the Atmos logger,
# warnings shown once per command, and the AWS identity that functions such as get_aws_account_id()
# use. Every fixture is generated here, uses only built-in Terraform behavior with local state, and never
# touches a cloud: the AWS scenarios end at an unreachable STS endpoint, and the successful STS exchange
# is covered by Go tests against an STS test server.
#
# Importing this module has no effects; each integration step calls one scenario function.
fixtures = module.load("fixtures.star")
lifecycle = module.load("lifecycle_extended.star")
semantics = module.load("semantics.star")

STACK = lifecycle.STACK
SECRET = "not-a-real-password"
BIG = "9007199254740993"
UNREACHABLE_STS = "http://127.0.0.1:1"

VALUES_MODULE = """
variable "large" {
  type = number
}
variable "ratio" {
  type = number
}
variable "items" {
  type = any
}
variable "nothing" {
  type    = any
  default = "unset"
}
variable "object" {
  type = any
}
variable "password" {
  type      = string
  sensitive = true
}
output "large" {
  value = var.large
}
output "ratio" {
  value = var.ratio
}
output "items" {
  value = var.items
}
output "nothing" {
  value = var.nothing
}
output "object" {
  value = var.object
}
output "password_length" {
  value = nonsensitive(length(var.password))
}
# The variable file Atmos wrote beside the module, exactly as OpenTofu read it.
output "varfile" {
  value = join("", [for f in fileset(path.module, "atmos-*.terraform.tfvars.json") : file("${path.module}/${f}")])
}
"""

VALUES_UNIT = """
terraform {
  source = "../../../modules/values"
}

locals {
  password = sensitive("@SECRET@")
}

inputs = {
  large    = @BIG@
  ratio    = 0.1
  items    = ["a", 2, null]
  nothing  = null
  object   = { name = "x", count = 3 }
  password = local.password
}
"""

PLAIN_ENCRYPTION_UNIT = """
terraform {
  source = "../../../modules/values"
}

remote_state {
  backend    = "local"
  config     = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
  encryption = { key_provider = "pbkdf2", passphrase = "correct-horse-battery-staple" }
}

inputs = {
  large    = 1
  ratio    = 1
  items    = []
  object   = {}
  password = "x"
}
"""

AWS_UNIT = """
terraform {
  source = "../../../modules/values"
}

inputs = {
  large    = 1
  ratio    = 1
  items    = []
  object   = {}
  password = get_aws_account_id()
}
"""


def project(prefix):
    root = semantics.new_project(prefix)
    fixtures.write(root, "modules/values/main.tf", VALUES_MODULE)
    return root


def combined(result):
    return result.stdout + result.stderr


def values():
    root = project("atmos-terragrunt-values-")
    lifecycle.write_unit(root, "values", VALUES_UNIT.replace("@SECRET@", SECRET).replace("@BIG@", BIG))

    # Trace logging is the most any command can reveal: the secret must stay out of it, and the plan and apply
    # must still succeed, so the sensitive input reaches OpenTofu even though it never touches a variable file.
    planned = semantics.terraform(root, "plan", "values", flags=["--logs-level=Trace"])
    semantics.expect_absent(planned, SECRET, "trace plan")
    applied = semantics.terraform(root, "apply", "values", ["-auto-approve"], flags=["--logs-level=Trace"])
    semantics.expect_absent(applied, SECRET, "trace apply")

    # What Terraform recorded, read from the state file rather than from any Atmos function under test.
    state_text = fs.read_file(lifecycle.state_path(root, "values"))
    fixtures.check('"value": ' + BIG in state_text, "A number beyond float64 precision did not survive into the state:\n" + state_text)
    outputs = json.decode(state_text)["outputs"]
    fixtures.check(outputs["large"]["value"] == int(BIG), "Exact integer lost: " + str(outputs["large"]))
    fixtures.check(outputs["ratio"]["value"] == 0.1, "Decimal lost: " + str(outputs["ratio"]))
    fixtures.check(outputs["items"]["value"] == ["a", 2, None], "List or null changed: " + str(outputs["items"]))
    # A null input stays null instead of falling back to the variable default; Terraform does not record null outputs.
    fixtures.check("nothing" not in outputs or outputs["nothing"]["value"] == None, "A null input became a value: " + str(outputs.get("nothing")))
    fixtures.check(outputs["object"]["value"] == {"name": "x", "count": 3}, "Object changed: " + str(outputs["object"]))
    fixtures.check(outputs["password_length"]["value"] == len(SECRET), "The sensitive input did not reach OpenTofu: " + str(outputs["password_length"]))

    # The variable file Atmos wrote for OpenTofu keeps the exact number and leaves the secret out of the file,
    # exactly like a secret: it reached OpenTofu through the environment, as the password length above proves.
    written = outputs["varfile"]["value"]
    fixtures.check(BIG in written, "The variable file lost the exact number:\n" + written)
    fixtures.check(SECRET not in written, "A sensitive input was written to the variable file:\n" + written)
    fixtures.check('"password"' not in written, "A sensitive input was written to the variable file:\n" + written)

    # Description and listing never reveal the value.
    for command in (["describe", "component", "values", "-s", STACK], ["list", "instances", "--format=json"]):
        semantics.expect_absent(semantics.run(root, command), SECRET, " ".join(command))
    ui.success("Values: exact numbers, null, and shapes survive into OpenTofu; a sensitive input is masked, kept out of the variable file, and still applied")


def logging():
    root = project("atmos-terragrunt-logging-")
    lifecycle.write_unit(root, "values", VALUES_UNIT.replace("@SECRET@", SECRET).replace("@BIG@", BIG))

    # At the default level Terragrunt's own events stay hidden.
    quiet = semantics.terraform(root, "plan", "values")
    semantics.expect_absent(quiet, "plugin=terragrunt", "default log level")

    # At Debug they appear through the Atmos logger, scoped to the component and stack, on stderr only.
    for level in ("Debug", "Trace"):
        loud = semantics.terraform(root, "plan", "values", flags=["--logs-level=" + level])
        for text in ("plugin=terragrunt", "component=values", "stack=" + STACK):
            semantics.expect(loud, text, level + " log level")
        fixtures.check("plugin=terragrunt" not in loud.stdout, level + " put Terragrunt log events on the data channel:\n" + loud.stdout)
        semantics.expect_absent(loud, SECRET, level + " log level")

    # A failing unit keeps its error, its exit code, and now also the events that preceded the failure.
    lifecycle.write_unit(root, "broken", 'terraform {\n  source = "../../../modules/values"\n}\ninputs = { broken = local.absent }\n')
    failed = semantics.terraform(root, "plan", "broken", failing=True, flags=["--logs-level=Debug"])
    semantics.expect(failed, "plugin=terragrunt", "debug log of a failing unit")
    hidden = semantics.terraform(root, "plan", "broken", failing=True)
    semantics.expect_absent(hidden, "plugin=terragrunt", "default log level of a failing unit")
    fixtures.check(hidden.exit_code == failed.exit_code, "The log level changed the exit code")
    ui.success("Logging: Terragrunt events are hidden by default, shown scoped and masked at Debug and Trace, and never change the outcome")


def warnings_once():
    root = project("atmos-terragrunt-warnings-")
    fixtures.write(root, "live/dev/plain/terragrunt.hcl", PLAIN_ENCRYPTION_UNIT)

    # One command prepares the unit several times; the warning about it is shown once.
    applied = semantics.terraform(root, "apply", "plain", ["-auto-approve"])
    lifecycle.expect_count(combined(applied), "no effect without remote_state.generate", 1, "apply output")
    ui.success("Warnings: a warning about a unit is shown once per command")


def aws_identity():
    root = project("atmos-terragrunt-aws-")
    lifecycle.write_unit(root, "account", AWS_UNIT)
    work = fixtures.workspace("atmos-terragrunt-aws-files-")
    fixtures.write(work, "credentials", "[atmos-identity]\naws_access_key_id = AKIAINVOCATIONONLY\naws_secret_access_key = invocation-only-secret\n")
    fixtures.write(work, "config", "")
    identity = {
        "AWS_PROFILE": "atmos-identity", "AWS_SHARED_CREDENTIALS_FILE": work + "/credentials", "AWS_CONFIG_FILE": work + "/config",
        "AWS_ENDPOINT_URL_STS": UNREACHABLE_STS, "AWS_REGION": "us-east-1", "AWS_EC2_METADATA_DISABLED": "true",
    }

    # get_aws_account_id() signs an STS request with the profile that the environment selects and sends it to the
    # configured endpoint. Nothing listens there, so the failure is the connection, which proves the request was
    # attempted with resolved credentials; credentials that were not found would fail before any connection.
    failed = semantics.terraform(root, "plan", "account", flags=["--logs-level=Debug"], env=identity, failing=True)
    semantics.expect(failed, "127.0.0.1:1", "STS request of the selected profile")
    semantics.expect_absent(failed, "could not be resolved", "STS request of the selected profile")
    semantics.expect_absent(failed, "AKIAINVOCATIONONLY", "STS request of the selected profile")

    # A profile that does not exist is named instead of falling back to other credentials.
    missing = dict(identity)
    missing["AWS_PROFILE"] = "no-such-profile"
    unresolved = semantics.terraform(root, "plan", "account", flags=["--logs-level=Debug"], env=missing, failing=True)
    semantics.expect(unresolved, "no-such-profile", "unresolvable profile")
    semantics.expect(unresolved, "invocation environment", "unresolvable profile")
    ui.success("AWS identity: functions in a unit use the profile, files, and endpoint of the invocation, and name what is missing")
