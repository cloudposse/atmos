def plan_component(name, stack):
    component = components.get(name = name, stack = stack, type = "application")
    return {
        "name": component.name,
        "version": component.vars["version"],
        "replicas": component.vars["replicas"],
        "region": component.env["DEPLOY_REGION"],
    }
