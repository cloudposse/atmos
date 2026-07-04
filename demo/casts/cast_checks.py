"""Shared assertions for cast validation scripts in atmos.d/**.

Validate steps run with cwd = demo/casts and import this module via:

    import os, sys
    sys.path.insert(0, os.getcwd())
    from cast_checks import load_text, assert_no_experimental, assert_body_color, assert_colored

These checks are regression tripwires only. The experimental indicator is
disabled at the source by ATMOS_EXPERIMENTAL=silence in the recording env
(cast-defaults.yaml .env.recording); color comes from ATMOS_FORCE_COLOR and
ATMOS_FORCE_TTY there. If either ever stops propagating to the recorded
commands again (see the !include env regression), these assertions fail the
recording instead of committing a broken cast.
"""

import json
import os
import re
from pathlib import Path

# A line that is just the simulated prompt (optionally styled) with typed input.
_PROMPT_LINE = re.compile(r"^(?:\x1b\[[0-9;]*m)*> ")
_SGR = re.compile(r"\x1b\[[0-9;]*m")


def load_text(path):
    """Decode a .cast file's output events into terminal text."""
    raw = Path(path).read_text()
    lines = raw.splitlines()
    return "".join(
        json.loads(line)[2] for line in lines[1:] if json.loads(line)[1] in ("o", "e")
    )


def assert_no_experimental(text):
    """The experimental indicator must be silenced at the recording source."""
    if "\U0001f9ea" in text or "experimental" in text.lower():
        raise SystemExit(
            "cast contains the experimental indicator - ATMOS_EXPERIMENTAL=silence "
            "did not reach the recorded commands"
        )


def assert_body_color(text):
    """Command output (non-prompt lines) must carry ANSI color.

    The simulated prompt is styled by the recorder itself, so its lines prove
    nothing about the recorded commands - require color on the body instead.
    """
    body = [l for l in text.splitlines() if l and not _PROMPT_LINE.match(l)]
    if not any(_SGR.search(l) for l in body):
        raise SystemExit(
            "cast body is missing ANSI color - forced color did not reach the "
            "recorded commands"
        )


def command_slug(command):
    """Convert a screengrab manifest command into its artifact slug.

    Matches the slugs produced by the legacy screengrab pipeline so existing
    docs references keep working (e.g. "atmos about --help" → "atmos-about--help").
    """
    slug = command.replace(" --charset=UTF-8", "")
    slug = slug.replace(" -", "-")
    slug = re.sub(r"\s+", "-", slug)
    return slug.replace("---", "--")


def sanitize_paths(text, repo_root):
    """Rewrite machine-specific repo paths to the stable harness placeholder.

    Uses the same "/absolute/path/to/repo" convention as the CLI test
    harness (tests/cli_test.go sanitizeOutput) so committed casts are
    identical no matter which machine records them.
    """
    placeholder = "/absolute/path/to/repo"
    roots = {str(repo_root), os.path.realpath(str(repo_root))}
    # macOS: cwd-derived paths may carry the /private prefix for /tmp and /var.
    roots.update("/private" + root for root in list(roots) if not root.startswith("/private"))
    for root in sorted(roots, key=len, reverse=True):
        text = text.replace(root, placeholder)
    return text


def assert_no_error_output(text):
    """No Atmos error-builder output may appear in a committed cast.

    Catches errors printed by commands that still exit zero; non-zero exits
    already fail the recording (the cast step discards the cast).
    """
    for marker in (
        "# Error",
        "**Error:**",
        "## Explanation",
        "## Hints",
        "Incorrect Usage",
        "Value for undeclared variable",
        "Values for undeclared variables",
        "undeclared variable",
        "undeclared variables",
        "Workspace \"",
        "You're now on a new, empty workspace",
        "currently selected workspace",
        "panic:",
    ):
        if marker in text:
            raise SystemExit(f"cast contains error output marker {marker!r}")


def assert_no_local_paths(text):
    """No machine-specific absolute paths may leak into a committed cast."""
    for marker in ("/Users/", "/home/", "/private/var/folders/", "C:\\Users\\"):
        if marker in text:
            raise SystemExit(
                f"cast leaks a machine-specific path (contains {marker!r})"
            )


def strip_ansi(text):
    """Remove ANSI escape sequences so needles match colorized output.

    Syntax-highlighted YAML places SGR codes between keys and values, so plain
    "key: value" substrings only match against stripped text.
    """
    return re.sub(r"\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07]*\x07", "", text)


def assert_colored(text, needle):
    """Require an SGR sequence on the same line as a known Atmos-rendered string.

    The needle is matched against the ANSI-stripped line (styling can split a
    phrase mid-word), while the color requirement checks the raw line.
    """
    for line in text.splitlines():
        if needle in strip_ansi(line) and _SGR.search(line) and not _PROMPT_LINE.match(line):
            return
    raise SystemExit(f"cast does not render {needle!r} with color")
