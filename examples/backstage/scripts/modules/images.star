# Both images must come from the same reviewed example revision.
def prepare(portal, runner):
    if not portal and not runner:
        return
    if not portal or not runner:
        fail("Prebuilt startup requires both --portal-image and --runner-image")
    for image in [portal, runner]:
        parts = image.split("@sha256:")
        if len(parts) != 2 or not parts[0] or len(parts[1]) != 64:
            fail("Prebuilt images must use a registry reference pinned by @sha256 digest")
        for index in range(len(parts[1])):
            if parts[1][index] not in "0123456789abcdef":
                fail("Invalid SHA-256 image digest")
    # Pull both before changing the local aliases. No source compilation is needed.
    exec.run(["docker", "pull", portal])
    exec.run(["docker", "pull", runner])
    exec.run(["docker", "tag", portal, "atmos-backstage:local"])
    exec.run(["docker", "tag", runner, "atmos-backstage-runner:local"])
