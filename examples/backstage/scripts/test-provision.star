# The full provisioning test is opt-in because it merges a PR and creates a bucket.
if ctx.flags["provision"]:
    steps.test(
        name = "backstage-provisioning",
        steps = [{
            "name": "git-to-catalog",
            "type": "script",
            "interpreter": "starlark",
            "script": 'checks = module.load("scripts/modules/backstage_tests.star")\nchecks.provision()',
        }],
    )
