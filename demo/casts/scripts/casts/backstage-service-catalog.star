# Record the running local demo. Lifecycle ownership stays with `atmos portal up/down`.
casts = module.load("../../casts.star")
checks = module.load("../../cast_checks.star")

def policy(text):
    casts.demo_policy(text)
    plain = checks.strip_ansi(text)
    for secret in ["Bearer ", "Authorization:", "YXRtb3M6YXRtb3M=", "atmos:atmos@"]:
        if secret in plain:
            fail("Backstage cast contains authentication material")

config = casts.demo_config("examples/backstage/service-catalog", fixture = "backstage")
# This cast deliberately uses the running demo; copying it would still share its
# loopback ports, container names, Git repository, and Terraform state.
config["working_directory"] = "../../examples/backstage"
config["policy"] = policy
rec = cast.recorder(title = "Backstage service catalog: provision with Atmos and Gitea", **config)

rec.comment("Backstage + Gitea Actions + an AWS emulator, managed by Atmos")
rec.run("atmos portal status", expect = "atmos-local-container-backstage", interval = "2s")
rec.comment("Test the catalog, TechDocs, stack inheritance, and input validation")
rec.comment("Then request a service: PR -> plan -> merge -> provision S3 -> register in Backstage")
rec.run(
    "atmos portal test --provision",
    expect = ["9 passed, 0 failed", "git-to-catalog", "1 passed, 0 failed"],
    reject = ["[failed]", "panic:", "Traceback"],
    timeout = "15m",
    interval = "3s",
)
rec.comment("Explore services and TechDocs: http://localhost:3000")
rec.comment("Review PRs and CI runs: http://localhost:3001/atmos/deployments")
rec.pause("3s")
casts.finish(rec)
