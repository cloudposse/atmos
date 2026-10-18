"""CI-only workaround for https://github.com/golang/go/issues/81238.

The 16 KiB reserve also raises fixedStack to 32 KiB. Go 1.26.9 only
shrinks a stack when used+stackNosplit is below one quarter of its old
size, leaving at least 16 KiB below the live frames after a shrink.
Remove this workaround when the supported Go release fixes the issue.
"""

import ctypes
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


VERSION = "go1.26.9"
SOURCE_SHA256 = "0225942ade2eed1e4ec5a0e813c66b3ed06c142c33c987b2fd9e031a0b871d3d"
OLD_RESERVE = b"goos.IsWindows*4096"
NEW_RESERVE = b"goos.IsWindows*16384"
RESERVE = 16384
DISPATCH_MARGIN = 2048


def patch_source(version, source):
    if version != VERSION:
        raise ValueError("Reassess the Windows exception workaround for this Go version")
    if hashlib.sha256(source).hexdigest() != SOURCE_SHA256:
        raise ValueError("Go runtime source does not match the validated release")
    if source.count(OLD_RESERVE) != 1:
        raise ValueError("Expected exactly one Windows exception reserve")
    return source.replace(OLD_RESERVE, NEW_RESERVE)


def overlay_flags(existing, overlay):
    path = overlay.as_posix()
    if "-overlay" in existing:
        raise ValueError("An existing Go overlay must be reconciled explicitly")
    if any(c in existing for c in "\r\n") or any(c in path for c in '\r\n"'):
        raise ValueError("Invalid Go environment or overlay path")
    return f'{existing} "-overlay={path}"'.strip()


def context_size():
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    initialize = kernel.InitializeContext
    initialize.argtypes = [ctypes.c_void_p, ctypes.c_uint32,
                           ctypes.POINTER(ctypes.c_void_p), ctypes.POINTER(ctypes.c_uint32)]
    initialize.restype = ctypes.c_int
    context = ctypes.c_void_p()
    size = ctypes.c_uint32()
    # AMD64 CONTEXT_ALL | CONTEXT_XSTATE, matching the hardware-exception frame.
    ok = initialize(None, 0x10005F, ctypes.byref(context), ctypes.byref(size))
    if ok or ctypes.get_last_error() != 122 or size.value == 0:
        raise ValueError("Unable to measure the Windows exception context")
    return size.value


def configure(go_env, size, temporary_root, environment_file):
    if (go_env["GOHOSTOS"], go_env["GOHOSTARCH"], go_env["GOOS"], go_env["GOARCH"]) != (
            "windows", "amd64", "windows", "amd64"):
        raise ValueError("This workaround was validated only for Windows/amd64")
    if size <= 0 or size + DISPATCH_MARGIN > RESERVE:
        raise ValueError("The measured exception context exceeds the validated reserve")
    source_path = Path(go_env["GOROOT"]) / "src" / "runtime" / "stack.go"
    source = patch_source(go_env["GOVERSION"], source_path.read_bytes())
    # Validate existing flags before creating files or changing the environment.
    overlay_flags(go_env["GOFLAGS"], temporary_root / "overlay.json")
    directory = Path(tempfile.mkdtemp(prefix="atmos-windows-runtime-", dir=temporary_root))
    replacement = directory / "stack.go"
    overlay = directory / "overlay.json"
    replacement.write_bytes(source)
    overlay.write_text(json.dumps({"Replace": {str(source_path): str(replacement)}}), encoding="utf-8")
    flags = overlay_flags(go_env["GOFLAGS"], overlay)
    with environment_file.open("a", encoding="utf-8") as output:
        output.write(f"GOFLAGS={flags}\n")
    print(f"Windows CI exception reserve: {RESERVE} bytes; context: {size} bytes; {VERSION}")
    print(f"Verified runtime source SHA256: {SOURCE_SHA256}")
    return overlay


def main():
    names = ["GOVERSION", "GOHOSTOS", "GOHOSTARCH", "GOOS", "GOARCH", "GOROOT", "GOFLAGS"]
    go_env = json.loads(subprocess.check_output(["go", "env", "-json", *names], text=True))
    configure(go_env, context_size(), Path(os.environ["RUNNER_TEMP"]), Path(os.environ["GITHUB_ENV"]))


if __name__ == "__main__":
    main()
