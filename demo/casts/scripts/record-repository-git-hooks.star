# Capture the live index's logical contents without refreshing or modifying it.
def index_state(root):
    return exec.run(["git", "ls-files", "--stage", "-z"], working_directory = root, output = "capture").stdout

# Assert that recording leaves the caller's staged files and selected working files intact.
def verify_unchanged(root, index, contents):
    if index_state(root) != index:
        fail("The live Git index changed during recording.")
    for path, before in contents.items():
        if fs.read_file(root + "/" + path) != before:
            fail("Working file changed during recording: " + path)

# Stage the tracked check files from a revision into the private index, skipping paths the revision lacks.
def stage_revision(root, private_index, base, paths):
    listing = exec.run(["git", "ls-tree", "-r", "-z", base, "--"] + paths, working_directory = root, output = "capture").stdout
    entries = [entry for entry in listing.split("\0") if entry]
    if not entries:
        fail("Revision " + base + " has none of the tracked check files.")
    for entry in entries:
        meta, path = entry.split("\t", 1)
        mode, _, sha = meta.split(" ")
        exec.run(["git", "update-index", "--add", "--cacheinfo", mode + "," + sha + "," + path], working_directory = root, env = private_index, output = "capture")

# Run the real repository suite with a private index containing actual working changes.
def record():
    root = fs.resolve("../..")
    paths = [
        ".atmos-git-hooks.yaml",
        ".atmos.d/check.yaml",
        ".atmos.d/format.yaml",
        "scripts/checks/entry.star",
        "scripts/checks/suite.star",
        "scripts/checks/checks.star",
        "docs/development.md",
    ]
    contents = {path: fs.read_file(root + "/" + path) for path in paths}
    defer(verify_unchanged, root, index_state(root), contents)
    exec.run(["mkdir", "-p", root + "/.context"], output = "capture")
    scratch = exec.run(["mktemp", "-d", root + "/.context/git-hooks-cast.XXXXXX"], output = "capture").stdout.strip()
    defer(exec.run, ["rm", "-rf", "--", scratch], output = "capture")
    private_index = {"GIT_INDEX_FILE": scratch + "/index"}
    exec.run(["git", "read-tree", "HEAD"], working_directory = root, env = private_index, output = "capture")
    base = os.getenv("ATMOS_CAST_GIT_HOOKS_BASE") or "HEAD"
    if base == "HEAD":
        exec.run(["git", "add", "--"] + paths, working_directory = root, env = private_index, output = "capture")
    else:
        # --base records the tracked check files as they were at that revision instead of the
        # working tree, so a real diff against HEAD exists when the working tree has no edits.
        stage_revision(root, private_index, base, paths)
    selected = exec.run(["git", "diff", "--cached", "--name-only", "-z"], working_directory = root, env = private_index, output = "capture").stdout
    if not selected:
        fail("Recording needs actual edits to the repository check catalog, scripts, commands, or development guide relative to " + base + ". Make an intended change first, or pass --base <rev> to record the diff against an earlier revision; no demo files are synthesized.")
    # Fail before recording if checks would fix anything; the hook then runs the same inputs.
    atmos.run(["check", "staged"], working_directory = root, env = private_index, output = "capture")
    atmos.run(["casts", "record", "repository", "git-hooks"], working_directory = fs.resolve("."), env = private_index)

record()
