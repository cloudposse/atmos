# Identation

This is a basic test case for indentation.
We use this for the tests in `tests/test-cases/indentation.yaml`
This is ran by `TestCLICommands` in `tests/cli_test.go`

Here we run `atmos describe config -f yaml` expecting indentation of 4
(default of 2 tested in every other test TODO make explicit test)

We compare this against the golden snapshot `TestCLICommands_indentation.stdout.golden`
