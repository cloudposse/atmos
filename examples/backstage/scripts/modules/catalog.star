# Native step-library HTTP requests return structured results and enforce status codes.
def request(path, body = None, token = None, expected = [200], base = "http://localhost:7007/api", authorization = None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    elif authorization:
        headers["Authorization"] = authorization
    result = steps.http(
        url = base + path,
        method = "GET" if body == None else "POST",
        headers = headers,
        body = "" if body == None else json.encode(body),
        expect = {"status": expected},
        timeout = "15s",
        output = "none",
    )
    return result.data if result.value else None

def guest_token():
    return request("/auth/guest/refresh")["backstageIdentity"]["token"]

def register_service(name):
    token = guest_token()
    # The backend reads raw YAML over the shared Atmos network. Browser-facing links
    # in the entity point at localhost:3001 instead of this internal container alias.
    target = "http://local-backstage-git:3000/atmos/deployments/raw/branch/main/catalog/" + name + ".yaml"
    request("/catalog/locations", {"type": "url", "target": target}, token, expected = [201, 409])
    ui.info("Waiting for Backstage to ingest " + name)
    steps.http(
        url = "http://localhost:7007/api/catalog/entities/by-name/component/default/" + name,
        headers = {"Authorization": "Bearer " + token},
        expect = {"status": [200]},
        retry = {"max_attempts": 60, "initial_delay": "2s", "backoff_strategy": "constant"},
        timeout = "5s",
        output = "none",
    )
    request("/catalog/refresh", {"entityRef": "component:default/" + name}, token, expected = [204])
    ui.success("Provisioned local bucket and registered `http://localhost:3000/catalog/default/component/" + name + "`")
