"""Behavioral tests for release planning, reconciliation, and anonymous verification."""
import copy
import hashlib
import json
import subprocess
import unittest
import urllib.error
from unittest.mock import patch, MagicMock

import catalog
import plan
import public


class Planning(unittest.TestCase):
    def setUp(self):
        self.config = {"path": "plugins/auth-secrets", "inputs": ["sdk/**", "proto/**"]}
        self.manifest = {"metadata": {"name": "cloudposse/auth-secrets", "version": "1.2.0"}, "spec": {"platforms": {key: {"executable": "bin/" + key.replace("/", "-") + "/plugin"} for key in plan.RUNNERS}}}
        self.old = copy.deepcopy(self.manifest)
        self.old["metadata"]["version"] = "1.1.0"
        self.changed = ["plugins/auth-secrets/auth.go"]
        self.subsequent = []
        self.exists = True

    def document(self, ref, path):
        if path == "plugins/releases.yaml":
            return {"packages": {"auth-secrets": self.config}}
        return self.old if ref == "base" else self.manifest

    def git(self, *args):
        if args[0] == "rev-parse":
            return "head"
        if args[0] == "diff":
            return "\n".join(self.changed if args[2] == "base" else self.subsequent)
        if args[0] == "log":
            return "accepted-commit"
        if args[0] == "ls-tree":
            return "plugins/auth-secrets/plugin.yaml" if self.exists else ""
        self.fail(args)

    def run_plan(self, base="base"):
        with patch.object(plan, "document", side_effect=self.document), patch.object(plan, "git", side_effect=self.git):
            return plan.plan("HEAD", base)

    def test_independent_manifest_version_and_exact_source(self):
        result = self.run_plan()
        self.assertEqual(6, len(result["build"]["include"]))
        entry = result["packages"]["include"][0]
        self.assertEqual("1.2.0", entry["version"])
        self.assertEqual("accepted-commit", entry["source"])
        self.assertEqual(plan.NAMESPACE + "auth-secrets", entry["repository"])
        self.exists = False
        self.run_plan()
        self.run_plan(None)

    def test_shared_inputs_require_bump_and_unrelated_changes_do_not(self):
        self.old = copy.deepcopy(self.manifest)
        for changed in ["sdk/api/client.go", "proto/atmos/api/v1/api.proto", "plugins/auth-secrets/main.go", "plugins/releases.yaml"]:
            self.changed = [changed]
            with self.assertRaisesRegex(ValueError, "require.*increase"):
                self.run_plan()
        self.changed = ["website/docs/hello.mdx"]
        self.run_plan()
        self.manifest["metadata"]["version"] = "1.0.0"
        with self.assertRaisesRegex(ValueError, "decrease"):
            self.run_plan()

    def test_reconciliation_rejects_changed_inputs_after_version(self):
        self.subsequent = ["sdk/api/client.go"]
        with self.assertRaisesRegex(ValueError, "after its version"):
            self.run_plan(None)

    def test_rejects_invalid_inventory_and_incomplete_platforms(self):
        self.config["path"] = "../outside"
        with self.assertRaises(ValueError): self.run_plan()
        self.config["path"] = "plugins/auth-secrets"
        self.config["inputs"] = ["../outside"]
        with self.assertRaises(ValueError): self.run_plan()
        self.config["inputs"] = []
        self.manifest["metadata"]["name"] = "different"
        with self.assertRaises(ValueError): self.run_plan()
        self.manifest["metadata"]["name"] = "cloudposse/auth-secrets"
        self.manifest["spec"]["platforms"]["linux/amd64"]["executable"] = "../plugin"
        with self.assertRaises(ValueError): self.run_plan()
        del self.manifest["spec"]["platforms"]["linux/amd64"]
        with self.assertRaises(ValueError): self.run_plan()
        with patch.object(plan, "git", return_value="head"), patch.object(plan, "document", return_value={"packages": {}}):
            with self.assertRaisesRegex(ValueError, "empty"):
                plan.plan("HEAD")
        for invalid in ["v1.0.0", "1.0", "1.0.0-rc1"]:
            self.manifest["metadata"]["version"] = invalid
            with self.assertRaises(ValueError): plan.version(self.manifest)
        for invalid in ["/root", "../root", "foo/../root", "a\\b", None, ""]:
            self.assertFalse(plan.safe_path(invalid))

    def test_git_and_yaml_adapters(self):
        with patch.object(subprocess, "check_output", return_value=" result \n") as run:
            self.assertEqual("result", plan.git("status"))
            run.assert_called_once_with(["git", "status"], text=True)
        with patch.object(plan, "git", return_value="metadata: {version: 1.0.0}"):
            self.assertEqual("1.0.0", plan.document("head", "plugin.yaml")["metadata"]["version"])


