"""A local, throttled HTTP source for a repeatable concurrent vendoring recording."""
import http.server
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

READY = Path("server.ready")
PID = Path("server.pid")


def start():
    READY.unlink(missing_ok=True)
    process = subprocess.Popen(
        [sys.executable, __file__, "serve"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    PID.write_text(str(process.pid))
    for _ in range(100):
        if READY.exists():
            return
        if process.poll() is not None:
            raise SystemExit("Local download fixture failed to start")
        time.sleep(0.05)
    process.terminate()
    raise SystemExit("Local download fixture did not become ready")


def stop():
    if PID.exists():
        try:
            os.kill(int(PID.read_text()), signal.SIGTERM)
        except ProcessLookupError:
            pass
        PID.unlink()
    READY.unlink(missing_ok=True)


class Handler(http.server.BaseHTTPRequestHandler):
    payload = b"# Example component source for the concurrent download demo.\n" * 8192

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", str(len(self.payload)))
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        if self.command == "HEAD":
            return
        try:
            for offset in range(0, len(self.payload), 16384):
                self.wfile.write(self.payload[offset:offset + 16384])
                self.wfile.flush()
                time.sleep(0.04)
        except (BrokenPipeError, ConnectionResetError):
            pass

    do_HEAD = do_GET

    def log_message(self, *_):
        pass


def serve():
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    names = ["vpc", "eks", "rds", "security-group", "kms", "s3", "iam", "dns"]
    sources = [
        {"component": name, "version": "1.0.0",
         "source": f"http://127.0.0.1:{server.server_port}/{name}.tf",
         "targets": [f"components/terraform/{name}"]}
        for name in names
    ]
    Path("vendor.yaml").write_text(json.dumps({
        "apiVersion": "atmos/v1", "kind": "AtmosVendorConfig",
        "metadata": {"name": "concurrent-vendoring"}, "spec": {"sources": sources},
    }, indent=2))
    READY.touch()
    server.serve_forever()


if __name__ == "__main__":
    {"start": start, "stop": stop, "serve": serve}[sys.argv[1]]()
