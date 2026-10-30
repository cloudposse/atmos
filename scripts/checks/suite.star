load("checks.star", "check_specs", "check_files", "module_dirs")

# Prepare formatter side effects and execute the enabled checks sequentially through the test UI.
def run_suite(sandbox, mode, only, base, skip):
    specs = check_specs(sandbox.root + "/.atmos-git-hooks.yaml")
    config = {
        "root": sandbox.root,
        "scope": sandbox.scope,
        "mode": mode,
        "base": sandbox.base,
        "branch_base": base,
        "only": only,
        "files": sandbox.files,
        "added": sandbox.added,
        "checks": specs,
    }
    modules = module_dirs(sandbox.files)
    sandbox.track([directory + "/" + name for directory in modules for name in ["go.mod", "go.sum"]])
    for binary in ["custom-gcl", "custom-gcl.exe"]:
        if fs.exists(sandbox.root + "/" + binary) and not fs.exists(binary):
            sandbox.copy([binary])
    only = [s for s in config["only"].split(",") if s]
    skip = skip.split(",")
    names = [spec["id"] for spec in specs]
    for name in only + [s for s in skip if s]:
        if name not in names:
            fail("Unknown repository check: " + name)

    # Pass filenames as data, never as step script text (which supports templates).
    fs.write_file(".git/repository-checks.json", json.encode(config))
    fs.write_file(".git/repository-checks.star", fs.read_file(sandbox.root + "/scripts/checks/checks.star"))
    checks = []
    for spec in specs:
        if only and spec["id"] not in only:
            continue
        enabled = spec["id"] not in skip
        enabled = enabled and (config["mode"] != "format" or spec.get("fix", False))
        enabled = enabled and (spec.get("always", False) or bool(check_files(spec, config)))
        checks.append({
            "name": spec["id"],
            "type": "script",
            "interpreter": "starlark",
            "when": "true" if enabled else "false",
            "script": "load(\".git/repository-checks.star\", \"run_check\")\nrun_check(" + json.encode(spec["id"]) + ", json.decode(fs.read_file(\".git/repository-checks.json\")))",
        })

    if not config["files"]:
        message = "No files selected for **" + config["scope"] + "** checks."
        if config["scope"] == "pr":
            command = "format" if config["mode"] == "format" else "check"
            message += " PR scope uses committed changes; stage edits and use `atmos " + command + " staged`, or use `atmos " + command + " all` for tracked working files."
        ui.info(message)
    steps.test(
        name = "Repository checks · " + config["scope"],
        output = "failures",
        fail = {"mode": "wait_all"},
        steps = checks,
    )
