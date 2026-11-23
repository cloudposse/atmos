# Shared helpers for disposable CLI fixtures. Importing this module has no effects.

def check(condition, message):
    # Preserve captured CLI diagnostics in the Atmos test report on failure.
    if not condition:
        fail(message)


def workspace(prefix):
    root = fs.mkdtemp(prefix=prefix)
    # Cleanup runs even when a later command or assertion fails.
    defer(fs.remove, root, recursive=True)
    return root


def copy_tree(source, target):
    # Use native filesystem operations rather than platform-specific shell tools.
    fs.mkdir(target, parents=True)
    for path in fs.glob(source + "/*"):
        name = path.replace("\\", "/").rsplit("/", 1)[1]
        destination = target + "/" + name
        if fs.stat(path).is_dir:
            copy_tree(path, destination)
        else:
            fs.write_file(destination, fs.read_file(path))


def configuration(package, scope="live/**", binary="terraform", base_path="."):
    return {
        "base_path": base_path,
        "components": {
            "terraform": {
                "base_path": "components/terraform",
                "command": binary,
                "terragrunt": {
                    "enabled": True,
                    "mode": "native",
                    "discovery": {
                        "enabled": True,
                        "included_paths": [scope],
                    },
                },
            },
        },
        "plugins": {"packages": {"terragrunt": {"path": package}}},
    }


def write(root, path, content):
    target = root + "/" + path
    fs.mkdir(target.rsplit("/", 1)[0], parents=True)
    fs.write_file(target, content, overwrite=True)


def invoke(root, arguments, revision="r1"):
    # Call the running Atmos binary with its normal process and output handling.
    return atmos.run(
        arguments,
        working_directory=root,
        env={
            "NO_COLOR": "1",
            "ATMOS_TELEMETRY_ENABLED": "false",
            "TF_IN_AUTOMATION": "true",
            "POC_REVISION": revision,
        },
        output="capture",
        check=False,
    )


def run(root, arguments, revision="r1"):
    result = invoke(root, arguments, revision)
    check(result.exit_code == 0, " ".join(arguments) + ":\n" + result.stdout + result.stderr)
    return result


def run_failing(root, arguments, revision="r1"):
    # Negative paths: the command must be rejected, and its diagnostics are returned.
    result = invoke(root, arguments, revision)
    check(result.exit_code != 0, " ".join(arguments) + " unexpectedly succeeded:\n" + result.stdout + result.stderr)
    return result


def git(root, *arguments):
    # Disable signing only for disposable fixture commits, never the source repo.
    result = exec.run(
        ["git", "-c", "commit.gpgsign=false"] + list(arguments),
        working_directory=root,
        output="capture",
    )
    return result.stdout.strip()


def commit(root, label):
    git(root, "add", ".")
    git(root, "commit", "-m", label)
    return git(root, "rev-parse", "HEAD")


def project(root, package, base_path="."):
    fs.mkdir(root, parents=True)
    git(root, "init", "-b", "main")
    git(root, "config", "user.name", "Atmos fixture")
    git(root, "config", "user.email", "fixture@example.invalid")
    config = configuration(package, base_path=base_path)
    write(root, "atmos.yaml", json.encode(config))
    return config
