#!/usr/bin/env atmos
# Field test: project-aware bindings from a standalone script.
step = ctx.args[0] if len(ctx.args) > 0 else "all"

def show(label, v):
    print("== " + label)
    print(json.encode(v))

def do_list():
    show("atmos.list components data", atmos.list("components").data)
    show("atmos.list stacks data", atmos.list("stacks").data)

def do_describe():
    r = atmos.describe("component", "mock", flags = {"stack": "dev"})
    show("describe mock vars", r.data["vars"])
    show("describe keys", sorted(r.data.keys()))

def do_config():
    show("config get positional base_path", atmos.config("get", "base_path").data)
    show("config get args=[base_path]", atmos.config("get", args = ["base_path"]).data)
    show("config get name_template", atmos.config("get", "stacks.name_template").data)

def do_components():
    c = components.get("mock", "dev", "terraform")
    print(c)
    show("components.get vars", c.vars)
    show("components.get path", c.path)
    show("components.get implementation", c.implementation)
    show("api vars", components.get("api", "dev", "app").vars)

if step in ("all", "list"):
    do_list()
if step in ("all", "describe"):
    do_describe()
if step in ("all", "config"):
    do_config()
if step in ("all", "components"):
    do_components()
if step == "missing":
    components.get("nope", "dev", "terraform")
if step == "badstack":
    components.get("mock", "nostack", "terraform")
if step == "plan":
    r = atmos.terraform("plan", "mock", "dev", output = "capture")
    print(r.stdout[-400:])
