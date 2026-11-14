infrastructure = module.load("modules/infrastructure.star")
infrastructure.run_terraform("plan", ctx.flags["service"])
