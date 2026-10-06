#!/usr/bin/env atmos
# Declare the interface; Atmos handles parsing and help before main runs.
def validate(args, flags):
    if flags["replicas"] < 1:
        fail("replicas must be positive")

def main(args, flags):
    ui.success("{}: {} replicas x 4 workers = {} workers".format(
        args["service"], flags["replicas"], flags["replicas"] * 4,
    ))

cli.command(
    description = "Calculate a service's total worker capacity",
    args = [cli.arg("service", description="Service name")],
    flags = [cli.flag("replicas", type="int", shorthand="r", default=2,
                      description="Number of service replicas")],
    validate = validate,
    run = main,
)
