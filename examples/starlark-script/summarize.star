#!/usr/bin/env atmos
# Read structured data without installing another interpreter.
if len(ctx.args) != 1:
    fail("usage: ./summarize.star services.json")

def total_replicas(services):
    total = 0
    for service in services:
        total += service["replicas"]
    return total

services = json.decode(fs.read_file(ctx.args[0]))
for service in services:
    ui.info("{}: {} replicas".format(service["name"], service["replicas"]))
ui.success("Total: {} services, {} replicas".format(
    len(services),
    total_replicas(services),
))
