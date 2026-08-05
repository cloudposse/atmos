<!-- atmos:template -->
# {{ .Config.project_name }}

Minimal Atmos project created with `atmos init basic`.

## What Was Generated

- `atmos.yaml` — Atmos CLI configuration (component and stack paths, stack naming)
- `stacks/_defaults.yaml` — shared configuration imported by every stack
- One stack file per environment you selected, each with the `greeting`
  component enabled (sets `vars.stage` to the environment name):
{{ range .Config.environments }}  - `stacks/{{ . }}.yaml`
{{ end -}}
- `components/terraform/greeting/` — a real, local-only Terraform component
  (no cloud account or emulator needed)

## Next Steps

1. Try out the generated `greeting` component in any environment you selected:

   ```shell
   atmos validate stacks
   atmos terraform apply greeting -s {{ index .Config.environments 0 }}
   ```

2. Add your own Terraform component under `components/terraform/<name>/`,
   configure it in a stack file, then plan and apply it the same way:

   ```shell
   atmos terraform plan <name> -s {{ index .Config.environments 0 }}
   atmos terraform apply <name> -s {{ index .Config.environments 0 }}
   ```

## Learn More

- Atmos documentation: https://atmos.tools
- Stacks: https://atmos.tools/core-concepts/stacks/
- Components: https://atmos.tools/core-concepts/components/
- CLI configuration: https://atmos.tools/cli/configuration
