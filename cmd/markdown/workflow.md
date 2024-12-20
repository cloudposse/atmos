# Invalid Command
The command `atmos workflow list` is not valid.

## Examples
Use one of the following commands:
```shell
$ atmos workflow                                    # Use interactive UI
$ atmos workflow <name> --file <file>              # Execute workflow
$ atmos workflow <name> --file <file> --stack <stack>
$ atmos workflow <name> --file <file> --from-step <step>
```
For more information, refer to the [docs](https://atmos.tools/cli/commands/workflow/).

# Missing Required Flag
The `--file` flag is required to specify a workflow manifest.

## Examples
– Deploy a workflow
```shell
$ atmos workflow deploy-infra --file workflow1
```
– Deploy with stack configuration                                               
```shell
$ atmos workflow deploy-infra --file workflow1 --stack dev
```
– Resume from a specific step                                                   
```shell
$ atmos workflow deploy-infra --file workflow1 --from-step deploy-vpc
```
For more information, refer to the [docs](https://atmos.tools/cli/commands/workflow/).

# Workflow File Not Found
The workflow manifest file could not be found.

## Examples
– List available workflows
```shell
$ ls workflows/
```
– Execute a specific workflow
```shell
$ atmos workflow --file <file>
```
For more information, refer to the [docs](https://atmos.tools/cli/commands/workflow/).

# Invalid Workflow
The specified workflow is not valid or has incorrect configuration.

## Examples
– Example of a valid workflow configuration:
```yaml
name: deploy-infra
description: Deploy infrastructure components
steps:
  - name: deploy-vpc
    command: atmos terraform apply vpc
  - name: deploy-eks
    command: atmos terraform apply eks
```
For more information, refer to the [docs](https://atmos.tools/cli/commands/workflow/).
