"""Exercise the action with real tar archives and a local fake Helm executable."""

import concurrent.futures
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("helm-diff.sh").resolve()
VERSION = "v3.15.10"
FAKE_ATMOS = r'''#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == "helm plugin install diff@v3.15.10" ]]
echo install >> "$TEST_LOG"
if [[ "${FAIL_INSTALL:-false}" == true ]]; then exit 42; fi
plugins="$ATMOS_XDG_CACHE_HOME/atmos/toolchain/helm-plugins"
mkdir -p "$plugins/helm-diff/bin" "$plugins/helm-diff/.git"
echo metadata > "$plugins/helm-diff/.git/config"
printf '#!/usr/bin/env bash\necho "%s"\n' "${INSTALL_VERSION:-v3.15.10}" > "$plugins/helm-diff/bin/diff"
chmod +x "$plugins/helm-diff/bin/diff"
'''
FAKE_HELM = r'''#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == "diff version" ]]
"$HELM_PLUGINS/helm-diff/bin/diff"
'''


class HelmDiffTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="helm diff tests ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        for name, contents in {
            "helm": FAKE_HELM,
            "atmos": FAKE_ATMOS,
        }.items():
            executable = self.bin / name
            executable.write_text(contents)
            executable.chmod(0o755)
        self.archive = self.root / "artifact" / "plugin.tar.gz"
        self.log = self.root / "installs"
        self.github_env = self.root / "github-env"
        self.env = {
            **os.environ,
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "RUNNER_TEMP": str(self.root),
            "GITHUB_ENV": str(self.github_env),
            "TEST_LOG": str(self.log),
            "INSTALL_VERSION": VERSION,
        }

    def run_action(self, mode, **env):
        return subprocess.run(
            ["bash", str(SCRIPT), mode, VERSION, str(self.archive)],
            env={**self.env, **env},
            capture_output=True,
            text=True,
            timeout=20,
            check=False,
        )

    def assert_success(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_prepare_restore_preserves_executable_without_installing(self):
        self.assert_success(self.run_action("prepare"))
        self.assertEqual(list(self.root.glob("helm-diff.*")), [])
        with tarfile.open(self.archive) as archive:
            self.assertFalse(any(".git" in Path(name).parts for name in archive.getnames()))
        self.assert_success(self.run_action("restore"))
        plugins = Path(self.github_env.read_text().strip().split("=", 1)[1])
        self.assertTrue(os.access(plugins / "helm-diff/bin/diff", os.X_OK))
        self.assertEqual(self.log.read_text().splitlines(), ["install"])

    def test_atmos_failure_is_propagated_without_shell_retries(self):
        result = self.run_action("prepare", FAIL_INSTALL="true")
        self.assertEqual(result.returncode, 42)
        self.assertEqual(self.log.read_text().splitlines(), ["install"])
        self.assertFalse(self.archive.exists())
        self.assertEqual(list(self.root.glob("helm-diff.*")), [])

    def test_wrong_version_is_not_published(self):
        result = self.run_action("prepare", INSTALL_VERSION="v0.0.0")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("version mismatch", result.stderr)
        self.assertFalse(self.archive.exists())
        self.assertEqual(list(self.root.glob("helm-diff.*")), [])

    def test_restore_rejects_wrong_version(self):
        self.assert_success(self.run_action("prepare"))
        result = subprocess.run(
            ["bash", str(SCRIPT), "restore", "v0.0.0", str(self.archive)],
            env=self.env, capture_output=True, text=True, timeout=20, check=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("version mismatch", result.stderr)
        self.assertFalse(self.github_env.exists())
        self.assertEqual(list(self.root.glob("helm-diff.*")), [])

    def test_corrupt_archive_fails_without_install_fallback(self):
        self.archive.parent.mkdir()
        self.archive.write_text("not a tarball")
        self.assertNotEqual(self.run_action("restore").returncode, 0)
        self.assertFalse(self.log.exists())
        self.assertFalse(self.github_env.exists())
        self.assertEqual(list(self.root.glob("helm-diff.*")), [])

    def test_parallel_consumers_have_independent_copies(self):
        self.assert_success(self.run_action("prepare"))

        def restore(index):
            env_file = self.root / f"github-env-{index}"
            self.assert_success(self.run_action("restore", GITHUB_ENV=str(env_file)))
            return Path(env_file.read_text().strip().split("=", 1)[1])

        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            paths = list(pool.map(restore, range(4)))
        self.assertEqual(len(set(paths)), 4)
        (paths[0] / "helm-diff/bin/diff").unlink()
        for plugins in paths[1:]:
            self.assertTrue(os.access(plugins / "helm-diff/bin/diff", os.X_OK))
        self.assertEqual(self.log.read_text().splitlines(), ["install"])

    def test_unknown_mode_fails_before_installing(self):
        self.assertNotEqual(self.run_action("invalid").returncode, 0)
        self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
