# Custom Commands

Customize Atmos to run any command you want to make it easier for teams to use your toolchain.

## Examples

Examples in this demo are all defined in the [`atmos.yaml`](atmos.yaml) configuration under the `commands` section.

### Help

Custom Commands are available via the `atmos --help` command, just like all the other commands in Atmos. This makes discovery
easier for developers, as they get a single pane of glass to your DevOps tooling.

Run the following command to get the help menu.

```shell
atmos --help
```

Notice that a few custom commands are available:
- `atmos hello`
- `atmos github`
- `atmos weather`

### Hello World!

This is the most basic example that shows how easy it is to add a command to atmos.

Run run the following command:

```shell
 atmos hello    
```

### Check GitHub Status

We all depend on GitHub these days! Let's make it easy to check the status of GitHub from the command line.

Just run this command:

```shell
atmos github status
```

- `atmos github stargazers cloudposse/atmos`
- `atmos weather --location LAX`
