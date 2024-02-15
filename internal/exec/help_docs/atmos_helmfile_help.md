# Atmos Helmfile Help

Check out the [Atmos Helmfile CLI documentation](https://atmos.tools/cli/commands/helmfile/usage).

Atmos supports all Helmfile commands and options described in
the [Helmfile docs](https://helmfile.readthedocs.io/en/latest).

__NOTE:__ Execute `helmfile --help` to see help for the Helmfile CLI commands.

In addition, the `component` argument and `stack` flag are required to generate the variables and backend config
for the component in the stack. For example: `atmos helmfile diff eks/echo-server --stack plat-ue2-prod`

<br/>

## Additions and differences from native Helmfile

- `atmos helmfile generate varfile` command generates a varfile for the component in the stack
  <br/>
- `atmos helmfile` commands support `[global options]` using the command-line flag `--global-options`.
  Usage: `atmos helmfile [command] [component] -s [stack] [command options] [arguments] --global-options="--no-color --namespace=test"`
  <br/>
- before executing the `helmfile` commands, Atmos runs the `aws eks update-kubeconfig` command to read kubeconfig from the EKS cluster and use it to authenticate with the cluster.
  This can be disabled in `atmos.yaml` CLI config by setting the `components.helmfile.use_eks` attribute to `false`
  <br/>
- double-dash `--` can be used to signify the end of the options for Atmos and the start of the additional native arguments and flags for the Helmfile commands
  <br/>
