# Tests

We have automated tests in packages, as well as standalone tests in this directory.

```
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
    └── schema.json

```

## Test Cases

Our convention is to implement a test-case configuration file per scenario. Then place all smoke tests related to that scenario in the file.

###  Environment Variables

The tests will automatically set some environment variables:

- `GO_TEST=1` is always set, so commands in atmos can disable certain functionality during tests
- `TERM` is set when `tty: true` to emulate a proper terminal

### Flags

To regenerate snapshots pass the `-regenerate-snaphosts` flag.

> ![WARNING]
>
> #### This will regenerate all the snapshots
>
> After regenerating, make sure to review the differences:
>
> ```shell
> git diff tests/snapshots
> ```

### Example Configuration

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
