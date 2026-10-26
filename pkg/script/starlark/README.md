# Embedded Starlark runtime

The Starlark package owns language bindings, module loading, component handles,
thread management, and parallel callable isolation. Each invocation has its own
session; parallel tasks share that session's synchronized output and component
cache. [The script host](../README.md) owns interpreter registration, invocation
normalization, process policy, tool pins, and authored error metadata.

Independent language bindings live under `stdlib/`:

- `atmos` constructs CLI arguments and exposes helpers from a host-supplied command
  catalog. A callback delegates execution to the engine's process services.
- `log` validates structured diagnostics, masks values, and writes to the injected
  logger. The host supplies step and task attribution.
- `config` owns typed configuration parsing, schemas, immutable records, and deep
  merging. The host supplies file reads, caller-relative paths, and an optional HTTP
  client; `internal/threadcontext` carries task cancellation to the bindings.
- `regex` implements stateless regular-expression operations.
- `cli` translates declarative script arguments and flags into native flag
  definitions. The standalone CLI host parses them using `StandardParser` and
  `PositionalArgsBuilder`; the binding calls validation and main functions only
  after parsing succeeds, and skips both for help.

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

Keep language-specific operations that depend on invocation state in the engine. Extract bindings
when they have independent behavior and a small host interface; do not expose the
session struct or split parallel execution away from its cancellation and output
lifecycle solely to reduce file counts.

## Script libraries

`module.load(path)` returns a frozen namespace of a file's public globals. It shares
initialization and caching with ordinary `load()` and supports concurrent callers.
The loader tracks pending dependencies across threads; parallel children inherit the
active initializer so cycles fail before waiting. It never holds the loader mutex
while reading or evaluating a file. Failed entries are removed after notifying waiters.

The automation parser accepts `.load` attributes, which upstream Starlark reserves as
a keyword. `program.go` temporarily substitutes equal-width identifier tokens outside
strings and comments, then restores the attribute names in the AST before resolution.
This preserves source positions and avoids modifying the dependency. Entry scripts,
loaded files, and dry runs share this parser; configuration and policy evaluators do not.

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

## Repository checks

Repository check scripts can use `git.sandbox(run=callback, scope="staged")` to
execute against an isolated index/checkout. The native `pkg/git/sandbox` package
uses the shared Git runner and atomic filesystem writer. Callback-local execution
settings propagate to file operations, commands, parallel tasks, and nested steps;
no process-wide chdir is used. `fixes="unstaged"` reconciles changes without writing
the live index. Callback defers run before reconciliation; cancellation discards
fixes. `fs.write_file(..., overwrite=True)` provides atomic replacements and `yaml.validate` checks every YAML
document without evaluating tags. `yaml.decode` reads a single YAML data document
into dictionaries, lists, and scalars. See their automation function references for the
public contracts.

## Typed configuration

`config.load` and `config.validate` accept inline schemas (types, field dictionaries, and immutable `schema.*`
descriptors) or paths to JSON Schema documents. `stdlib/config/data.go` parses YAML nodes for both formats to preserve
source positions, after strictly checking JSON syntax. Expanded node counts and depth are bounded; duplicate
and non-string keys, non-finite numbers, custom tags, and merge keys are rejected. Configuration is plain data.

`stdlib/config/schema.go` snapshots inline schema helpers and validates copied defaults; `stdlib/config/validate.go` validates
without coercion and creates deeply frozen mapping records with attribute access. `stdlib/config/jsonschema.go` uses
the existing JSON Schema library and engine-backed local/HTTPS resource loaders, including relative `$ref` documents.
File schemas retain standard JSON Schema default semantics. `stdlib/config/remote.go` applies request timeouts, byte/resource limits, TLS/redirect rules, and cancellation;
remote documents are checked for local-file references before compilation. There is no process-global cache.
Validation failures map JSON pointers back to source nodes (or the nearest parent for missing fields).

`configRecord.merge` delegates to `pkg/merge.MergeWithOptions`, with the same replace/append/merge list
strategies. The JSON conversion boundary uses `json.Number` to preserve arbitrary-precision integers and
float types. Merge results are fresh frozen records; validation is explicit via `config.validate` rather than
implicitly retaining or re-fetching either source schema. The reserved `merge` attribute is a bound method;
record JSON encoding remains based on data items, and a field named `merge` is accessible by index.
