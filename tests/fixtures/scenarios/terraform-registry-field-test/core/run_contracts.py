"""Generation, policy application, scheduling failures and shared-caller checks."""
import json
from pathlib import Path
import sys

import yaml

from run import Case
from run_extra import simple


def generation(c):
    dev = simple(c)
    doc = yaml.safe_load(dev.read_text())
    doc["components"]["terraform"]["producer"].update(backend_type="local", backend={"local": {}})
    dev.write_text(yaml.safe_dump(doc, sort_keys=False))
    main = c.project / "components/terraform/marker/main.tf"
    main.write_text(main.read_text().replace('backend "local" {}', ''))
    c.command("generate-varfile", ["terraform", "generate", "varfile", "producer", "-s", "dev", "-f", str(c.root / "vars.json")], [0])
    c.command("generate-backend", ["terraform", "generate", "backend", "producer", "-s", "dev"], [0])
    c.command("plan", ["terraform", "plan", "producer", "-s", "dev", "-detailed-exitcode"], [2])
    plan = c.project / "components/terraform/marker/dev-producer.planfile"
    original = c.root / "original.planfile"
    if plan.exists():
        original.write_bytes(plan.read_bytes())
    c.command("generate-plan-json", ["terraform", "generate", "planfile", "producer", "-s", "dev", "--file=" + str(plan)], [0])
    c.command("show-json", ["terraform", "show", "producer", "-s", "dev", "-json", str(original)], [0])
    c.command("plan-diff-same", ["terraform", "plan-diff", "producer", "-s", "dev", "--orig=" + str(original), "--new=" + str(original)], [0])
    dev.write_text(dev.read_text().replace("dev-producer-v1", "dev-producer-v2"))
    c.command("changed-plan", ["terraform", "plan", "producer", "-s", "dev"], [0])
    changed = c.root / "changed.planfile"
    if plan.exists():
        changed.write_bytes(plan.read_bytes())
    c.command("plan-diff-changed", ["terraform", "plan-diff", "producer", "-s", "dev", "--orig=" + str(original), "--new=" + str(changed)], [2])
    c.command("apply", ["terraform", "apply", "producer", "-s", "dev", "--from-plan=" + str(changed)], [0])
    c.command("destructive-plan", ["terraform", "plan", "producer", "-s", "dev", "-destroy"], [0])
    before = c.state_snapshot()
    c.command("deny-saved-destructive-apply", ["terraform", "apply", "producer", "-s", "dev", "--from-plan"], [77],
              env={"ATMOS_POLICY_DIR": str(c.project / "policy")})
    c.check("policy rejects saved destructive plan without state mutation", before == c.state_snapshot())
    c.command("skip-planfile", ["terraform", "plan", "producer", "-s", "dev", "--skip-planfile"], [0])
    c.save()


def failure(c):
    dev = simple(c)
    doc = yaml.safe_load(dev.read_text())
    doc["components"]["terraform"]["bad"] = {"metadata": {"component": "marker"}, "vars": {"name": "fail"}}
    dev.write_text(yaml.safe_dump(doc, sort_keys=False))
    main = c.project / "components/terraform/marker/main.tf"
    main.write_text(main.read_text().replace('resource "terraform_data" "marker" {', '''resource "terraform_data" "marker" {
  lifecycle {
    precondition {
      condition = var.name != "fail"
      error_message = "FIELD_TEST_EXPECTED_FAILURE"
    }
  }
'''))
    for mode in ("fail-fast", "keep-going"):
        c.command(mode, ["terraform", "plan", "--all", "-s", "dev", "--failure-mode=" + mode,
                         "--max-concurrency=1", "--execution-summary-file=" + str(c.root / (mode + ".json"))], [1])
    c.save()


