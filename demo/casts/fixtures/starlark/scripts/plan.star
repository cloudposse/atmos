load("components.star", "plan_component")

def release_plan():
    selected = ctx.component
    print("Release plan for {} in {}".format(selected.name, selected.stack))
    results = steps.parallel(
        tasks = [
            steps.task(
                name = name,
                function = plan_component,
                args = [name, selected.stack],
                retry = {"max_attempts": 3, "initial_delay": "100ms"},
                timeout = "10s",
            )
            for name in selected.settings["release_components"]
        ],
        max_concurrency = 2,
    )
    for result in results:
        print("  {}: v{} | replicas={} | region={}".format(
            result["name"], result["version"], result["replicas"], result["region"],
        ))
    ui.success("All {} component plans ready. No infrastructure changed.".format(len(results)))
    return results

output = release_plan()
