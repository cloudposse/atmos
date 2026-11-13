def service_name(name):
    if not regex.search("^[a-z][a-z0-9-]{1,28}[a-z0-9]$", name):
        fail("service must be 3-30 lowercase letters/digits/hyphens, starting with a letter and ending with a letter/digit")
    return name

def run_terraform(operation, name):
    name = service_name(name)
    atmos.git("pull", "deployments")
    repo = ".atmos/git/deployments"
    path = "stacks/deploy/services/" + name + ".yaml"
    # Require the reviewed configuration to exist on the checked-out main commit.
    exec.run(["git", "cat-file", "-e", "HEAD:" + path], working_directory = repo, output = "capture")
    flags = {"identity": "local-aws"}
    if operation == "apply":
        flags["auto-approve"] = True
    atmos.terraform(operation, "bucket", "dev-" + name, flags = flags, working_directory = repo)
    return name

def seed():
    exec.run(["mkdir", "-p", ".atmos/ci/runner", ".atmos/ci/state"])
    # Seed once: repeated portal up must not overwrite reviewed infrastructure code.
    repo = ".atmos/git/deployments"
    if not fs.exists(repo + "/.git"):
        atmos.git("clone", "deployments")
    seeded = exec.run(["git", "cat-file", "-e", "HEAD:atmos.yaml"], working_directory = repo, output = "capture", check = False)
    if seeded.exit_code != 0:
        # There is no recursive copy operation in the filesystem module.
        exec.run(["cp", "-R", "seed/.", repo])
        atmos.git("commit", "deployments", flags = {
            "message": "Seed Atmos infrastructure and catalog",
            "path": [".gitignore", "atmos.yaml", "catalog", "components", "stacks", ".gitea", "ci"],
        })
        atmos.git("push", "deployments")
    ui.success("Git repository ready: http://localhost:3001/atmos/deployments")

def publish_request(name):
    name = service_name(name)
    atmos.scaffold("generate", "/demo/scaffolds/service", "generated", flags = {
        "defaults": True,
        "set": ["service_name=" + name],
    })
    atmos.git("clone", "request")
    # Publish only missing files so retries preserve changes made by reviewers.
    # Separate roots also exclude scaffold bookkeeping from the infrastructure repo.
    for root in ["stacks/deploy/services", "catalog"]:
        if not fs.exists("repository/" + root + "/" + name + ".yaml"):
            steps.publish(
                source = "generated/" + root + "/" + name + ".yaml",
                target = {
                    "kind": "git",
                    "repository": "request",
                    "path": root,
                    "commit": {"message": "Request " + name + ": add " + root},
                },
            )
