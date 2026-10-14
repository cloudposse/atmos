# Embedded Starlark runtime

The root package owns engine registration, invocation state, module loading,
component context, process execution, and parallel task orchestration. Each
invocation has its own session; parallel tasks share that session's synchronized
output and component cache.

Independent language bindings live under `stdlib/`:

- `atmos` constructs CLI arguments and exposes helpers from a host-supplied command
  catalog. A callback delegates execution to the engine's process services.
- `log` validates structured diagnostics, masks values, and writes to the injected
  logger. The host supplies step and task attribution.
- `regex` implements stateless regular-expression operations.

The CLI snapshots its completed command tree through `cmd/internal` and
`pkg/flags.CommandCatalog`, after custom commands and aliases are registered.
This supplies command names, shorthand mappings, and native compatibility flag
spellings without importing `cmd` into the runtime or sharing mutable Cobra
objects with concurrent scripts. Actual parsing, validation, environment binding,
and command execution still happen in the invoked Atmos binary.

Other embedding hosts can supply `WithAtmosCommands`. Without a catalog, the
explicit `atmos.run`, `atmos.terraform`, `atmos.helm`, and `atmos.toolchain` helpers
remain available; additional named helpers reflect the host's catalog. Explicit
flag prefixes pass through unchanged. Bare flags use catalog metadata when
available, otherwise the ordinary long-flag spelling.

Keep operations that depend on invocation state in the engine. Extract bindings
when they have independent behavior and a small host interface; do not expose the
session struct or split parallel execution away from its cancellation and output
lifecycle solely to reduce file counts.

## Component resolution and task deadlines

`ctx.component` is a lazy handle. Reading its attributes resolves the component
using the invocation context, because Starlark's attribute interface does not
provide the calling thread. The invocation deadline and cancellation still apply;
a task-specific timeout does not interrupt this lazy attribute resolution.

For resolution bounded by a task deadline, call `components.get` inside the task:

```python
def read():
    component = components.get(name="api", stack="dev", type="terraform")
    return component.vars

output = steps.parallel(tasks=[steps.task(name="read", function=read, timeout="5s")])
```

Both paths share the same per-invocation cache. Successful resolutions are reused;
failures can be retried. A `components.get` caller waiting for another resolver can
cancel without waiting for that resolver to finish. Supply the reference directly,
or read its fields before starting parallel tasks; reading `ctx.component.name`
inside the task itself would trigger the invocation-scoped resolution first.
