"""Restore this job's service containers after runner hardening restarts Docker."""

import json
import os
import subprocess
import time


def docker(*args):
    """Bound every Docker operation, including diagnostics."""
    return subprocess.check_output(["docker", *args], text=True, timeout=30)


def diagnose(container):
    """Keep the original failure even when the Docker daemon is unavailable."""
    for args in [("inspect", container), ("logs", "--tail", "100", container)]:
        try:
            print(docker(*args), flush=True)
        except (OSError, subprocess.SubprocessError) as error:
            print(f"Container diagnostics failed: {error}", flush=True)


def restore(job_services):
    """Start only declared stopped services, then require their health checks."""
    for name, service in job_services.items():
        container = service["id"]
        try:
            state = json.loads(docker("inspect", container))[0]["State"]
            if not state["Running"]:
                print(f"Restoring service {name} after Docker restart", flush=True)
                docker("start", container)
                state = json.loads(docker("inspect", container))[0]["State"]
            deadline = time.monotonic() + 120
            while not (
                state["Running"]
                and state.get("Health", {}).get("Status", "healthy") == "healthy"
            ):
                if time.monotonic() >= deadline:
                    raise RuntimeError(
                        f"Service {name} did not become healthy within 120 seconds"
                    )
                time.sleep(2)
                state = json.loads(docker("inspect", container))[0]["State"]
            print(f"Service {name} is ready", flush=True)
        except Exception:
            diagnose(container)
            raise


if __name__ == "__main__":
    restore(json.loads(os.environ["JOB_SERVICES"]))
