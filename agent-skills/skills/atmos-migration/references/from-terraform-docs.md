# Migrating from terraform-docs

This reference covers replacing a standalone [terraform-docs](https://terraform-docs.io) CLI setup
(`.terraform-docs.yml` plus a manual invocation, Makefile target, or pre-commit hook) with
`atmos docs generate`. This is not a lossy swap: `atmos docs generate` imports
`github.com/terraform-docs/terraform-docs` directly as a Go module (a direct dependency in `go.mod`,
not a shelled-out binary) and uses it to render the same Terraform documentation, merged into a
Gomplate-templated document alongside any other YAML input. See
[atmos.tools/cli/configuration/docs](https://atmos.tools/cli/configuration/docs) and
[atmos.tools/cli/commands/docs/generate](https://atmos.tools/cli/commands/docs/generate) for the
full user-facing docs.

## Identifying the User's Shape

| Shape                                                                          | Notes |
|---------------------------------------------------------------------------------|-------|
| Single module, `.terraform-docs.yml` + manual/Makefile `terraform-docs` invocation | Direct swap — see Recipe below |
| Pre-commit hook running the `terraform-docs` binary                             | Same swap; replace the hook's command, see Recipe step 4 |
| Multi-module repo relying on `terraform-docs`' `recursive.enabled` module discovery | Gap — no Atmos equivalent; see Common Gotchas |

## Recipe

1. **Map `.terraform-docs.yml` settings into `docs.generate.<name>.terraform` in `atmos.yaml`:**

   | `.terraform-docs.yml`              | `docs.generate.<name>.terraform` | Notes |
   |-------------------------------------|-----------------------------------|-------|
   | `formatter:`                        | `format:`                         | Atmos supports `markdown table`, `markdown`, `tfvars hcl`, `tfvars json` — not the full upstream set (no `asciidoc`, `json`, `pretty`, `toml`, `xml`, `yaml`) |
   | `sort.by:`                          | `sort_by:`                        | Sort is implicitly enabled whenever `sort_by` is non-empty; there's no separate `sort.enabled` field |
   | `sections.show`/`sections.hide` for `inputs`, `outputs`, `providers` | `show_inputs:` / `show_outputs:` / `show_providers:` (booleans) | Only these three sections are controllable; `header`, `footer`, `requirements`, `resources`, `data-sources`, `modules` have no Atmos equivalent |
   | (module root directory)             | `source:` (relative to `base-dir`) | |
   | `.terraform-docs.yml` file itself   | the `terraform:` block             | No separate config file — it's inline in `atmos.yaml` |

   ```yaml
   # atmos.yaml
   docs:
     generate:
       readme:
         base-dir: .
         input:
           - "./README.yaml"
         template: "./README.md.gotmpl"
         output: "./README.md"
         terraform:
           source: src/
           enabled: true
           format: "markdown table"
           show_inputs: true
           show_outputs: true
           show_providers: false
           sort_by: "name"
   ```

2. **Add a Go template that renders the `terraform_docs` data key.** `atmos docs generate` injects
   the rendered Terraform docs into the merged template data under `terraform_docs`:

   ```gotmpl
   {{- $data := (ds "config") -}}
   # {{ $data.name | default "Project Title" }}

   {{ $data.description | default "No description provided." }}

   {{ if has $data "terraform_docs" }}
   ## Terraform Documentation

   {{ $data.terraform_docs }}
   {{ end }}
   ```

3. **Run `atmos docs generate <name>` and diff the output** against the README the standalone
   `terraform-docs` binary produced, to confirm parity before removing the old tooling.

4. **Replace the CI/pre-commit step.** A pre-commit hook or CI step that runs
   `terraform-docs markdown table ./module > README.md` becomes a step that runs
   `atmos docs generate <name>`.

5. **Remove the standalone setup**: delete `.terraform-docs.yml`, and drop the `terraform-docs`
   binary from `dependencies.tools`/`.tool-versions` (if toolchain-managed) or from CI setup steps,
   once the generated output is verified equivalent.

## Common Gotchas

### No recursive multi-module scan

`docs.generate.<name>.terraform.source` targets exactly one Terraform module directory per named
generator. Standalone `terraform-docs`' `recursive.enabled` (scanning a `modules/` tree and writing a
README per submodule) has no direct equivalent — a multi-module repo needs one
`docs.generate.<name>` block (and one `atmos docs generate <name>` invocation) per module, not a
single recursive pass.

### Running both tools at once

Don't leave the standalone `terraform-docs` binary wired into CI/pre-commit alongside
`atmos docs generate` — both write into the same `<!-- BEGIN_TF_DOCS -->` / `<!-- END_TF_DOCS -->`
markers (or overwrite the whole file, depending on template), so running both produces
duplicate or conflicting content.

### Hand-edited content inside generated sections

Content between the generated markers (or the whole output file, depending on template design) is
overwritten on every `atmos docs generate` run, same as with standalone `terraform-docs`. Put
hand-written prose in the `input` YAML or outside the generated section, not inside it.

## What to NOT Do

- Do not keep the standalone `terraform-docs` binary installed "just in case" once
  `atmos docs generate` output is verified equivalent for every module.
- Do not hand-roll a shell wrapper or Makefile target around the `terraform-docs` binary when
  `docs.generate` already covers the need — that reintroduces the exact tool sprawl this migration
  removes.
