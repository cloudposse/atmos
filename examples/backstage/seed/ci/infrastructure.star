def run(operation):
    if operation not in ["plan", "apply"]:
        fail("Expected plan or apply")
    atmos.validate("stacks")
    services = affected_services(os.getenv("BASE_REVISION"))
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
    for service in services:
        flags = {"identity": "false"}
        if operation == "apply":
            flags["auto-approve"] = True
        atmos.terraform(operation, "bucket", service["stack"], flags = flags)
        if operation == "apply":
            register(service["name"])

def affected_services(base):
    if not base:
        fail("BASE_REVISION must identify the reviewed base commit")
    if base.strip("0") == "":
        # A new branch has no base commit. Describe the current configuration,
        # which also handles the seed commit before any services exist.
        stacks = json.decode(atmos.describe("stacks", flags = {"format": "json"}, output = "capture").stdout)
        affected = []
        for stack, config in stacks.items():
            for component in config.get("components", {}).get("terraform", {}):
                affected.append({"component_type": "terraform", "component": component, "stack": stack})
    else:
        # Native selection includes shared Terraform source and inherited stack
        # changes, not just edits to individual service manifest filenames.
        affected = json.decode(atmos.describe("affected", flags = {
            "base": base, "format": "json", "include-dependents": True,
        }, output = "capture").stdout)
    return select_services(affected)

def select_services(affected):
    services = []
    seen = {}
    for item in affected:
        if item.get("component_type") != "terraform" or item.get("component") != "bucket":
            continue
        stack = item["stack"]
        if not stack.startswith("dev-"):
            fail("Unexpected bucket stack: " + stack)
        name = stack.removeprefix("dev-")
        if not regex.search("^[a-z][a-z0-9-]{1,28}[a-z0-9]$", name):
            fail("Invalid service stack name: " + stack)
        # Destruction needs a separate reviewed workflow while the configuration
        # still exists. Never silently treat deletion as a successful deployment.
        if item.get("deleted", False):
            fail("Service deletion requires explicit destroy before removing its stack: " + stack)
        if stack not in seen:
            services.append({"stack": stack, "name": name})
            seen[stack] = True
    return services

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
