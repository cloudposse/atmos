# Hands-on Labs

**Hands-on Labs** are complete, copy-and-run Atmos projects that combine multiple Atmos features into a working solution
for a real-world use case.

Where an [Example](https://atmos.tools/examples) demonstrates *one* Atmos feature in isolation, a **Lab** demonstrates
*an entire workflow* — components, stacks, custom commands, CI/CD pipelines, and the surrounding glue — assembled into a
single project you can clone, configure, and run.

|                         | [Examples](https://atmos.tools/examples) | [Gists](https://atmos.tools/gists) | Hands-on Labs                                 |
|-------------------------|------------------------------------------|------------------------------------|-----------------------------------------------|
| **Scope**               | One feature                              | One concept (unmaintained)         | One complete use case                         |
| **Size**                | Minimal (scan in 1 min)                  | Small                              | Large (a whole project)                       |
| **Goal**                | "How does feature X work?"               | "Here's an idea"                   | "Give me a working starting point I can copy" |
| **Maintained / tested** | ✅                                        | ❌                                  | ✅                                             |
| **Runnable as-is**      | ✅ (mock-only)                            | ⚠️ adaptation needed               | ✅ (real, with documented prerequisites)       |

## How to use a Lab

Each Lab is a self-contained directory. To use one:

```shell
# Clone this repo
git clone git@github.com:cloudposse/atmos.git

# Copy a lab into a new repo of your own
cp -r atmos/labs/<lab-name>/ my-project/
cd my-project/

# Follow the Lab's README to set a few documented inputs, then run it
```

Every Lab is **vendor-neutral**: there are no organization-, person-, or account-specific identifiers. All
environment-specific values (regions, account IDs, ARNs, VPC/subnet IDs, names) are parameterized inputs with documented
placeholder defaults.

## Available Labs

```shell
1. └── aws-ami-packer-github-actions/   # Build, scan, approve & share hardened AWS AMIs with Atmos + Packer + GitHub Actions.
```

### [`aws-ami-packer-github-actions`](aws-ami-packer-github-actions/)

Build a hardened Amazon Linux 2023 AMI with Packer orchestrated by Atmos, validate it, optionally scan it, gate
promotion behind a manual approval, tag the approved image, and share it across AWS accounts — all driven from a GitHub
Actions pipeline and a set of `atmos ami` custom commands.

**Atmos features combined:** Packer components, stacks for Packer, Go templating, nested custom commands, and GitHub
Actions CI/CD with a manual approval gate.

## Conventions

Labs follow a consistent structure so they stay trustworthy:

- **Self-contained** — no dependency on files outside the Lab directory.
- **Parameterized** — all environment-specific values are inputs with placeholder defaults.
- **Vendor-neutral** — neutral placeholders only (e.g., `namespace`, `123456789012`, `arn:aws:...:EXAMPLE`).
- **Runnable with a standard account** — steps needing proprietary services are optional and clearly gated.
- **Documented** — `Overview → Architecture → Prerequisites → Run → Customize → Clean up → Learn More`.
- **CI-validated** — linted and statically validated (`atmos validate stacks`, `atmos describe`).
