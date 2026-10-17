"""Exercise recovery without requiring a Docker daemon."""

import contextlib
import importlib.util
import io
import json
import pathlib
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    "restore_services", pathlib.Path(__file__).with_name("restore_services.py")
)
services = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(services)


class RestoreServicesTest(unittest.TestCase):
    def test_healthy_container_is_not_restarted(self):
        with patch.object(services, "docker") as docker:
            docker.return_value = (
                '[{"State":{"Running":true,"Health":{"Status":"healthy"}}}]'
            )
            services.restore({"floci": {"id": "container-id"}})
            docker.assert_called_once_with("inspect", "container-id")

    def test_stopped_container_is_started_and_waited_for(self):
        with patch.object(services, "docker") as docker, patch.object(
            services.time, "sleep"
        ):
            docker.side_effect = [
                '[{"State":{"Running":false}}]',
                "",
                '[{"State":{"Running":true,"Health":{"Status":"starting"}}}]',
                '[{"State":{"Running":true,"Health":{"Status":"healthy"}}}]',
            ]
            services.restore({"floci": {"id": "container-id"}})
            self.assertEqual(docker.call_args_list[1].args, ("start", "container-id"))
            self.assertEqual(docker.call_count, 4)

    def test_running_service_without_healthcheck_fails_with_diagnostics(self):
        with patch.object(
            services, "docker", return_value='[{"State":{"Running":true}}]'
        ), patch.object(services, "diagnose") as diagnose:
            with self.assertRaisesRegex(RuntimeError, "registry.*health check"):
                services.restore({"registry": {"id": "container-id"}})
            diagnose.assert_called_once_with("container-id")

    def test_healthcheck_alone_does_not_make_stopped_service_ready(self):
        with patch.object(services, "docker") as docker, patch.object(
            services.time, "sleep"
        ) as sleep:
            docker.side_effect = [
                '[{"State":{"Running":false,"Health":{"Status":"healthy"}}}]',
                "",
                '[{"State":{"Running":false,"Health":{"Status":"healthy"}}}]',
                '[{"State":{"Running":true,"Health":{"Status":"healthy"}}}]',
            ]
            services.restore({"registry": {"id": "container-id"}})
            sleep.assert_called_once_with(2)
            self.assertEqual(docker.call_count, 4)

    def test_timeout_reports_container_diagnostics(self):
        with patch.object(
            services,
            "docker",
            return_value='[{"State":{"Running":true,"Health":{"Status":"unhealthy"}}}]',
        ), patch.object(services.time, "monotonic", side_effect=[0, 121]), patch.object(
            services, "diagnose"
        ) as diagnose:
            with self.assertRaisesRegex(RuntimeError, "floci.*healthy"):
                services.restore({"floci": {"id": "container-id"}})
            diagnose.assert_called_once_with("container-id")

    def test_start_failure_is_not_hidden(self):
        with patch.object(
            services,
            "docker",
            side_effect=['[{"State":{"Running":false}}]', RuntimeError("start failed")],
        ), patch.object(services, "diagnose") as diagnose:
            with self.assertRaisesRegex(RuntimeError, "start failed"):
                services.restore({"floci": {"id": "container-id"}})
            diagnose.assert_called_once_with("container-id")

    def test_empty_services_do_not_touch_docker(self):
        with patch.object(services, "docker") as docker:
            services.restore({})
            docker.assert_not_called()

    def test_diagnostics_preserve_logs_when_inspect_fails(self):
        with patch.object(
            services,
            "docker",
            side_effect=[OSError("daemon unavailable"), "service logs"],
        ) as docker:
            services.diagnose("container-id")
            self.assertEqual(
                docker.call_args_list[-1].args,
                ("logs", "--tail", "100", "container-id"),
            )

    def test_diagnostics_exclude_credentials_and_health_output(self):
        inspected = [
            {
                "Config": {"Env": ["TOKEN=generated-secret"]},
                "State": {
                    "Status": "exited",
                    "Running": False,
                    "ExitCode": 1,
                    "Health": {
                        "Status": "unhealthy",
                        "Log": [{"Output": "health-secret"}],
                    },
                },
            }
        ]
        output = io.StringIO()
        with patch.object(
            services, "docker", side_effect=[json.dumps(inspected), "service logs"]
        ), contextlib.redirect_stdout(output):
            services.diagnose("container-id")
        rendered = output.getvalue()
        self.assertNotIn("generated-secret", rendered)
        self.assertNotIn("health-secret", rendered)
        self.assertEqual(
            json.loads(rendered.splitlines()[0]),
            {"Status": "exited", "Running": False, "ExitCode": 1, "Health": "unhealthy"},
        )
        self.assertIn("service logs", rendered)

    def test_malformed_inspect_still_collects_logs(self):
        output = io.StringIO()
        with patch.object(
            services, "docker", side_effect=["invalid json", "service logs"]
        ), contextlib.redirect_stdout(output):
            services.diagnose("container-id")
        self.assertIn("Container state diagnostics failed", output.getvalue())
        self.assertIn("service logs", output.getvalue())

    def test_docker_calls_are_bounded(self):
        with patch.object(
            services.subprocess, "check_output", return_value="state"
        ) as call:
            self.assertEqual(services.docker("inspect", "container-id"), "state")
            call.assert_called_once_with(
                ["docker", "inspect", "container-id"], text=True, timeout=30
            )


if __name__ == "__main__":
    unittest.main()
