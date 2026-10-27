load("../../casts.star", "demo_config", "finish")

rec = cast.recorder(title="Container Steps: build and run an image", **demo_config("examples/container-step/build-run", fixture="container-step"))


# Keep container lifecycle ownership in the existing workflow.
rec.comment("build a local image, then run it in a later step")
rec.run("atmos workflow build-run -f container-step", expect="hello from the built Atmos example image", duration="650ms", interval="750ms")


# Check the container output before publishing the recording.
finish(rec)
