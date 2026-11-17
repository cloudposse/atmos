# Shared checks for the native Atmos test groups in atmos.yaml.
# Each exported check authenticates independently; no tokens are written to step outputs.
catalog = module.load("catalog.star")

def check(condition, message):
    if not condition:
        fail(message)

def pause():
    steps.sleep(timeout = "1s", output = "none")

def wait_entity(kind, name, token):
    for attempt in range(60):
        entity = catalog.request("/catalog/entities/by-name/" + kind + "/default/" + name, token = token, expected = [200, 404])
        if entity.get("kind"):
            return entity
        pause()
    fail("Catalog did not ingest " + kind + "/" + name)

def run_template(template, values, token):
    response = catalog.request("/scaffolder/v2/tasks", {"templateRef": "template:default/" + template, "values": values}, token, expected = [201])
    task_id = response["id"]
    for attempt in range(60):
        task = catalog.request("/scaffolder/v2/tasks/" + task_id, token = token)
        if task["status"] == "completed":
            events = catalog.request("/scaffolder/v2/tasks/" + task_id + "/events", token = token)
            for event in events:
                if event["type"] == "completion":
                    return event["body"]["output"]
            fail("Missing task output")
        check(task["status"] in ["open", "processing"], "Task " + task_id + " failed; inspect Backstage task logs")
        pause()
    fail("Task did not finish: " + task_id)

def wait_job(sha, job, git_api):
    for attempt in range(600):
        statuses = catalog.request("/commits/" + sha + "/statuses", base = git_api)
        matching = [status for status in statuses if "/ " + job + " " in status["context"]]
        if matching:
            status = sorted(matching, key = lambda item: item["id"])[-1]
            if status["status"] == "success":
                ui.success("PASS Gitea Actions " + job)
                return
            check(status["status"] not in ["failure", "error"], "Gitea " + job + " failed: " + status["target_url"])
        pause()
    fail("Gitea " + job + " did not complete; inspect Actions logs")


def catalog_entities():
    token = catalog.guest_token()
    wait_entity("template", "describe-atmos", token)
    wait_entity("template", "request-atmos-service", token)
    entity = wait_entity("component", "payments", token)
    check(entity["spec"]["owner"] == "platform-team", "Payments owner is missing")

def techdocs_metadata():
    token = catalog.guest_token()
    wait_entity("component", "payments", token)
    # This endpoint caught the invalid ../techdocs reference even when static HTML was 200.
    catalog.request("/techdocs/metadata/entity/default/component/payments", token = token)

def techdocs_content():
    token = catalog.guest_token()
    docs = steps.http(url = "http://localhost:7007/api/techdocs/static/docs/default/component/payments/index.html",
                      headers = {"Authorization": "Bearer " + token}, expect = {"status": [200]}, output = "none")
    check("Atmos service handbook" in docs.value, "TechDocs handbook missing")

def describe_stack(stack, replicas):
    token = catalog.guest_token()
    wait_entity("template", "describe-atmos", token)
    output = run_template("describe-atmos", {"stack": stack}, token)
    content = output["text"][0]["content"].strip().removeprefix("```json").removesuffix("```").strip()
    config = json.decode(content)
    check(config["vars"]["replicas"] == replicas, "Incorrect replica override for " + stack)
    check(config["vars"]["owner"] == "platform-team", "Missing inherited owner for " + stack)

def reject_unknown_stack():
    token = catalog.guest_token()
    wait_entity("template", "describe-atmos", token)
    catalog.request("/scaffolder/v2/tasks", {"templateRef": "template:default/describe-atmos", "values": {"stack": "not-allowed"}}, token, expected = [400])

def reject_invalid_service():
    token = catalog.guest_token()
    wait_entity("template", "request-atmos-service", token)
    for name in ["../escape", "Uppercase", "ab", "trailing-", "a" * 31]:
        catalog.request("/scaffolder/v2/tasks", {"templateRef": "template:default/request-atmos-service", "values": {"name": name}}, token, expected = [400])

def provision():
    token = catalog.guest_token()
    git_api = "http://localhost:3001/api/v1/repos/atmos/deployments"
    # A successful test changes main, so the next run gets a new name.
    head = catalog.request("/branches/main", base = git_api)["commit"]["id"]
    name = "smoke-" + head[:12]
    output = run_template("request-atmos-service", {"name": name}, token)
    pull_url = output["links"][0]["url"]
    pull_number = pull_url.rsplit("/", 1)[1]
    unmerged = atmos.portal("plan", flags = {"service": name}, check = False, output = "capture")
    check(unmerged.exit_code != 0, "An unmerged request must not be provisionable")
    ui.success("PASS unmerged request blocked")
    # Readiness of Gitea's mergeability calculation is asynchronous.
    for attempt in range(30):
        pull = catalog.request("/pulls/" + pull_number, base = git_api)
        if pull.get("mergeable"):
            break
        pause()
    check(pull.get("mergeable"), "Test pull request is not mergeable")
    wait_job(pull["head"]["sha"], "plan", git_api)
    # Throwaway emulator account atmos:atmos (base64), not a real credential.
    catalog.request("/pulls/" + pull_number + "/merge", {"Do": "merge"}, base = git_api,
            authorization = "Basic YXRtb3M6YXRtb3M=", expected = [200, 204])
    merged = catalog.request("/pulls/" + pull_number, base = git_api)
    wait_job(merged["merge_commit_sha"], "deploy", git_api)
    # Floci's local S3 endpoint accepts this probe without authentication.
    # Verify real emulator state, not only Terraform's recorded output.
    steps.http(url = "http://localhost:4567/atmos-backstage-dev-" + name,
               method = "HEAD", expect = {"status": [200]}, output = "none")
    entity = wait_entity("component", name, token)
    wait_entity("resource", name + "-bucket", token)
    check(any([r["type"] == "dependsOn" and r["targetRef"] == "resource:default/" + name + "-bucket" for r in entity.get("relations", [])]), "Missing catalog resource relation")
    ui.success("PASS Git PR -> merged Atmos stack -> local S3 bucket -> Git-backed catalog: " + name)
