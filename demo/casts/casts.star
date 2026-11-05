"""Shared settings for repository cast declarations; the constructor is cast.recorder."""
load("cast_checks.star", "assert_no_experimental", "assert_no_error_output", "assert_no_local_paths", "strip_ansi")

def demo_policy(text):
    assert_no_experimental(text)
    assert_no_error_output(text)
    assert_no_local_paths(text)
    if not regex.search(r"\x1b\[[0-9;]*m", text):
        fail("cast has no color")
    if regex.search(r"\bgho_[A-Za-z0-9]{36}\b", strip_ansi(text)):
        fail("cast leaks a GitHub token")
    if regex.search(r"\[(?:step|list-stacks|env|env-file|build-run)\]", strip_ansi(text)):
        fail("cast leaks step labels")

def demo_config(name, fixture):
    root = fs.resolve("../..")
    return {
        "working_directory": "../../.context/cast-fixtures/" + fixture,
        "output": {"mode": "raw", "cast": "../../website/static/casts/" + name + ".cast"},
        "defaults": {
            "cast": {"rate": "12ms", "width": 120, "height": 36},
            "simulate": {"mode": "typed", "cursor": True, "prompt": {"text": "> ", "style": "command"}, "rate": "35ms", "jitter": 0.25},
        },
        "env": {
            "NO_COLOR": "", "TERM": "xterm-256color", "COLORTERM": "truecolor",
            "ATMOS_FORCE_COLOR": "1", "CLICOLOR_FORCE": "1", "FORCE_COLOR": "1",
            "ATMOS_FORCE_TTY": "true", "ATMOS_KEYRING_TYPE": "memory",
            "ATMOS_EXPERIMENTAL": "silence", "ATMOS_LOGS_LEVEL": "Warning",
            "ATMOS_TELEMETRY_ENABLED": "false",
        },
        "show": {"labels": False},
        "replacements": {root: "/absolute/path/to/repo", "/private" + root: "/absolute/path/to/repo"},
        "policy": demo_policy,
    }

def finish(rec):
    if env.get("CAST_VALIDATE", "false") == "true":
        rec.validate()
    else:
        rec.generate()
