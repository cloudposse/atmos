<!-- atmos:template -->
# {{ .Config.project_name }}

Minimal Atmos project created with `atmos init basic`.

## What Was Generated

- `atmos.yaml` — Atmos CLI configuration (component and stack paths, stack naming)
- `stacks/_defaults.yaml` — shared configuration imported by every stack
- `stacks/dev.yaml` — the `dev` stack (sets `vars.stage: dev`)
- `components/terraform/` — where your Terraform components live

## Next Steps

1. Add a Terraform component under `components/terraform/<name>/`
2. Configure it in `stacks/dev.yaml` (see the commented example)
3. Validate your configuration:

   ```shell
   atmos validate stacks
   ```

4. Plan and apply the component:

   ```shell
   atmos terraform plan <name> -s dev
   atmos terraform apply <name> -s dev
   ```

## Learn More

- Atmos documentation: https://atmos.tools
- Stacks: https://atmos.tools/core-concepts/stacks/
- Components: https://atmos.tools/core-concepts/components/
- CLI configuration: https://atmos.tools/cli/configuration
