def run(operation):
    if operation not in ["plan", "apply"]:
        fail("Expected plan or apply")
    atmos.validate("stacks")
    # Use shared local state across ephemeral checkouts. Atmos workspaces separate services.
    fs.write_file("components/terraform/bucket/ci_backend_override.tf.json", json.encode({
        "terraform": {"backend": {"local": {
            "path": "/state/terraform.tfstate", "workspace_dir": "/state/workspaces",
        }}},
        "provider": {"aws": {
            "region": "us-east-1", "access_key": "test", "secret_key": "test",
            "skip_credentials_validation": True, "skip_requesting_account_id": True,
            "skip_metadata_api_check": True, "s3_use_path_style": True,
            "endpoints": [{"s3": "http://local-backstage-aws:4566", "sts": "http://local-backstage-aws:4566"}],
        }},
    }), overwrite = True)
    files = exec.run(["git", "diff", "--name-only", "--diff-filter=AM", os.getenv("BASE_REVISION"), "HEAD", "--", "stacks/deploy/services"], output = "capture").stdout.splitlines()
    for path in files:
        name = path.rsplit("/", 1)[1].removesuffix(".yaml")
        if not regex.search("^[a-z][a-z0-9-]{1,28}[a-z0-9]$", name):
            fail("Invalid service manifest name")
        flags = {"identity": "false"}
        if operation == "apply":
            flags["auto-approve"] = True
        atmos.terraform(operation, "bucket", "dev-" + name, flags = flags)
        if operation == "apply":
            register(name)

def register(name):
    base = "http://atmos-local-container-backstage:7007/api"
    token = steps.http(url = base + "/auth/guest/refresh", expect = {"status": [200]},
                       retry = {"max_attempts": 60, "initial_delay": "2s", "backoff_strategy": "constant"},
                       timeout = "5s", output = "none").data["backstageIdentity"]["token"]
    headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json"}
    target = "http://local-backstage-git:3000/atmos/deployments/raw/branch/main/catalog/" + name + ".yaml"
    steps.http(url = base + "/catalog/locations", method = "POST", headers = headers,
               body = json.encode({"type": "url", "target": target}), expect = {"status": [201, 409]}, output = "none")
    steps.http(url = base + "/catalog/refresh", method = "POST", headers = headers,
               body = json.encode({"entityRef": "component:default/" + name}), expect = {"status": [204, 404]}, output = "none")
    ui.success("Registered " + name + " in Backstage")
