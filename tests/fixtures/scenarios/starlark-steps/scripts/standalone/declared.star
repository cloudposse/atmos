#!/usr/bin/env atmos
def check(args, flags):
    if flags["count"] == 99:
        return False

def main(args, flags):
    print(json.encode({"args": args, "flags": flags}))

cli.command(
    description = "Every flag type",
    args = [
        cli.arg("target", description = "Deploy target"),
        cli.arg("region", description = "Optional region", required = False),
    ],
    flags = [
        cli.flag("name", type = "string", default = "dflt", shorthand = "n", description = "A name"),
        cli.flag("count", type = "int", default = 1, shorthand = "c", description = "A count"),
        cli.flag("flag", type = "bool", shorthand = "f", description = "A bool"),
        cli.flag("list", type = "string_list", shorthand = "l", env = "FT_LIST", description = "A list"),
        cli.flag("stage", choices = ["dev", "prod"], default = "dev", env = "FT_STAGE", description = "Stage"),
        cli.flag("token", required = True, env = "FT_TOKEN", description = "API token"),
    ],
    validate = check,
    run = main,
)
