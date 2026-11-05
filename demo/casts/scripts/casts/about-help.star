load("../../casts.star", "demo_config", "finish")

config = demo_config("screengrabs/atmos-about--help", fixture="basic")
config["width"] = 90
rec = cast.recorder(title="atmos about --help", **config)


# Capture help at the documentation column width.
rec.run("atmos about --help", expect=["USAGE", "Display information about Atmos"], rate="0s", duration="0s", interval="0s")


# Keep the review image local; only the cast is published.
rec.screenshot("../../.context/about-help.png")


finish(rec)
