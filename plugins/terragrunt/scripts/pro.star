# Opt-in CLI acceptance tests for running Terragrunt units under Atmos Pro and CI, where `plan` and
# `apply` happen on different runners and the plan travels as an artifact. Real OpenTofu or
# Terraform, local state, and the built-in terraform_data resource; no cloud and no Atmos Pro API
# (the payloads sent to Atmos Pro are covered with a fake API in the host's tests/ package).
#
# A runner is modelled by its execution state: the checkout (sources) and the remote backend (the
# unit's state directory stands in for it) survive, while the whole `.workdir` of the earlier run is
# gone. Only the documented artifact set, the plan file and its record, comes back from the plan
# store. Importing this module has no effects; each integration step calls one scenario.
fixtures = module.load("fixtures.star")
lifecycle = module.load("lifecycle_extended.star")
hardening = module.load("hardening.star")

STACK = lifecycle.STACK
REVIEWED_SHA = "0123456789abcdef0123456789abcdef01234567"


def atmos_in(root, arguments, failing=False):
    # The planfile store keys artifacts by commit; the fixture is not a Git repository, so say which.
    result = atmos.run(
        arguments,
        working_directory=root,
        env={"NO_COLOR": "1", "ATMOS_TELEMETRY_ENABLED": "false", "TF_IN_AUTOMATION": "true", "POC_REVISION": "r1", "ATMOS_CI_SHA": REVIEWED_SHA},
        output="capture",
        check=False,
    )
    label = " ".join(arguments) + ":\n" + result.stdout + result.stderr
    if failing:
        fixtures.check(result.exit_code != 0, label + "\nunexpectedly succeeded")
    else:
        fixtures.check(result.exit_code == 0, label)
    return result


def reviewed_plan_in_store():
    # The plan runner: plan the unit, then publish the plan artifact to the store.
    root = hardening.new_project("atmos-terragrunt-pro-")
    lifecycle.write_unit(root, "planned", lifecycle.PLAN_UNIT.replace("@GREETING@", "reviewed"))
    lifecycle.terraform(root, "plan", "planned")
    published = atmos_in(root, ["terraform", "planfile", "upload", "planned", "-s", STACK, "--sha", REVIEWED_SHA])
    hardening.expect(published, "Uploaded planfile", "the planfile upload")
    fixtures.check(not fs.exists(lifecycle.state_path(root, "planned")), "Planning created state")
    return root


def new_runner(root):
    # Everything the earlier run left in its execution state is gone; the checkout and backend remain.
    fs.remove(root + "/.workdir", recursive=True)
    fixtures.check(len(lifecycle.planfiles(root)) == 0, "The new runner still has a plan file")


