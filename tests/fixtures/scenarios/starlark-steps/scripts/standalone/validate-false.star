#!/usr/bin/env atmos
# A validate callback that returns False rejects the input as a usage error.
def check(args, flags):
    return False

cli.command(run = lambda args, flags: print("main ran"), validate = check)
