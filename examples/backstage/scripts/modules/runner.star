# Development option (`portal up --runner-from-source`): the runner normally uses the
# upstream Atmos release image. This builds Atmos from this checkout instead, to
# exercise native CI and Git features that are not released yet. It calls the repository's
# `atmos build` command, which requires Go.
def build_binary():
    architecture = atmos.container("exec", "backstage", flags = {"stack": "local"},
                                   args = ["--", "arch"], output = "capture").stdout.strip()
    targets = {"aarch64": "arm64", "x86_64": "amd64"}
    if architecture not in targets:
        fail("Unsupported runner architecture: " + architecture)
    destination = fs.resolve(".atmos/bin/atmos")
    steps.spin(
        title = "Building runner Atmos from this checkout (linux/" + targets[architecture] + ")",
        command = 'atmos build binary --target=linux --output="$ATMOS_RUNNER_BINARY"',
        working_directory = fs.resolve("../.."),
        env = {"GOARCH": targets[architecture], "ATMOS_RUNNER_BINARY": destination},
    )