def relocated_checkout_restores_relative_workdir_plan():
    # Keep the backend and plan store outside either checkout, as a real CI service would.
    shared = fixtures.workspace("atmos-terragrunt-shared-")
    root = lifecycle.project(fixtures.workspace("atmos-terragrunt-plan-runner-"), "project", fs.resolve("."), hardening.engine(), stacks=True)
    config = json.decode(fs.read_file(root + "/atmos.yaml"))
    store = shared + "/store"
    state = shared + "/state/terraform.tfstate"
    config["components"]["terraform"]["planfiles"] = {"default": "local", "stores": {"local": {"type": "local/dir", "options": {"path": store}}}}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    lifecycle.write_unit(root, "planned", lifecycle.PLAN_UNIT.replace("@GREETING@", "relocated"))
    unit = "live/dev/planned/terragrunt.hcl"
    original = fs.read_file(root + "/" + unit)
    fixtures.write(root, unit, original.replace('config  = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }', 'config  = { path = "' + lifecycle.hcl_path(state) + '", workspace_dir = "' + lifecycle.hcl_path(shared + "/state/workspaces") + '" }'))
    fixtures.write(root, "stacks/portable.yaml", "import:\n  - terragrunt://live/dev/planned\ncomponents:\n  terraform:\n    planned:\n      metadata:\n        working_directory: .workdir/portable\n")

    atmos_in(root, ["terraform", "plan", "planned", "-s", "portable"])
    atmos_in(root, ["terraform", "planfile", "upload", "planned", "-s", "portable", "--sha", REVIEWED_SHA])
    fixtures.check(not fs.exists(state), "Planning created backend state")

    apply_root = fixtures.workspace("atmos-terragrunt-apply-runner-") + "/project"
    fixtures.copy_tree(root, apply_root)
    if fs.exists(apply_root + "/.workdir"):
        fs.remove(apply_root + "/.workdir", recursive=True)
    fixtures.check(root != apply_root, "The apply runner reused the planning checkout")
    atmos_in(apply_root, ["terraform", "planfile", "download", "planned", "-s", "portable"])
    files = fs.glob(apply_root + "/.workdir/portable/*.planfile")
    fixtures.check(len(files) == 1 and fs.exists(files[0] + ".atmos.json"), "The relocated runner did not restore the plan and record: " + str(files))
    applied = atmos_in(apply_root, ["terraform", "apply", "planned", "-s", "portable", "--from-plan", "--", "-no-color"])
    fixtures.check("Plan:" not in applied.stdout, "The relocated apply planned again:\n" + applied.stdout)
    fixtures.check("Apply complete! Resources: 1 added" in applied.stdout, "The relocated checkout did not apply the reviewed plan:\n" + applied.stdout)
    fixtures.check(fs.exists(state), "The apply runner did not use the shared backend")
    fixtures.check(not fs.exists(files[0] + ".atmos.json"), "The consumed plan record was retained")
    again = atmos_in(apply_root, ["terraform", "apply", "planned", "-s", "portable", "--from-plan", "--", "-no-color"], failing=True)
    fixtures.check("no saved plan found" in again.stdout + again.stderr, "The consumed plan could be applied twice:\n" + again.stdout + again.stderr)


def restore_artifacts(root):
    # The apply runner restores the artifact where this unit's workdir will be.
    restored = atmos_in(root, ["terraform", "planfile", "download", "planned", "-s", STACK])
    hardening.expect(restored, "Downloaded planfile", "the planfile download")
    files = lifecycle.planfiles(root)
    fixtures.check(len(files) == 1, "Expected the restored plan file, found " + str(files))
    fixtures.check(fs.exists(files[0] + ".atmos.json"), "The plan record did not travel with the plan")
    workdir = files[0].rsplit("/", 1)[0]
    fixtures.check(sorted([path.replace("\\", "/").rsplit("/", 1)[1] for path in fs.glob(workdir + "/*")]) == sorted([files[0].replace("\\", "/").rsplit("/", 1)[1], files[0].replace("\\", "/").rsplit("/", 1)[1] + ".atmos.json"]), "Restoring must write only the documented artifact set: " + str(fs.glob(workdir + "/*")))
    return files[0]


def expect_rejected(root, reason, context):
    result = lifecycle.terraform(root, "apply", "planned", flags=["--from-plan"], failing=True)
    output = hardening.output_of(result)
    # Error text is wrapped to the terminal width, which can split a phrase across lines.
    fixtures.check(reason in " ".join(output.split()), "Expected '" + reason + "' for " + context + ":\n" + output)
    fixtures.check("Apply complete" not in output, "A plan was applied despite " + context)
    fixtures.check(not fs.exists(lifecycle.state_path(root, "planned")), context + " created state")


