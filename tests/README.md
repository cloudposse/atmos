# Tests

We have automated tests in packages, as well as standalone tests in this directory.

Smoke tests are implemented to verify the basic functionality and expected behavior of the compiled `atmos` binary, simulating real-world usage scenarios.

```shell
├── cli_test.go                                      # Responsible for smoke testing
├── fixtures/
│   ├── components/
│   │   └── terraform/                               # Components that are conveniently reused for tests
│   ├── scenarios/                                   # Test scenarios consisting of stack configurations (valid & invalid)
│   │   ├── complete/                                # Advanced stack configuration with both broken and valid stacks
│   │   ├── metadata/                                # Test configurations for `metadata.*`
│   │   └── relative-paths/                          # Test configurations for relative imports
│   └── schemas/                                     # Schemas used for JSON validation
├── snapshots/                                       # Golden snapshots (what we expect output to look like)
│   ├── TestCLICommands.stderr.golden
│   └── TestCLICommands_which_atmos.stdout.golden
└── test-cases/
    ├── complete.yaml
    ├── core.yaml
    ├── demo-custom-command.yaml
    ├── demo-stacks.yaml
    ├── demo-vendoring.yaml
    ├── metadata.yaml
    ├── relative-paths.yaml
    └── schema.json                                 # JSON schema for validation

```

## Test Cases

Our convention is to implement a test-case configuration file per scenario. Then place all smoke tests related to that scenario in the file.

###  Environment Variables

The tests will automatically set some environment variables:

- `GO_TEST=1` is always set, so commands in atmos can disable certain functionality during tests
- `TERM` is set when `tty: true` to emulate a proper terminal

### Flags

To regenerate ALL snapshots pass the `-regenerate-snaphosts` flag.

> ![WARNING]
>
> #### This will regenerate all the snapshots
>
> After regenerating, make sure to review the differences:
>
> ```shell
> git diff tests/snapshots
> ```

To regenerate the snapshots for a specific test, just run:

(replace `TestCLICommands/check_atmos_--help_in_empty-dir` with your test name)

```shell
go test ./tests -v -run 'TestCLICommands/check_atmos_--help_in_empty-dir' -timeout 2m -regenerate-snapshots
```

After generating new golden snapshots, don't forget to add them.

```shell
git add tests/snapshots/*
```

### Example Configuration

We support an explicit type `!not` on the `expect.stdout` and `expect.stderr` sections (not on `expect.diff`)

Snapshots are enabled by setting the `snapshots` flag, and using the `expect.diff` to ignore line-level differences. If no differences are expected, use an empty list. Note, things like paths will change between local development and CI, so some differences are often expected.

We recommend testing incorrect usage with `expect.exit_code` of non-zero. For example, passing unsupported arguments.


```yaml
# yaml-language-server: $schema=schema.json

tests:
  - name: atmos circuit-breaker
    enabled: true                             # Whether or not to enable this check
    snapshot: true                            # Enable golden snapshot. Use together with `expect.diff`
    description: >                            # Friendly description of what this test is verifying
      Ensure atmos breaks the infinite loop when shell depth exceeds maximum (10).
    workdir: "fixtures/scenarios/complete/"
    command: "atmos"                          # Command to run
    args:                                     # Arguments or flags passed to command
      - "help"
    expect:                                   # Assertions
      diff: []                                # List of expected differences
      stdout:                                 # Expected output to stdout or TTY. All TTY output is directed to stdout
      stderr:                                 # Expected output to stderr;
        - "^$"                                # Expect no output
      exit_code: 0                            # Expected exit code
```
