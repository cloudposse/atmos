# Test steps

Run these commands from this directory. No cloud credentials or deployment are needed.

```shell
atmos workflow passing -f tests
atmos workflow demo -f tests
atmos workflow verbose -f tests
atmos test
```

`demo` deliberately fails its authentication check and exits nonzero. Its output
appears immediately; the regional matrix still runs. The default report hides
successful logs. `verbose` uses `output: all` to display them.

The test tree uses green/red dots, per-check durations, and a bottom progress bar
in a terminal. CI receives a static tree and the same failure details.
Concurrency belongs to nested `parallel` and `matrix` steps.

For post-deployment checks, use the same step in a component hook:

```yaml
hooks:
  smoke-tests:
    events: [after.terraform.apply]
    kind: step
    type: test
    on_failure: fail
    with:
      title: Post-deployment tests
      steps:
        - name: health
          type: http
          url: https://example.com/health
          expect:
            status: [200]
```

`on_failure: fail` makes failed checks fail the deployment command. The test group
itself defaults to `fail.mode: wait_all`; use `fail_fast` to cancel remaining tests
or `best_effort` to report failures without failing the group.

The recorded demo is published at
`website/static/casts/examples/tests/tests.cast`. Regenerate it from the repository
root with `atmos --chdir=demo/casts casts generate demo fixtures tests`.
