# Load the shared YAML catalog and reject configuration mistakes before running checks.
def check_specs(path):
    catalog = yaml.decode(fs.read_file(path))
    if type(catalog) != "dict" or sorted(catalog.keys()) != ["checks"] or type(catalog["checks"]) != "list" or not catalog["checks"]:
        fail("Check catalog must contain a nonempty checks list: " + path)
    handlers = check_handlers()
    names = {}
    for spec in catalog["checks"]:
        validate_check_spec(spec, handlers)
        if spec["id"] in names:
            fail("Duplicate repository check: " + spec["id"])
        names[spec["id"]] = True
    return catalog["checks"]

# Validate catalog fields so typos cannot silently disable checks or formatting safeguards.
def validate_check_spec(spec, handlers):
    fields = {"id": "string", "handler": "string", "files": "string", "exclude": "string", "selection": "string", "fix": "bool", "always": "bool"}
    if type(spec) != "dict":
        fail("Each repository check must be a mapping")
    for key, value in spec.items():
        if key not in fields or type(value) != fields[key]:
            fail("Invalid repository check field: " + str(key))
    if not regex.search("^[a-z0-9][a-z0-9-]*$", spec.get("id", "")):
        fail("Repository check requires a lowercase id")
    if spec.get("handler") not in handlers:
        fail("Unknown handler for " + spec["id"] + ": " + str(spec.get("handler")))
    if spec.get("selection", "files") not in ["files", "added"]:
        fail("Unknown file selection for " + spec["id"])
    for key in ["files", "exclude"]:
        if key in spec:
            regex.search(spec[key], "")

# Bind declarative handler names to Starlark functions without a conditional dispatcher.
def check_handlers():
    return {
        "branch_file_limit": branch_file_limit,
        "gofumpt": gofumpt,
        "tidy": tidy,
        "lint_config": lint_config,
        "custom_lint": custom_lint,
        "module_compatibility": module_compatibility,
        "agent_file_sizes": lambda config, files: check_agent_file_sizes(),
        "symlinks": lambda config, files: check_symlinks(),
        "editorconfig": editorconfig,
        "trailing_whitespace": lambda config, files: format_text(config, files, trim_whitespace),
        "end_of_file": lambda config, files: format_text(config, files, fix_eof),
        "yaml_syntax": yaml_syntax,
        "large_files": large_files,
        "secrets": secrets,
    }

# Select existing regular files using this check’s own include and exclude patterns.
def selected(spec, files):
    return [p for p in files if regex.search(spec.get("files", "."), p) and (not spec.get("exclude") or not regex.search(spec["exclude"], p)) and fs.exists(p) and fs.stat(p, follow_symlinks = False).is_file]

# Choose the check’s input set, including added-only selection for size limits.
def check_files(spec, config):
    files = config[spec.get("selection", "files")]
    return selected(spec, files)

# Dispatch a named check through the registry with its selected files.
def run_check(name, config):
    specs = [s for s in config["checks"] if s["id"] == name]
    if not specs:
        fail("Unknown check " + name)
    spec = specs[0]
    check_handlers()[spec["handler"]](config, check_files(spec, config))

# Apply a text transformation inside the sandbox, skipping binary and non-UTF-8 files.
def format_text(config, files, transform):
    before = contents(files)
    for path in files:
        data = before[path]
        # Starlark strings preserve raw bytes; ignore binary/non-UTF-8 files.
        if "\x00" in data or "".join(data.codepoints()) != data:
            continue
        fixed = transform(data)
        if fixed != data:
            fs.write_file(path, fixed)
    report_changes(config, before)

# Validate every selected YAML file without evaluating custom tags or imposing a schema.
def yaml_syntax(config, files):
    for path in files:
        yaml.validate(fs.read_file(path))

# Reject added files above 500 KiB unless Git attributes mark them for LFS.
def large_files(config, files):
    oversized = []
    for path in files:
        if fs.stat(path).size > 500 * 1024:
            attr = exec.run(["git", "check-attr", "-z", "filter", "--", path], output = "capture").stdout
            if not attr.endswith("\x00lfs\x00"):
                oversized.append(path)
    if oversized:
        fail("Files exceed 500 KiB: " + ", ".join(oversized))

# Format selected Go files in bounded argument batches and report changes.
def gofumpt(config, files):
    before = contents(files)
    for offset in range(0, len(files), 50):
        exec.run(["gofumpt", "-w", "--"] + files[offset:offset + 50])
    report_changes(config, before)

# Validate the repository’s golangci-lint configuration with the installed tool.
def lint_config(config, files):
    exec.run(["golangci-lint", "config", "verify"])

# Run the prebuilt custom linter, using the explicit comparison base for PR checks.
def custom_lint(config, files):
    lint_env = {"GOLANGCI_NEW_FROM_REV": config["base"]} if config["scope"] == "pr" else {}
    exec.run(["go", "tool", "mage", "lint:precommit"], env = lint_env)

# Check the root module’s compatibility constraints through the existing Mage target.
def module_compatibility(config, files):
    exec.run(["go", "tool", "mage", "lint:goModCheck"])

# Run the existing Atmos validation checks against all files or the affected scope.
def editorconfig(config, files):
    args = ["validate", "--exclude", "tests/fixtures/**", "--exclude", "**/*.go", "--exclude", "specs/**", "--exclude", "README.md", "--format", "rich"]
    if config["scope"] != "all":
        args += ["--affected", "--base", config["base"]]
    atmos.run(args)

