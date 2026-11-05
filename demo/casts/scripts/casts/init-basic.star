load("../../casts.star", "demo_config", "finish")

def setup(c):
    c.run("rm -rf my-project")

# Reset only the generated project inside the isolated demo fixture.
rec = cast.recorder(title="atmos init: bootstrap a new project", shell="/bin/sh", write_rate="25ms", key_interval="250ms", timeout="3m", **demo_config("examples/init/init-basic", fixture="init"))
rec.setup(setup)


# Drive the real prompts and select the dev and staging environments.
rec.comment("bootstrap a brand-new Atmos project from a built-in template")
rec.terminal.pause("500ms")
rec.terminal.send("atmos init basic ./my-project")
rec.terminal.wait("Project name", timeout="10s")
rec.terminal.pause("350ms")
rec.terminal.key("enter")
rec.terminal.wait("Which environments do you need?", timeout="10s")
rec.terminal.pause("350ms")
rec.terminal.key("down")
rec.terminal.pause("350ms")
rec.terminal.key("space")
rec.terminal.pause("350ms")
rec.terminal.key("enter")
rec.terminal.wait("Generated 12 files.", timeout="15s")
rec.terminal.pause("500ms")
rec.terminal.close()


# Continue with ordinary commands in the newly generated project.
rec.cd("my-project")
rec.comment("inspect the project that was just generated")
rec.run("atmos list stacks", expect=["dev", "staging"], reject="prod")


# Verify the generated instances.
rec.comment("the greeting component was generated for each selected environment")
rec.run("atmos list instances", expect=["greeting", "dev", "staging"])


# Inspect the component catalog.
rec.comment("the component catalog summarizes the generated Terraform component")
rec.run("atmos list components", expect=["greeting", "terraform"])


# Show the generated hierarchy.
rec.comment("tree mode makes the generated stack hierarchy easy to scan")
rec.run("atmos list stacks --format=tree", expect="Stacks")


# Check the project configuration.
rec.comment("the generated project is immediately valid, no manual wiring needed")
rec.run("atmos validate stacks", expect="All stacks validated successfully")


# Deploy the local greeting component and verify both environments.
rec.comment("prove it: deploy every selected environment for real, right here")
rec.run("atmos terraform deploy --all --deploy-run-init", expect=["Apply complete! Resources: 1 added, 0 changed, 0 destroyed.", "Hello from the dev stage!", "Hello from the staging stage!"])


# The same declaration also validates the committed recording.
finish(rec)
