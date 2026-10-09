"""Build a self-contained Lambda package using only the Python standard library."""

import os
from pathlib import Path
import re
import shutil


release = os.environ.get("ATMOS_DEMO_RELEASE", "v1")
if not re.fullmatch(r"[a-zA-Z0-9_-]+", release):
    raise SystemExit("ATMOS_DEMO_RELEASE must contain letters, digits, '_' or '-'")

package = Path(".build/package")
shutil.rmtree(package, ignore_errors=True)
package.mkdir(parents=True)
shutil.copyfile("src/handler.py", package / "handler.py")
(package / "release.py").write_text(f"RELEASE = {release!r}\n", encoding="utf-8")
