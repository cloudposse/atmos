---
title: Say Something (Text-to-Speech)
tags: [Automation]
---

# Say Something Demo

This example demonstrates the `say` step type in custom commands, workflows,
and Terraform lifecycle hooks. Use this type of step to announce when things
happen in your workflows, like when something completes or fails.

`say` works across platforms by detecting an available speech engine (`say` on macOS, `spd-say`/`espeak`/`espeak-ng` on Linux, PowerShell's `System.Speech` on Windows). When no engine is available — or when running in CI or another headless environment — it degrades gracefully according to the `print` policy (by default, printing the message as a Markdown blockquote).

## Prerequisites

- Atmos CLI installed
- A text-to-speech engine for audio output (optional — without one, messages are printed)

## Quick Start

```bash
cd examples/say-something

# Custom command: explain when say steps are useful
atmos say something

# Workflow: the same say step type in a workflow
atmos workflow notify -f say

# Pick the first available voice from a cross-platform stack
atmos workflow voices -f say

# Hear slow vs. fast speech
atmos workflow rates -f say

# See the three print policies
atmos workflow print-modes -f say

# Announce each milestone of a build pipeline
atmos workflow pipeline -f say

# Terraform hook: announce whether apply was successful or not successful
ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true atmos terraform apply hello-world -s test
```

`atmos say something` is a nested custom command defined in `atmos.yaml`. The `atmos workflow ...`
commands run workflow definitions from `stacks/workflows/say.yaml`. The Terraform
example runs `components/terraform/hello-world` and fires an
`after.terraform.apply` hook with `when: always`, so the message is shown for
both successful and failed applies.

## How `say` Works

```yaml
steps:
  - name: notify
    type: say
    content: "Deployment is complete"
    voice: [Samantha, Microsoft Zira, en-us]   # first installed voice wins
    rate: normal                                # slow | normal | fast
    print: fallback                             # fallback | always | never
```

## Terraform Apply Hook

The `hello-world` component demonstrates `say` as a lifecycle hook:

```yaml
hooks:
  announce-apply:
    kind: step
    type: say
    events:
      - after.terraform.apply
    when: always
    with:
      print: always
      content: >-
        {{ if eq .status "success" -}}
        Terraform apply for {{ .atmos_component }} in {{ .stack }} was successful.
        {{- else -}}
        Terraform apply for {{ .atmos_component }} in {{ .stack }} was not successful.
        {{- end }}
```

Use `print: always` so the message is visible in logs even when text-to-speech
is available. The hook receives the apply outcome as `{{ .status }}` and runs
for both success and failure because `when: always` is set.

### Cross-platform voices (`voice`)

Voice selection works like a CSS `font-family` stack: you provide an **ordered list** of candidate voices and the first one actually installed on the host is used. If none match, the engine's default voice is used.

Voice names are platform-specific, so a portable stack mixes them:

| Platform | Example voice names |
|----------|---------------------|
| macOS    | `Samantha`, `Alex`, `Daniel` |
| Windows  | `Zira`, `David` (matches `Microsoft Zira Desktop`) |
| Linux    | `en-us`, `en-gb` (espeak language codes) |

List installed voices with `say -v "?"` (macOS), `espeak --voices` (Linux), or via PowerShell's `GetInstalledVoices()` (Windows).

### Speech rate (`rate`)

`slow`, `normal` (default), or `fast`, mapped to each engine's native cadence.

### Print policy (`print`)

| Value | Behavior |
|-------|----------|
| `fallback` (default) | Speak when possible; otherwise print the message as a Markdown blockquote. |
| `always` | Always print the blockquote **and** also speak when possible. |
| `never` | Speak when possible; otherwise stay silent (no printed output). |

## CI/CD Considerations

`say` never fails a workflow. In CI it skips speech and follows the `print` policy (`fallback` by default prints the message), so you can leave `say` steps in workflows that run both locally and in pipelines.