class PublicAccess(unittest.TestCase):
    source = plan.NAMESPACE + "auth-secrets"

    def test_visibility_fails_closed_and_uses_encoded_package_path(self):
        with patch.object(subprocess, "check_output", return_value='{"visibility":"public"}') as run:
            public.require_public(self.source)
            self.assertEqual("orgs/cloudposse/packages/container/atmos%2Fplugins%2Fauth-secrets", run.call_args.args[0][-1])
        for visibility in ["private", "internal", None]:
            with patch.object(subprocess, "check_output", return_value=json.dumps({"visibility": visibility})):
                with self.assertRaisesRegex(RuntimeError, "must be public"):
                    public.require_public(self.source)
        with patch.object(subprocess, "check_output", side_effect=subprocess.CalledProcessError(1, "gh")):
            with self.assertRaisesRegex(RuntimeError, "Provision"):
                public.require_public(self.source)
        for source in ["oci://ghcr.io/other/plugin", self.source + "/nested", self.source + ":1.0.0"]:
            with self.assertRaises(ValueError): public.repository(source)

    def objects(self):
        objects = {}
        def add(value):
            data = value if isinstance(value, bytes) else json.dumps(value).encode()
            digest = "sha256:" + hashlib.sha256(data).hexdigest()
            objects[digest] = data
            return {"digest": digest}
        blob = add(b"plugin bytes")
        manifest = add({"config": blob, "layers": [blob]})
        index = {"manifests": [manifest, manifest]}
        objects["1.0.0"] = json.dumps(index).encode()
        return objects

    def test_anonymous_pull_verifies_index_manifests_and_blobs(self):
        objects = self.objects()
        seen = []
        def fetch(url, headers=None, limit=None):
            if "/token?" in url:
                self.assertIsNone(headers)
                return b'{"token":"anonymous"}'
            self.assertEqual("Bearer anonymous", headers["Authorization"])
            ref = url.rsplit("/", 1)[-1]
            seen.append(ref)
            return objects[ref]
        with patch.object(public, "fetch", side_effect=fetch):
            digest = public.anonymous_pull(self.source, "1.0.0")
            self.assertEqual("sha256:" + hashlib.sha256(objects["1.0.0"]).hexdigest(), digest)
            self.assertEqual(3, len(seen))
            with self.assertRaisesRegex(ValueError, "mismatch"):
                public.anonymous_pull(self.source, "1.0.0", "sha256:" + "0" * 64)
            with self.assertRaisesRegex(ValueError, "Unsupported"):
                public.anonymous_pull(self.source, "1.0.0", "md5:bad")
        with self.assertRaisesRegex(ValueError, "reference"):
            public.anonymous_pull(self.source, "../bad")
        objects["1.0.0"] = json.dumps({"manifests": [{"digest": "sha256:" + f"{index:064x}"} for index in range(130)]}).encode()
        def oversized(url, headers=None, limit=None):
            if "/token?" in url: return b'{"token":"anonymous"}'
            if url.endswith("1.0.0"): return objects["1.0.0"]
            return b'{}'
        # Each declared digest is still checked before following another object.
        with patch.object(public, "fetch", side_effect=oversized):
            with self.assertRaisesRegex(ValueError, "mismatch"):
                public.anonymous_pull(self.source, "1.0.0")

    def test_bounded_fetch(self):
        response = MagicMock()
        response.__enter__.return_value.read.return_value = b"payload"
        with patch.object(public.urllib.request, "urlopen", return_value=response) as call:
            self.assertEqual(b"payload", public.fetch("https://ghcr.io/test"))
            self.assertEqual(60, call.call_args.kwargs["timeout"])
            with self.assertRaisesRegex(ValueError, "limit"):
                public.fetch("https://ghcr.io/test", limit=2)


class Reconciliation(unittest.TestCase):
    def test_latest_accepted_manifest_read_under_lock_and_digest_pinned_update(self):
        package = {"name": "auth-secrets", "repository": plan.NAMESPACE + "auth-secrets", "version": "1.0.0"}
        with patch.object(catalog.subprocess, "run") as run, patch.object(catalog, "plan", return_value={"packages": {"include": [package]}}) as planner, patch.object(catalog, "require_public") as gate, patch.object(catalog, "anonymous_pull", return_value="sha256:digest") as pull:
            result = catalog.reconcile("/tmp/atmos")
            self.assertEqual(["git", "fetch", "origin", "main"], run.call_args_list[0].args[0])
            planner.assert_called_once_with("origin/main")
            self.assertEqual(package["repository"] + "@sha256:digest", run.call_args_list[1].args[0][-1])
            self.assertEqual("1.0.0", result[0]["version"])
            self.assertEqual(2, gate.call_count)
            self.assertEqual("stable", pull.call_args.args[1])

    def test_missing_release_is_deferred_but_permission_failure_is_fatal(self):
        package = {"name": "auth-secrets", "repository": plan.NAMESPACE + "auth-secrets", "version": "1.0.0"}
        for code in [404, 403]:
            with patch.object(catalog.subprocess, "run"), patch.object(catalog, "plan", return_value={"packages": {"include": [package]}}), patch.object(catalog, "require_public"), patch.object(catalog, "anonymous_pull", side_effect=urllib.error.HTTPError("url", code, "missing", {}, None)):
                with self.assertRaises(RuntimeError if code == 404 else urllib.error.HTTPError):
                    catalog.reconcile("atmos")

class Entrypoints(unittest.TestCase):
    def test_plan_command_outputs_json(self):
        import runpy
        fixture = Planning()
        fixture.setUp()
        def output(args, **kwargs):
            if args[1] == "show":
                ref, path = args[2].split(":", 1)
                return json.dumps(fixture.document(ref, path))
            return fixture.git(*args[1:])
        with patch("sys.argv", ["plan.py", "--base", "base"]), patch.object(subprocess, "check_output", side_effect=output), patch("builtins.print") as printer:
            runpy.run_module("plan", run_name="__main__")
            self.assertEqual(6, len(json.loads(printer.call_args.args[0])["build"]["include"]))

    def test_public_command_requires_visibility(self):
        import runpy
        with patch("sys.argv", ["public.py", PublicAccess.source]), patch.object(subprocess, "check_output", return_value='{"visibility":"public"}') as run:
            runpy.run_module("public", run_name="__main__")
            self.assertEqual(1, run.call_count)


if __name__ == "__main__":
    unittest.main()
