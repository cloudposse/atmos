infrastructure = module.load("modules/infrastructure.star")
catalog = module.load("modules/catalog.star")
name = infrastructure.run_terraform("apply", ctx.flags["service"])
catalog.register_service(name)
