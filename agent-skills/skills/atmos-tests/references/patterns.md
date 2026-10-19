# Test Suite Patterns

The URLs, scripts, and stack names below are placeholders. Replace them with
actual project targets. The HTTP and language checks fail on unmet expectations;
printing an error without a failing exit status would count as success.

## Custom Command with Native HTTP Checks

Define this in `atmos.yaml` or imported CLI configuration, then run `atmos test`:

```yaml
commands:
  - name: test
    description: Run smoke tests against the deployed application
    steps:
      - name: smoke-tests
        type: test
        title: Post-deployment tests
        output: failures
        fail:
          mode: wait_all
        steps:
          - name: health
            title: Health
            type: http
            url: https://app.example.com/health
            timeout: 10s
            expect:
              status: [200]
              response: ['"status"\s*:\s*"ok"']
            retry:
              max_attempts: 3
              delay: 2s
          - name: homepage
            title: Homepage
            type: http
            url: https://app.example.com/
            expect:
              status: [200]
```

`expect.response` is a list of response-body regexes; at least one must match.
It is not a JSONPath assertion. HTTP retries cover transport errors, 5xx, and 429
by default; consult the HTTP documentation for `retry.conditions` when another
response should be retried. Use a script for precise JSON assertions.

For a workflow, put the same test step under that workflow's `steps` and invoke
it through `atmos workflow <name> -f <file>`.

## Parallel Checks with a Dependent Script Assertion

Insert this group as a child of `type: test`. Homepage can run alongside health;
response validation waits for health. If health fails, validation is skipped.
The health response is `.steps.health.value`, not a shared temporary file.

```yaml
- name: endpoints
  title: Endpoints
  type: parallel
  max_concurrency: 2
  steps:
    - name: health
      title: Health response
      type: http
      url: https://app.example.com/health
      expect:
        status: [200]
    - name: validate-response
      title: Health payload
      type: script
      needs: [health]
      interpreter: python3
      env:
        HEALTH_RESPONSE: '{{ .steps.health.value }}'
      script: |
        import json
        import os

        response = json.loads(os.environ["HEALTH_RESPONSE"])
        if response.get("status") != "ok":
            raise SystemExit("Expected health status 'ok'")
        if response.get("ready") is not True:
            raise SystemExit("Application is not ready")
        print("Health payload is valid")
    - name: homepage
      title: Homepage
      type: http
      url: https://app.example.com/
      expect:
        status: [200]
```

Declare Python in the owning command/workflow's `dependencies.tools` if it is
not provided by the project's execution environment. Passing raw response data
through `env` avoids embedding it as executable Python source.

## Matrix Cases with Existing Shell Tests

Insert this as another child of `type: test`, alongside a parallel group rather
than inside it. Two regions times two stages expand to four leaf cases. The
checked-in script must inspect the target and return nonzero on failure.

```yaml
- name: regions
  title: Regional checks
  type: matrix
  matrix:
    region: [us-east-1, eu-west-1]
    stage: [dev, staging]
  max_concurrency: 2
  steps:
    - name: smoke
      title: Smoke test
      type: shell
      env:
        AWS_REGION: '{{ .matrix.region }}'
        TEST_STAGE: '{{ .matrix.stage }}'
      command: ./tests/smoke.sh
```

Use matrix variables in HTTP URLs, script environment variables, or other
supported templated fields too. Keep credentials separate from axis values.
If cases write files, give each combination its own path. A matrix can also use
child `needs` to sequence dependent checks within each combination.

## Preconditions and Other Interpreters

Check prerequisites before the suite when they are setup requirements rather
than cases to report. These two steps belong in the owning command/workflow's
`steps` list; Node must already be available or declared in `dependencies.tools`.

```yaml
- name: prerequisites
  type: require
  tools: [node]
  files: [tests/contract.json]
- name: contract-tests
  type: test
  title: Integration contract
  steps:
    - name: contract
      type: script
      interpreter: node
      script: |
        const fs = require('node:fs');
        const contract = JSON.parse(fs.readFileSync('tests/contract.json', 'utf8'));
        if (contract.version !== 1) {
          throw new Error('Expected contract version 1');
        }
        console.log('Contract version is valid');
```

`require` can also be a leaf when file/tool/directory presence is itself a test.
Use `type: atmos` for native checks such as `command: validate stacks`; a successful
`describe` command alone does not assert that a deployment is healthy.

## Run After Terraform Apply

Place this in the component's `hooks` configuration using the existing lifecycle
event. The hook envelope contains `on_failure`; test fields belong under `with`.

```yaml
hooks:
  smoke-tests:
    events: [after.terraform.apply]
    kind: step
    type: test
    on_failure: fail
    with:
      title: Post-deployment tests
      output: failures
      fail:
        mode: wait_all
      steps:
        - name: health
          title: Health
          type: http
          url: https://app.example.com/health
          expect:
            status: [200]
```

Set `on_failure: fail` for a deployment gate: step hooks otherwise default to
warning. The same `with.steps` can contain the parallel and matrix patterns above.
Use the hook skill for component working-directory rules, authentication, and
hook conditions. Do not invent a new deployment trigger or a separate test runner.

## Debugging and Failure Policy

For more context, change the group's scalar output to `all`. To stop remaining
work after a failure, use `fail: {mode: fail_fast}`. For explicitly advisory checks,
`fail: {mode: best_effort}` allows a successful group despite red failures.
A leaf's `continue: always` tolerates that leaf's failure without turning it green.
Preserve these deliberate policies when extending an existing suite.
