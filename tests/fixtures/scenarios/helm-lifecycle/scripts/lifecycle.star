"""Assertions for the native Helm lifecycle scenario; no shell parsing."""

def cli(args, check = True):
    result = exec.run(
        [env["ATMOS_CLI_PATH"]] + args,
        env = {"NO_COLOR": "1"},
        check = False,
        output = "capture",
    )
    if check and result.exit_code != 0:
        # Include the command's diagnostic, never a secret-bearing stdout payload.
        fail("atmos {} failed (exit {}): {}".format(" ".join(args[:2]), result.exit_code, result.stderr.strip()))
    return result

def kubectl(args):
    return cli(["emulator", "exec", "kubernetes", "-s", "dev", "--", "kubectl"] + args)

def resource(kind, name, namespace = ""):
    scope = ["-n", namespace] if namespace else []
    return json.decode(kubectl(scope + ["get", kind, name, "-o", "json"]).stdout)

def assert_absent(kind, name, namespace = ""):
    scope = ["-n", namespace] if namespace else []
    # Only a missing object becomes empty output. Forbidden, connection failures,
    # unknown resource kinds and other errors still fail the checked command.
    result = kubectl(scope + ["get", kind, name, "--ignore-not-found", "-o", "json"])
    if result.stdout.strip():
        fail("expected {}/{} to be absent in {}".format(kind, name, namespace or "cluster scope"))

def assert_release_records(present, namespace = ""):
    scope = ["-n", namespace] if namespace else ["--all-namespaces"]
    records = json.decode(kubectl(["get", "secrets"] + scope + ["-l", "owner=helm,name=demo", "-o", "json"]).stdout)
    if bool(records["items"]) != present:
        fail("Helm release records: expected present={}".format(present))

def assert_crd_group():
    crd = resource("crd", "widgets.lifecycle.atmos.test")
    if crd["spec"]["group"] != "lifecycle.atmos.test":
        fail("create_crds did not install the expected CRD group")

def condition_true(obj, condition_type):
    return any([
        condition.get("status") == "True"
        for condition in obj.get("status", {}).get("conditions", [])
        if condition.get("type") == condition_type
    ])

def assert_job_complete():
    if not condition_true(resource("job", "demo-jobs-job", "demo-jobs"), "Complete"):
        fail("wait_for_jobs returned before demo-jobs-job completed")

def assert_hook_only():
    if condition_true(resource("deployment", "demo-hook-only", "demo-hook-only"), "Available"):
        fail("hookOnly unexpectedly satisfied the Deployment readiness gate")

def failure_text(component):
    result = cli(["helm", "apply", component, "-s", "dev", "--identity", "local-k3s"], check = False)
    if result.exit_code == 0:
        fail("{} unexpectedly succeeded".format(component))
    text = regex.replace(r"\x1b\[[0-9;]*m", "", result.stdout + "\n" + result.stderr)
    if "failed to perform helm release operation" not in text:
        fail("{} failed outside the Helm lifecycle operation: {}".format(component, text))
    return text

def assert_timeout():
    text = failure_text("demo-timeout")
    if not regex.search(r"(?s)context.*deadline exceeded|timed out waiting for condition", text):
        fail("timed release did not report a lifecycle timeout: " + text)

def hook_failed(text, job):
    return (
        "job {} failed: BackoffLimitExceeded".format(job) in text or
        "job failed job={} reason=BackoffLimitExceeded".format(job) in text
    )

def assert_failed_install():
    text = failure_text("demo-install-fail")
    if not hook_failed(text, "demo-install-fail-failing-hook"):
        fail("failed install did not report the expected hook failure: " + text)

def deployment_state():
    deployment = resource("deployment", "demo", "demo")
    container = deployment["spec"]["template"]["spec"]["containers"][0]
    return {
        "replicas": deployment["spec"]["replicas"],
        "image": container["image"],
        "port": container["ports"][0]["containerPort"],
    }

def assert_failed_upgrade():
    before = deployment_state()
    text = failure_text("demo-upgrade-fail")
    # Preserve the fixture's macOS nested-k3s fallback, but still verify rollback.
    hook_failure = hook_failed(text, "demo-failing-hook") or regex.search(
        r"(?s)job demo-failing-hook failed.*BackoffLimitExceeded", text,
    )
    timeout = regex.search(r"(?s)context.*deadline exceeded", text)
    if not hook_failure and not (env["RUNNER_OS"] == "macOS" and timeout):
        fail("failed upgrade did not report the expected post-upgrade hook failure: " + text)
    after = deployment_state()
    if before != after:
        fail("rollback did not restore Deployment/demo: before={} after={}".format(before, after))

def assert_dependency_ready():
    observed = resource("configmap", "dag-dependent-dependency-observed", "lifecycle-dag")
    if observed["data"].get("ready") != "true":
        fail("dependent did not observe the ready foundation Deployment")

def assert_release_progress(operation, component, message):
    result = cli(["helm", operation, component, "-s", "dev", "--identity", "local-k3s"])
    text = regex.replace(r"\x1b\[[0-9;]*m", "", result.stdout + "\n" + result.stderr)
    start = 0
    for needle in [message, "oss-ingress", "helm-oss"]:
        position = text.find(needle, start)
        if position < 0:
            fail("{} did not report release progress: {}".format(operation, text))
        start = position + len(needle)

def assert_admission_hooks():
    webhook = resource("validatingwebhookconfiguration", "oss-ingress-admission")
    if not webhook["webhooks"][0]["clientConfig"].get("caBundle"):
        fail("ingress-nginx post-install hook did not populate the webhook CA")
    jobs = json.decode(kubectl(["-n", "helm-oss", "get", "jobs", "-l", "app.kubernetes.io/instance=oss-ingress", "-o", "json"]).stdout)
    if jobs["items"]:
        fail("ingress-nginx hook-succeeded policy left Jobs behind")

def ingress_state():
    deployment = resource("deployment", "oss-ingress-controller", "helm-oss")
    container = deployment["spec"]["template"]["spec"]["containers"][0]
    values = {item["name"]: item.get("value") for item in container.get("env", [])}
    return deployment["spec"]["replicas"], values

def assert_deployed_secrets():
    _, values = ingress_state()
    expected_key = "\n".join([
        "-----BEGIN PRIVATE KEY-----", "ATMOS-OSS-LINE-A", "ATMOS-OSS-LINE-B", "-----END PRIVATE KEY-----",
    ])
    if values.get("ATMOS_SINGLE_LINE_SECRET") != "atmos-oss-token-ABCD1234":
        fail("single-line secret differs from the fixture value")
    if values.get("ATMOS_MULTILINE_SECRET") != expected_key:
        fail("multiline secret differs from the fixture value")

def assert_ingress_upgrade():
    replicas, values = ingress_state()
    if replicas != 2 or values.get("ATMOS_UPGRADE_MARKER") != "upgraded":
        fail("ingress upgrade did not apply the expected replicas and marker")
