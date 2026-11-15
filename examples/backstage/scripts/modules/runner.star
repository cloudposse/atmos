# Development option (`portal up --runner-from-source`): the runner normally uses the
# upstream Atmos release image. This builds Atmos from this checkout instead, to
# exercise native CI and Git features that are not released yet. It requires Go.
def build_binary():
    architecture = atmos.container("exec", "backstage", flags = {"stack": "local"},
                                   args = ["--", "arch"], output = "capture").stdout.strip()
    targets = {"aarch64": "arm64", "x86_64": "amd64"}
    if architecture not in targets:
        fail("Unsupported runner architecture: " + architecture)
    destination = fs.resolve(".atmos/bin/atmos")
    exec.run(["mkdir", "-p", ".atmos/bin"])
    steps.spin(
        title = "Building runner Atmos from this checkout (linux/" + targets[architecture] + ")",
        command = 'go build -ldflags="-s -w" -o "$ATMOS_RUNNER_BINARY" .',
        working_directory = fs.resolve("../.."),
        env = {"GOOS": "linux", "GOARCH": targets[architecture], "CGO_ENABLED": "0", "ATMOS_RUNNER_BINARY": destination},
    )
