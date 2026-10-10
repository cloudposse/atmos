#!/usr/bin/env atmos
# Standalone inputs fixture: one required argument, a required flag, choices, an environment
# binding, a bool, and an int.
def main(args, flags):
    print(json.encode({"args": args, "flags": flags}))

cli.command(
    description = "Inputs fixture",
    args = [cli.arg("service", description = "Service name")],
    flags = [
        cli.flag("token", required = True, description = "API token"),
        cli.flag("stage", choices = ["dev", "prod"], description = "Stage"),
        cli.flag("region", env = "FT_REGION", description = "Region"),
        cli.flag("verbose", type = "bool", description = "Verbose output"),
        cli.flag("count", type = "int", default = 1, description = "A count"),
    ],
    run = main,
)