def restored_plan_is_applied_on_another_runner():
    root = reviewed_plan_in_store()
    module_path = "modules/unit/main.tf"
    unit_path = "live/dev/planned/terragrunt.hcl"
    original_module = fs.read_file(root + "/" + module_path)
    original_unit = fs.read_file(root + "/" + unit_path)

    # A tampered plan, a swapped record, and a missing record are all refused.
    new_runner(root)
    planfile = restore_artifacts(root)
    fs.write_file(planfile, "tampered", overwrite=True)
    expect_rejected(root, "create a new plan", "a tampered plan")

    new_runner(root)
    planfile = restore_artifacts(root)
    record = json.decode(fs.read_file(planfile + ".atmos.json"))
    record["PlanDigest"] = "0" * 64
    fs.write_file(planfile + ".atmos.json", json.encode(record), overwrite=True)
    expect_rejected(root, "create a new plan", "a record that belongs to another plan")

    new_runner(root)
    planfile = restore_artifacts(root)
    fs.remove(planfile + ".atmos.json")
    expect_rejected(root, "no Atmos plan record", "a plan without its record")

    # A different source revision: the module or the unit differs from what was reviewed.
    new_runner(root)
    restore_artifacts(root)
    fixtures.write(root, module_path, original_module + "# another revision\n")
    expect_rejected(root, "create a new plan", "a different module revision")
    fixtures.write(root, module_path, original_module)

    new_runner(root)
    restore_artifacts(root)
    fixtures.write(root, unit_path, original_unit.replace("reviewed", "edited-after-review"))
    expect_rejected(root, "create a new plan", "a different unit revision")
    fixtures.write(root, unit_path, original_unit)

    # A working directory that holds anything beyond the restored artifacts cannot be verified.
    new_runner(root)
    planfile = restore_artifacts(root)
    fs.write_file(planfile.rsplit("/", 1)[0] + "/rogue.tf", 'output "rogue" { value = "unreviewed" }\n')
    expect_rejected(root, "not part of the reviewed plan", "unreviewed files in the workdir")

    # With only the documented artifact set restored, the workdir is re-provisioned from the checkout
    # and the reviewed plan applies exactly, without planning again.
    new_runner(root)
    planfile = restore_artifacts(root)
    applied = lifecycle.terraform(root, "apply", "planned", flags=["--from-plan"])
    fixtures.check("Plan:" not in applied.stdout, "Applying a restored plan planned again:\n" + applied.stdout)
    fixtures.check("Apply complete! Resources: 1 added" in applied.stdout, applied.stdout)
    fixtures.check(lifecycle.state_output(root, "planned") == "reviewed|||", "Applied value differs from the reviewed plan")
    fixtures.check(not fs.exists(planfile), "A consumed plan file was kept")
    fixtures.check(not fs.exists(planfile + ".atmos.json"), "A consumed plan record was kept")

    # The same artifact cannot be applied twice.
    again = lifecycle.terraform(root, "apply", "planned", flags=["--from-plan"], failing=True)
    hardening.expect(again, "has no saved plan", "a second apply of the restored plan")
    ui.success("Cross-runner saved plans: tamper, record, revision, and workdir checks; exact apply from the documented artifact set")


def deploy_applies_after_verifying_the_stored_plan():
    # `deploy` in CI reconciles instead of replaying: it downloads the stored plan, plans again, and
    # applies the new plan only when the two agree.
    root = hardening.new_project("atmos-terragrunt-deploy-")
    config = json.decode(fs.read_file(root + "/atmos.yaml"))
    store = root + "/store"
    config["components"]["terraform"]["planfiles"] = {"default": "local", "stores": {"local": {"type": "local/dir", "options": {"path": store}}}}
    config["ci"] = {"enabled": True}
    fixtures.write(root, "atmos.yaml", json.encode(config))
    lifecycle.write_unit(root, "planned", lifecycle.PLAN_UNIT.replace("@GREETING@", "reviewed"))

    atmos_in(root, ["terraform", "plan", "planned", "-s", STACK, "--ci"])
    artifacts = fs.glob(store + "/live/dev/planned/*.tfplan.tar")
    fixtures.check(len(artifacts) == 1, "The CI plan did not store its artifact: " + str(fs.glob(store + "/live/dev/planned/*")))
    deployed = atmos_in(root, ["terraform", "deploy", "planned", "-s", STACK, "--ci"])
    output = hardening.output_of(deployed)
    fixtures.check("Apply complete! Resources: 1 added" in output, "deploy did not apply the verified plan:\n" + output)
    fixtures.check("without verification" not in output, "deploy applied without verifying the stored plan:\n" + output)
    fixtures.check(lifecycle.state_output(root, "planned") == "reviewed|||", "Deployed value differs from the reviewed plan")
    ui.success("Deploy: the stored plan of a unit is verified against a fresh plan before it is applied")
