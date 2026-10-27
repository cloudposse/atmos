load("../../casts.star", "demo_config", "finish")

def setup(c):
    c.step("workdir", source="../../../demo/casts/fixtures/cast-tools", path=".cast-bin", reset=True)
    c.run("rm -f .env.cast")

def cleanup(c):
    c.run("rm -f .env.cast")

# Prepare a deterministic credential provider and always remove the exported token file.
rec = cast.recorder(title="atmos env masks terminal secrets", **demo_config("examples/demo-env/masked-token", fixture="demo-env"))
rec.setup(setup)
rec.cleanup(cleanup)


# Establish the fixture context before showing environment output.
rec.comment("start with the stack that uses the GitHub provider")
rec.run("atmos list stacks", expect="demo")


# Terminal output must mask the token while preserving ordinary variables.
rec.comment("token is masked; normal env stays visible")
rec.run("atmos env", expect=["export GITHUB_TOKEN=<MASKED>", "export DEMO_ENVIRONMENT=local-development"], reject=cast.regex(r"gho_[A-Za-z0-9]{36}"))


# File output intentionally retains the token for automation.
rec.comment("automation output stays raw")
rec.run("atmos env --output-file .env.cast")
rec.run("grep -q '^export GITHUB_TOKEN=gho_' .env.cast && echo 'raw token preserved for automation'", expect="raw token preserved for automation")
rec.run("grep -q '^export DEMO_ENVIRONMENT=local-development$' .env.cast && echo 'normal env preserved too'", expect="normal env preserved too")


# Generate and validate, or check the saved cast without running its actions.
finish(rec)
