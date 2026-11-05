"""Shared cast validation for embedded Starlark steps."""

def load_text(path):
    lines = fs.read_file(path).splitlines()
    if not lines:
        fail("empty cast: " + path)
    header = json.decode(lines[0])
    if header.get("version") not in [2, 3]:
        fail("unsupported cast version: " + path)
    events = [json.decode(line) for line in lines[1:] if line.strip()]
    return "".join([event[2] for event in events if event[1] in ["o", "e"]])

def strip_ansi(text):
    return regex.replace(r"\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)", "", text)

def assert_no_experimental(text):
    if "🧪" in text or "experimental" in text.lower():
        fail("cast contains the experimental indicator")

def assert_colored(text, needle):
    for line in text.splitlines():
        if needle in strip_ansi(line) and regex.search(r"\x1b\[[0-9;]*m", line):
            return
    fail("cast does not render {!r} with color".format(needle))

def assert_no_error_output(text):
    plain = strip_ansi(text)
    for marker in [
        "# Error", "**Error:**", "## Explanation", "## Hints", "Incorrect Usage",
        "Value for undeclared variable", "Values for undeclared variables", "undeclared variable",
        'Workspace "', "You're now on a new, empty workspace", "currently selected workspace",
        "panic:", "Incomplete lock file information for providers",
    ]:
        if marker in plain:
            fail("cast contains error output marker {!r}".format(marker))

def assert_no_local_paths(text):
    plain = strip_ansi(text)
    for marker in ["/Users/", "/home/", "/private/var/folders/", "C:\\Users\\", "stack-imports/"]:
        if marker in plain:
            fail("cast leaks a local path: " + marker)
    if ".cache/atmos/" in plain.replace("/sanitized/.cache/atmos/", "") or regex.search(r"(?:\.\./){4,}", plain):
        fail("cast leaks a local cache or deeply relative path")
