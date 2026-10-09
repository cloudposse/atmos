"""Route Floci's Lambda function URL through the emulator's published host port."""

import os
from pathlib import Path
import subprocess
from urllib.parse import urlsplit


root = Path(__file__).resolve().parents[3]
function_url = subprocess.check_output(
    ["atmos", "aws", "cfn", "output", "app", "FunctionUrl", "-s", "advanced"],
    cwd=root, text=True,
).strip()
endpoint = os.environ["AWS_ENDPOINT_URL"].rstrip("/")
hostname = urlsplit(function_url).hostname or ""
if ".lambda-url." not in hostname:
    raise SystemExit(f"Unexpected Lambda function URL: {function_url}")
url_id = hostname.split(".", 1)[0]
print(f"{endpoint}/lambda-url/{url_id}/", end="")
