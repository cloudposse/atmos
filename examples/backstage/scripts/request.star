# Runs in Backstage's isolated task workspace.
infrastructure = module.load("modules/infrastructure.star")
infrastructure.publish_request(ctx.flags["service"])
