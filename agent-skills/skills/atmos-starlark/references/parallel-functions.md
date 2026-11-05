# Parallel functions

```python
def describe(name):
    return {"service": name, "region": env["REGION"]}

output = steps.parallel(
    tasks = [
        steps.task(name=n, function=describe, args=[n], timeout="30s")
        for n in ["api", "worker"]
    ],
    max_concurrency = 2,
)
```

Pass functions, not their evaluated results. Use either `functions` or `tasks`;
named tasks must be unique. `max_concurrency` defaults to 4. Shared globals, closures,
default arguments, task arguments, and returned branch results become immutable. Create
branch-local collections and return them; do not append to a shared results list. Nested
groups each have their own concurrency limit, so avoid unbounded nested fan-out.

Default behavior finishes all branches and aggregates failures. `fail_fast=True`
cancels outstanding work only after a branch's retry policy is exhausted. The
caller always joins running work. A task timeout spans all attempts and backoff.

Retry repeats the entire function. Make its side effects repeatable or place the
retry around a narrower function. Use existing retry keys such as `max_attempts`,
`initial_delay`, `backoff_strategy`, and `max_delay`; `delay` is not a valid key.
Provide `max_attempts` or a task timeout: an explicit policy without an attempt
limit retries until success, timeout, or cancellation. Output-regex `conditions` do not
apply to function tasks; they work on `exec.run(..., retry={...})`, where they match the
failed attempt's output. A task timeout failure reads `task "<name>" timed out after <duration>`.

Use `steps.task(..., timeout="30s")` for per-task limits. A `timeout:` on the YAML script step
is enforced: the interpreter and its subprocesses are canceled and the step fails with
`step timed out`.

Ctrl-C cancels the script, runs `defer` calls with a 30 s grace, and exits 130 without an error box; a second Ctrl-C
exits at once. This holds in every embedded script session.

Script output is live: lines appear as they are produced. Inside `steps.parallel` with
more than one task, each line is prefixed `[<task name>] ` and lines never interleave
mid-line. Errors are single-line messages with the Starlark traceback shown below.
