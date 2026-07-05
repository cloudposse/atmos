<!-- atmos:template -->
# [[ .Config.project_name ]]

An [Atmos](https://atmos.tools) landing zone for Azure: flat `dev`,
`staging`, and `prod` environments with a resource group, virtual network,
subnet, and network security group.

The scaffold includes only resources that apply against `floci/az` without
extra infrastructure. Azure Storage and Key Vault are intentionally omitted:
the current emulator can run them only with a data-plane DNS/proxy shim, and
Log Analytics is not implemented.

## Quick start

```shell
atmos test
```

To work with one environment:

```shell
atmos emulator up azure -s dev
atmos terraform apply --all -s dev -i false
atmos terraform output network -s dev
atmos emulator down azure -s dev
```

On macOS, native Terraform may reject the emulator's self-signed certificate.
Run the Azure Terraform flow in Linux (for example a container or CI runner)
until the emulator certificate is standards-compliant or trusted in the system
keychain.

## Layout

```
atmos.yaml
components/terraform/network/
stacks/_defaults.yaml
stacks/dev.yaml
stacks/staging.yaml
stacks/prod.yaml
```
