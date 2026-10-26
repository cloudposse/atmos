# Hook Context

Starlark script steps work in `kind: step`, `kind: steps`, and `type: test` hooks:

```yaml
hooks:
  smoke-test:
    events: [after.terraform.apply]
    kind: step
    type: script
    on_failure: fail
    with:
      interpreter: starlark
      script: |
        if ctx.operation.status == "success":
            ctx.component.exec(ctx.component.settings.get("smoke_command", ["./smoke-test"]))
            ui.success("Smoke tests passed for " + ctx.component.name)
```

`ctx.hook` has `name` and canonical `event`. `ctx.operation` has `command`, `status`,
`exit_code`, `error`, `stdout`, and `stderr`. These describe the parent operation,
not a hook subprocess. Before hooks have `None` for unavailable results. After hooks
use the supplied lifecycle outcome; absent errors are `None`. Parent stdout/stderr
are not captured by ordinary lifecycle hooks and remain `None`. Outside hooks,
`ctx.hook` and `ctx.operation` are `None`. Aggregate/unbound hooks have no component.

Use `ctx.component.settings.get("post_apply", {})` for inherited behavior controls.
The config snapshot is read-only, including nested lists/maps, and works in loaded
functions and parallel tasks. `ctx.component.exec` uses the hook's component workdir;
hook env overrides component env, followed by command, step, and per-call overrides.
Do not assume Terraform outputs/state are attached or refreshed automatically.
Atmos commands called from a hook run their normal lifecycle; scope events or use
supported hook-skipping flags to avoid re-entering the same hook. `ATMOS_HOOK_DEPTH` counts nesting; a hook at depth 8
fails with `hook recursion limit exceeded`. `print()` in a hook reaches stdout, so hooks on data commands (for example
`terraform output -json`) must use `ui.*` for human messages.