# Scan staged contents, the PR commit range, or the materialized tracked files for secrets.
def secrets(config, files):
    scans = {
        "staged": ["git", "--pre-commit", "--staged"],
        "pr": ["git", "--log-opts=" + config["base"] + "..HEAD"],
        # Only tracked contents are materialized in the sandbox.
        "all": ["dir", "."],
    }
    exec.run(["gitleaks"] + scans[config["scope"]] + ["--redact", "--verbose"])

# Capture file contents before formatting, retaining absent paths for newly generated files.
def contents(paths):
    return {p: fs.read_file(p) if fs.exists(p) else None for p in paths}

# Report formatter differences and fail check/commit modes so fixes require review.
def report_changes(config, before):
    changed = [p for p in before if before[p] != (fs.read_file(p) if fs.exists(p) else None)]
    if changed:
        print("Modified: " + ", ".join(changed))
        if config["mode"] != "format":
            fail("Formatting changed; review fixes and restage")

# Find each selected Go file’s nearest module, deduplicating modules and skipping vendor paths.
def module_dirs(files):
    modules = {}
    for path in files:
        if "/vendor/" in "/" + path or not (path.endswith(".go") or path.split("/")[-1] == "go.mod"):
            continue
        parts = path.split("/")[:-1]
        for n in range(len(parts), -1, -1):
            directory = "/".join(parts[:n]) or "."
            if fs.exists(directory + "/go.mod"):
                modules[directory] = True
                break
    return sorted(modules)

# Tidy each affected module sequentially and propagate both changes and operational failures.
def tidy(config, files):
    modules = module_dirs(files)
    paths = [directory + "/" + name for directory in modules for name in ["go.mod", "go.sum"]]
    before = contents(paths)
    failed = False
    for directory in sorted(modules):
        result = exec.run(["go", "mod", "tidy"], working_directory = directory, check = False)
        failed = failed or result.exit_code != 0
    report_changes(config, before)
    if failed:
        fail("go mod tidy failed")

# Find broken tracked symlinks using NUL-delimited Git entries to preserve unusual filenames.
def check_symlinks():
    entries = exec.run(["git", "ls-files", "-s", "-z"], output = "capture").stdout
    broken = []
    for entry in entries.split("\x00"):
        if not entry:
            continue
        metadata, _, path = entry.partition("\t")
        if metadata.split(" ")[0] == "120000" and not fs.exists(path):
            broken.append(path)
    if broken:
        (errors.build("Broken tracked symlinks")
            .with_explanation("\n".join(broken))
            .with_hint("Restore the targets or remove obsolete symlinks.")
            .fail())

# Enforce the existing size budgets for repository and agent instruction files.
def check_agent_file_sizes():
    limits = [("CLAUDE.md", 40000), (".conductor/*/CLAUDE.md", 40000), (".claude/agents/*.md", 25000)]
    oversized = []
    for pattern, limit in limits:
        for path in fs.glob(pattern):
            if not fs.exists(path):
                continue
            info = fs.stat(path)
            if info.is_file and info.size > limit:
                oversized.append("{}: {} bytes (limit {})".format(path, info.size, limit))
    if oversized:
        (errors.build("Agent instruction files exceed size limits")
            .with_explanation("\n".join(oversized))
            .with_hint("Move detailed guidance into linked docs; preserve mandatory instructions.")
            .fail())

# Keep existing LF/CRLF endings; whitespace cleanup does not add a final newline.
def trim_whitespace(data):
    lines = data.split("\n")
    result = []
    for i in range(len(lines)):
        line = lines[i]
        ending = "\r\n" if line.endswith("\r") else "\n"
        if i == len(lines) - 1:
            ending = ""
        result.append(line.rstrip(" \t\v\f\r") + ending)
    return "".join(result)

# Ensure nonempty text has one final newline while preserving LF or CRLF style.
def fix_eof(data):
    if not data:
        return data
    newline = "\r\n" if "\r\n" in data else "\n"
    body = data.rstrip("\r\n")
    return body + newline if body else ""

# Limit the resulting branch diff, including this commit’s staged changes, to 150 files.
def branch_file_limit(config, files):
    source_head = exec.run(["git", "-C", config["root"], "rev-parse", "--verify", "HEAD"], output = "capture", check = False)
    ancestor = "HEAD"  # The sandbox supplies an empty baseline for an initial commit.
    if source_head.exit_code == 0:
        base = exec.run(["git", "-C", config["root"], "rev-parse", "--verify", config["branch_base"] + "^{commit}"], output = "capture", check = False)
        if base.exit_code != 0:
            (errors.build("Branch comparison base is unavailable: " + config["branch_base"])
                .with_hint("Fetch the base ref or pass --base-ref <branch-or-SHA> to atmos check staged.")
                .fail())
        ancestor = exec.run(["git", "merge-base", "HEAD", base.stdout.strip()], output = "capture").stdout.strip()
    changed = exec.run(["git", "diff", "--cached", "--name-only", "-z", "--find-renames", ancestor, "--"], output = "capture").stdout
    count = len([path for path in changed.split("\x00") if path])
    if count > 150:
        (errors.build("Branch changes {} files; the repository limit is 150.".format(count))
            .with_hint("Split the changes into smaller PRs or a PR stack. For a stack, compare against the parent branch with --base-ref.")
            .fail())
