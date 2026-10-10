#!/usr/bin/env atmos
# --help prints usage and ends the script: the print below must never run.
cli.command(run = lambda args, flags: print("main ran"), description = "Help fixture")
print("TRAILING SIDE EFFECT")