def shared_callers(c):
    dev = simple(c)
    doc = yaml.safe_load(dev.read_text())
    doc["components"]["helmfile"] = {"smoke": {"vars": {"field_marker": "helmfile-scope"}}}
    doc["components"]["packer"] = {"smoke": {"vars": {"field_marker": "packer-scope"}}}
    dev.write_text(yaml.safe_dump(doc, sort_keys=False))
    config = yaml.safe_load((c.project / "atmos.yaml").read_text())
    config["components"]["helmfile"] = {"base_path": "components/helmfile", "use_eks": False}
    stub = c.root / "packer-controlled"
    stub.write_text("#!" + sys.executable + "\nimport json,os,sys\nprint(json.dumps({'argv':sys.argv[1:],'cwd':os.getcwd()}))\n")
    stub.chmod(0o755)
    config["components"]["packer"] = {"base_path": "components/packer", "command": str(stub)}
    (c.project / "atmos.yaml").write_text(yaml.safe_dump(config, sort_keys=False))
    helm = c.project / "components/helmfile/smoke"
    helm.mkdir(parents=True)
    (helm / "helmfile.yaml.gotmpl").write_text('releases:\n  - name: smoke\n    namespace: field-test\n    chart: ./chart\n    values:\n      - marker: {{ .Values.field_marker | quote }}\n')
    chart = helm / "chart"
    (chart / "templates").mkdir(parents=True)
    (chart / "Chart.yaml").write_text("apiVersion: v2\nname: field-test\nversion: 0.1.0\n")
    (chart / "templates/marker.yaml").write_text('apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: field-test\ndata:\n  marker: {{ .Values.marker | quote }}\n')
    packer = c.project / "components/packer/smoke"
    packer.mkdir(parents=True)
    (packer / "main.pkr.hcl").write_text("variable \"field_marker\" { type = string }\n")
    c.command("helmfile-template", ["helmfile", "template", "smoke", "-s", "dev"], [0])
    c.command("packer-controlled-validate", ["packer", "validate", "smoke", "-s", "dev"], [0])
    c.save()


def stored_plan(c):
    dev = simple(c)
    config = yaml.safe_load((c.project / "atmos.yaml").read_text())
    config["components"]["terraform"]["planfiles"] = {
        "default": "local", "priority": ["local"], "verify": "fail",
        "stores": {"local": {"type": "local/dir", "options": {"path": str(c.root / "artifacts")}}}}
    config["ci"] = {"enabled": True, "checks": {"enabled": False}, "comments": {"enabled": False}}
    (c.project / "atmos.yaml").write_text(yaml.safe_dump(config, sort_keys=False))
    c.command("reviewed-plan", ["terraform", "plan", "producer", "-s", "dev"], [0])
    c.command("upload-reviewed-plan", ["terraform", "planfile", "upload", "producer", "-s", "dev", "--store=local"], [0])
    dev.write_text(dev.read_text().replace("dev-producer-v1", "dev-producer-v2"))
    before = c.state_snapshot()
    c.command("reject-drift", ["terraform", "deploy", "producer", "-s", "dev", "--verify-plan", "--ci"], [1])
    c.check("drift rejection preserves state", c.state_snapshot() == before)
    dev.write_text(dev.read_text().replace("dev-producer-v2", "dev-producer-v1"))
    c.command("deploy-reviewed-plan", ["terraform", "deploy", "producer", "-s", "dev", "--verify-plan", "--ci"], [0])
    c.output("verified-output")
    c.save()


if __name__ == "__main__":
    root, baseline, candidate = sys.argv[1:]
    root = Path(root)
    for engine in ("terraform", "tofu"):
        for revision, binary in (("baseline", baseline), ("candidate", candidate)):
            for name, operation in (("generation", generation), ("failure", failure), ("stored-plan", stored_plan)):
                operation(Case(root / f"{engine}-{revision}-{name}", revision, engine, binary))
    for revision, binary in (("baseline", baseline), ("candidate", candidate)):
        shared_callers(Case(root / f"{revision}-shared", revision, "terraform", binary))
