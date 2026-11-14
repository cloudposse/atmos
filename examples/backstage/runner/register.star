def main():
    if fs.exists("/runner-data/.runner"):
        return
    response = steps.http(
        url = "http://local-backstage-git:3000/api/v1/repos/atmos/deployments/actions/runners/registration-token",
        headers = {"Authorization": "Basic YXRtb3M6YXRtb3M="},
        expect = {"status": [200]}, output = "none",
    )
    exec.run(["act_runner", "register", "--no-interactive", "--config", "/opt/runner/config.yaml",
              "--instance", "http://local-backstage-git:3000", "--token", response.data["token"],
              "--name", "atmos-backstage", "--labels", "atmos-local:host"], output = "capture")
main()
