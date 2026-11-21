# Verify that discovery works from a packaged binary, without a development path.
fixtures = module.load("fixtures.star")


def copy_source(source, target):
    # Keep local build products out of the disposable source tree.
    fs.mkdir(target, parents=True)
    for path in fs.glob(source + "/*"):
        name = path.replace("\\", "/").rsplit("/", 1)[1]
        if name in ["bin", ".git", ".atmos", ".terraform", ".terragrunt-cache", ".terragrunt-stack"]:
            continue
        if fs.stat(path).is_dir:
            copy_source(path, target + "/" + name)
        else:
            fs.write_file(target + "/" + name, fs.read_file(path))


def pack_native_release(work, package):
    # Build against the exact manifest being packed: the handshake binds its digest.
    target = exec.run(["go", "env", "GOOS", "GOARCH"], output="capture").stdout.split()
    platform = target[0] + "/" + target[1]
    payload = work + "/source/plugins/terragrunt"
    copy_source(package, payload)
    copy_source(fs.resolve("../../sdk"), work + "/source/sdk")
    manifest = yaml.decode(fs.read_file(payload + "/plugin.yaml"))
    executable = manifest["spec"]["runtime"]["executable"]
    manifest["spec"]["platforms"] = {platform: {"executable": executable}}
    fixtures.write(payload, "plugin.yaml", json.encode(manifest))
    fixtures.run(work, ["plugin", "build", payload])
    layout = work + "/release"
    fixtures.run(work, ["plugin", "pack", payload, "--output", layout])
    verify_registry_links(work, payload, layout, manifest)
    return struct(layout=layout, version=manifest["metadata"]["version"])


def release_annotations(layout):
    # The index manifest's annotations are what GHCR reads to link a package to its repository.
    index = json.decode(fs.read_file(layout + "/index.json"))
    digest = index["manifests"][0]["digest"].split(":")[1]
    return json.decode(fs.read_file(layout + "/blobs/sha256/" + digest))["annotations"]


def verify_registry_links(work, payload, layout, manifest):
    # The repository declared in plugin.yaml links the published package; nothing else is implied.
    annotations = release_annotations(layout)
    metadata = manifest["metadata"]
    fixtures.check(annotations["org.opencontainers.image.source"] == metadata["repository"], "The package is not linked to its repository: " + str(annotations))
    fixtures.check(annotations["org.opencontainers.image.version"] == metadata["version"], "Missing version annotation: " + str(annotations))
    fixtures.check(annotations["org.opencontainers.image.title"] == metadata["display_name"], "Missing title annotation: " + str(annotations))

    # An explicit source overrides the manifest, and a source that could leak credentials is refused.
    override = work + "/release-override"
    fixtures.run(work, ["plugin", "pack", payload, "--output", override, "--source", "https://github.com/acme/internal-plugins"])
    fixtures.check(release_annotations(override)["org.opencontainers.image.source"] == "https://github.com/acme/internal-plugins", "--source did not override the manifest repository")
    refused = fixtures.run_failing(work, ["plugin", "pack", payload, "--output", work + "/release-unsafe", "--source", "https://user:token@github.com/acme/internal-plugins"])
    fixtures.check("without credentials" in refused.stdout + refused.stderr, "A source with credentials must be refused with an explanation:\n" + refused.stderr)
    fixtures.check(not fs.exists(work + "/release-unsafe"), "A refused pack left a layout behind")


def install_and_discover(work, package, release):
    project = work + "/project"
    fixtures.copy_tree(package + "/fixtures/explicit-stack", project)
    config = fixtures.configuration(package)
    # Keep cache writes disposable and prove installation does not need a registry.
    config["plugins"] = {
        "packages": {},
        "cache_dir": work + "/cache",
        "policy": {"offline": True},
        "discovery": {"default_registry": False},
    }
    fixtures.write(project, "atmos.yaml", json.encode(config))
    source = "oci://registry.example.invalid/atmos/terragrunt"
    fixtures.run(project, ["plugin", "install", source, "--as", "terragrunt", "--version", release.version, "--from-layout", release.layout])
    lock_path = project + "/.atmos.plugins.lock.yaml"
    before = fs.read_file(lock_path)
    fixtures.run(project, ["plugin", "install", "--frozen"])
    fixtures.check(before == fs.read_file(lock_path), "Frozen restore changed the release lock")
    installed = yaml.decode(fs.read_file(project + "/atmos.yaml"))["plugins"]["packages"]["terragrunt"]
    fixtures.check("path" not in installed, "Installed package still uses a development path")
    result = fixtures.run(project, ["list", "instances", "--format=json"])
    rows = json.decode(result.stdout)
    identities = sorted([(row["Component"], row["Stack"]) for row in rows])
    expected = [(name, "live") for name in ["dev/app", "dev/network", "prod/app", "prod/network"]]
    fixtures.check(identities == expected, "Unexpected installed unit identities: " + str(identities))
    fixtures.check(all([row["Type"] == "terraform" for row in rows]), "Installed units lost their component type")
    fixtures.check(not fs.exists(project + "/live/.terragrunt-stack"), "Installed discovery wrote generated stacks")


def main():
    package = fs.resolve(".")
    work = fixtures.workspace("atmos-terragrunt-release-")
    # Packing needs no Terraform executable and does not evaluate HCL.
    fixtures.write(work, "atmos.yaml", "{}")
    release = pack_native_release(work, package)
    install_and_discover(work, package, release)
    ui.success("Native OCI package: offline install, frozen restore, and explicit-stack discovery")
