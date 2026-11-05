#!/usr/bin/env atmos
# The default is not one of the choices: a declaration mistake in the script, reported as such.
cli.command(
    run = lambda args, flags: None,
    flags = [cli.flag("stage", default = "qa", choices = ["dev", "prod"])],
)
