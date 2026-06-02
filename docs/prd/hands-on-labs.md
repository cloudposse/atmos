# Hands-on Labs - Product Requirements Document

**Status:** Implemented
**Version:** 1.1
**Last Updated:** 2026-06-02

---

## Executive Summary

Hands-on Labs are a new content type for the Atmos documentation site: complete, end-to-end, copy-and-run reference projects that show how to combine multiple Atmos features into a working solution for a real-world use case.

Where an **Example** demonstrates *one* Atmos feature in isolation (so a reader can scan it in a minute), a **Lab** demonstrates *an entire workflow* — Atmos components, stacks, custom commands, CI/CD pipelines, and the surrounding glue — assembled into a single project the reader can clone, configure, and run.

**The first Lab** is **"Build, Scan, Approve & Share AWS AMIs with Atmos + Packer + GitHub Actions"** — a production-shaped pipeline that builds a hardened Amazon Linux 2023 AMI with Packer (orchestrated by Atmos), runs an optional security scan, gates promotion behind a manual approval, tags the approved image, and shares it across AWS accounts.

**Key distinction:**

| | Examples | Gists | Hands-on Labs |
|---|---|---|---|
| **Scope** | One feature | One concept (unmaintained) | One complete use case |
| **Size** | Minimal (scan in 1 min) | Small | Large (a whole project) |
| **Goal** | "How does feature X work?" | "Here's an idea" | "Give me a working starting point I can copy" |
| **Maintained / tested** | ✅ | ❌ | ✅ |
| **Runnable as-is** | ✅ (mock-only) | ⚠️ adaptation needed | ✅ (real, with documented prerequisites) |

---

## Vision & Strategic Goals

### Vision Statement

**"Clone a Lab, change a few values, and you have a working Atmos-powered solution in production shape."**

### Strategic Goals

1. **Reduce time-to-value** — give users a complete, opinionated starting point instead of a blank project they must assemble from a dozen separate examples.
2. **Show Atmos at full strength** — demonstrate how components, stacks, custom commands, templating, and CI integrations compose into a real solution.
3. **Bridge the gap** — fill the space between bite-sized Examples and a full greenfield implementation.
4. **Stay trustworthy** — every Lab is maintained and CI-validated, so what users copy actually works.
5. **Be reusable** — Labs are vendor-neutral and self-contained; anyone can run them without proprietary credentials.

### Non-Goals

- Labs are **not** a replacement for Examples. Examples must stay small and single-feature.
- Labs are **not** marketing demos or slideware — they are real, runnable code with documentation.
- Labs are **not** full product reference architectures with every production concern solved; they are focused, complete starting points.

---

## Problem Statement

Today the site offers two content tiers:

- **Examples** (`/examples`) — minimal, single-feature, mock-only projects. Great for learning one concept quickly. Each is intentionally tiny so a reader can understand it in a minute.
- **Gists** (`/gists`) — community-contributed snippets that illustrate a concept but are not actively maintained.

There is no tier that answers the most common adoption question: *"I understand the individual pieces — now show me a complete, working project that ties them together for my use case, so I can copy it and start from there."*

Users who want to do something realistic (e.g., build and govern machine images, run a GitOps pipeline, manage multi-account deployments) currently have to stitch together knowledge from many separate Examples plus external blog posts. This is the highest-friction part of onboarding.

**Hands-on Labs** close that gap.

---

## What Is a Hands-on Lab

A Lab is a self-contained project directory plus a narrative walkthrough. It has these defining properties:

1. **Complete** — contains everything needed to run: `atmos.yaml`, components, stacks, scripts, CI workflows, and docs.
2. **Copy-and-run** — a reader can copy the directory into a new repo, set a small number of documented inputs, and run it.
3. **Real, not mocked** — Labs perform actual work (e.g., build a real AMI). Any step that would require proprietary credentials (private package repos, commercial scanners) is **optional and pluggable**, with a working default that needs only a standard cloud account.
4. **Multi-feature** — a Lab deliberately combines several Atmos capabilities (this is what separates it from an Example).
5. **Maintained & tested** — validated in CI to the extent possible without cloud credentials (lint, `atmos validate stacks`, `atmos describe`, template rendering, workflow/script shellcheck).
6. **Vendor-neutral** — no organization-, person-, or account-specific identifiers. Inputs are parameterized and documented.
7. **Well-documented** — a walkthrough explains *what* it does, *why* it's structured that way, *how* to run it, and *what to change* for your environment.

---

## Information Architecture & Site Integration

### Top-level navigation

Add a **"Labs"** item to the site navbar, positioned immediately after **"Examples"**:

```
Learn | Reference | Examples | Labs | Pro | Community | Changelog | Roadmap
```

### Routing & content model

Labs reuse the existing **`file-browser`** plugin already powering `/examples` and `/gists`, so the runnable project files are browsable and copyable in the site, with "view on GitHub" links. A Lab also has a narrative walkthrough page.

Two integration pieces:

1. **Source directory** — a new top-level `labs/` directory in the repo (sibling of `examples/` and `gists/`). Each subdirectory is one Lab and holds the complete runnable project.

2. **`file-browser` plugin instance** in `website/docusaurus.config.js`, mirroring the `examples` and `gists` instances:

   ```js
   [
     path.resolve(__dirname, 'plugins', 'file-browser'),
     {
       id: 'labs',
       sourceDir: '../labs',
       routeBasePath: '/labs',
       title: 'Hands-on Labs',
       description: 'Complete, copy-and-run Atmos projects that combine multiple features into a working solution for a real-world use case.',
       githubRepo: 'cloudposse/atmos',
       githubBranch: 'main',
       githubPath: 'labs',
     },
   ],
   ```

3. **Navbar entry** in the `navbar.items` array:

   ```js
   { to: '/labs', position: 'left', label: 'Labs' },
   ```

4. **Narrative walkthrough docs** — each Lab gets a walkthrough under `website/docs/labs/<lab-name>.mdx` (rendered in the main docs site, linked from the `/labs` browser via the plugin's `DOCS_MAP`). The walkthrough uses `EmbedFile` to embed real files from the Lab so docs and code never drift.

5. **File-browser metadata** — register the Lab in the plugin's `TAGS_MAP` and `DOCS_MAP` (`website/plugins/file-browser/index.js`), the same mechanism Examples use, so the Lab shows with the correct category tag and links to its walkthrough.

### Repository layout

```
labs/                                  # NEW top-level directory (sibling of examples/, gists/)
├── README.md                          # Index of all Labs
└── <lab-name>/                        # One complete, runnable project per Lab
    ├── README.md                      # Lab overview + run instructions
    ├── atmos.yaml                     # Atmos configuration
    ├── components/                    # Terraform / Packer / Helmfile components
    ├── stacks/                        # Stack manifests
    ├── scripts/                       # Supporting scripts (provisioners, helpers)
    ├── .github/workflows/             # CI/CD pipeline(s)
    └── docs/                          # Lab-specific reference policies + customization checklist

website/docs/labs/<lab-name>.mdx       # Narrative walkthrough (EmbedFile the real files)
```

### Relationship to Examples and Gists

- **Examples stay as-is**: minimal, single-feature, scan-in-a-minute. The Examples guidance ("each example should cover just one Atmos feature") is unchanged. See the example-creator agent (`.claude/agents/example-creator.md`).
- **Labs are the new "everything assembled" tier**, larger and multi-feature.
- A Lab's walkthrough should **link back to the relevant Examples** for each individual feature it uses, so readers can drill from the assembled solution down into the focused explanation of any single piece.

---

## Lab Authoring Conventions

These conventions keep Labs consistent and trustworthy.

1. **Self-contained** — no dependency on files outside the Lab directory; no shared/vendored harnesses.
2. **Parameterized inputs** — all environment-specific values (regions, account IDs, ARNs, VPC/subnet IDs, names) are inputs (stack vars, env vars, or workflow inputs), never hardcoded. Provide sensible placeholder defaults and document each.
3. **Vendor-neutral** — no organization names, personal names, or real account identifiers anywhere. Use neutral placeholders (e.g., `namespace`, `123456789012`, `arn:aws:...:EXAMPLE`).
4. **Runnable with a standard account** — the default path requires only a standard cloud account/CLI. Steps needing proprietary services (private package repositories, commercial security scanners) are isolated, optional, and clearly gated behind a flag/variable so the Lab runs without them.
5. **Documented prerequisites** — list required tools and versions (Atmos, Packer/OpenTofu, cloud CLI), required permissions, and any one-time setup.
6. **Idempotent & safe** — destructive operations require explicit confirmation; cleanup steps are provided.
7. **Comment density matches surrounding code** — scripts and HCL are commented to explain *why*, suitable for readers learning the pattern.
8. **README structure** — Overview → Architecture → Prerequisites → Run → Customize → Clean up → Learn More (links to relevant Examples and docs).
9. **CI-validated** — see Testing & CI below.

---

## First Lab: AWS AMI Pipeline with Atmos + Packer + GitHub Actions

### Lab name (proposed)

`labs/aws-ami-packer-github-actions/`

### One-line description

Build a hardened Amazon Linux 2023 AMI with Packer orchestrated by Atmos, validate it, optionally scan it, gate promotion behind a manual approval, tag the approved image, and share it across AWS accounts — all driven from a GitHub Actions pipeline and a set of Atmos custom commands.

### What it teaches (Atmos features combined)

- **Packer components in Atmos** — `components.packer` configuration, `atmos packer init/validate/build/output`.
- **Stacks for Packer** — passing build configuration (source AMI, instance type, networking, encryption, tags, provisioner script lists) as stack vars.
- **Go templating in stacks** — resolving inputs (e.g., source AMI name) from environment variables at build time.
- **Nested custom commands** — an `atmos ami <subcommand>` command tree (get image id, tag, list/get tags, launch/terminate instances, share) that wraps helper scripts and templated Atmos calls.
- **CI/CD integration** — a multi-stage GitHub Actions pipeline using OIDC auth, ephemeral runners, and a manual approval gate.
- **Governance pattern** — promoting images only after approval and enforcing "launch only approved images" via tag-based IAM/SCP conditions (documented as a reference policy).

### Architecture overview

```text
        ┌──────────────────────────────────────────────────────────────┐
        │ GitHub Actions Pipeline (.github/workflows/ami.yml)          │
        ├──────────────────────────────────────────────────────────────┤
        │ build (Packer via Atmos) → launch test instance              │
        │   → health check → [optional] scan                           │
        │   → ⏸ manual approval gate (GitHub Environment)              │
        │   → tag ScanStatus=approved → share AMI → cleanup            │
        └───────────────┬──────────────────────────────────────────────┘
                        │ atmos packer build / atmos ami …
        ┌───────────────▼──────────────────────────────────────────────┐
        │ Atmos                                                        │
        │  • Packer component (components/packer/<image>/main.pkr.hcl)  │
        │  • Stack (stacks/<image>.yaml) — all build inputs as vars     │
        │  • Custom commands (atmos ami …) → scripts/atmos/*.sh         │
        └───────────────┬──────────────────────────────────────────────┘
                        │ packer build
        ┌───────────────▼──────────────────────────────────────────────┐
        │ Packer (amazon-ebs)                                          │
        │  provisioners: patch-os → harden → [optional] scan agent     │
        │               → install-packages → finalize                  │
        │  post-processor: manifest.json                               │
        └──────────────────────────────────────────────────────────────┘
```

### Lab contents

**`atmos.yaml`**
- Configures the Packer component type (`components.packer.command`, `base_path`).
- Defines the nested `atmos ami` custom command tree. Subcommands delegate to `scripts/atmos/*.sh` and use templated `atmos packer output ... -q <query>` to resolve the built image ID from the Packer manifest.
- Standard stacks config (`name_template`, included/excluded paths) and templating enabled.

**`components/packer/<image>/main.pkr.hcl`**
- `amazon-ebs` builder with a parameterized source-AMI data lookup, instance type, networking (VPC/subnet, temporary security group CIDRs), KMS encryption toggle, block-device sizing, assume-role support, image-sharing attributes, and merged tags.
- Dynamic shell provisioners driven by a stack-provided ordered list of scripts.
- `manifest` post-processor for machine-readable build output.
- All credentials/inputs read from variables/`env()`; no secrets in the template.

**`stacks/<image>.yaml`**
- A single stack that sets all build inputs as vars: source AMI name (templated from env), image name, region, instance type, volume size/type, encryption + KMS key, networking, sharing targets, tags (including an initial `ScanStatus: pending`), and the ordered provisioner script list.
- Placeholder values for all account-specific identifiers, clearly marked.

**`components/packer/<image>/scripts/`** (provisioners, run during the build, in order)
- `patch-os.sh` — apply OS updates (no reboot; the new kernel activates when instances launch from the AMI).
- `harden.sh` — baseline OS hardening (SSH config, sysctl, audit rules, optional firewall/SELinux/integrity tooling), all toggle-driven.
- `install-scan-agent.sh` *(optional)* — install/activate a vulnerability-scanning agent from a private repository. Disabled by default; documented as the integration point for any scanner.
- `install-packages.sh` — install and enable the runtime packages/services the image should ship with (presented as an editable list).
- `finalize.sh` — clean instance-specific data (cloud-init, scanner host IDs, logs) so instances launched from the AMI boot clean.

**`scripts/atmos/`** (host-side helpers backing `atmos ami` subcommands)
- Get image ID from Packer output, tag image, list/get tags, launch/terminate test instances, list instances by image, and share the image (and its snapshots, with optional KMS grants) to a list of accounts. Account lists and IDs are placeholders/inputs.

**`.github/workflows/`**
- **`ami.yml`** — the main pipeline (build → launch → health-check → optional scan → approval gate → tag → share → cleanup). Uses OIDC role assumption, ephemeral runners, a GitHub Environment for the manual approval gate, and step summaries. Inputs (source AMI name, etc.) via `workflow_dispatch`.

**`docs/`** (Lab-specific)
- A reference IAM identity policy, an OIDC trust policy, and an org SCP for "launch only `ScanStatus=approved` images", plus a customization checklist.

### Prerequisites (documented in the Lab README)

- Atmos, Packer (or the configured build command), and the cloud CLI installed (pinned versions listed).
- A standard AWS account and credentials with permission to build AMIs and launch EC2 instances (a reference IAM policy is included).
- For CI: an OIDC-federated IAM role and a GitHub Environment configured for the approval gate (setup steps included).
- Optional: a private package repository and a security-scanning subscription **only** if the optional scan step is enabled.

### How a reader runs it

```bash
# 1. Copy the lab into a new repo
cp -r labs/aws-ami-packer-github-actions/ my-ami-pipeline/
cd my-ami-pipeline/

# 2. Edit stacks/<image>.yaml: set region, networking, KMS, sharing targets, tags
# 3. Build locally
atmos packer init  <image> -s <stack>
atmos packer build <image> -s <stack>

# 4. Inspect / operate the result with custom commands
atmos ami get-ami-id  <image> -s <stack>
atmos ami list-tags   <image> -s <stack>
atmos ami launch-instance <image> -s <stack> --type t3.small

# 5. Or run the full governed pipeline from GitHub Actions (workflow_dispatch)
```

### What the reader customizes

- Stack vars (region, instance type, networking, encryption/KMS, image name, sharing targets, tags).
- The provisioner script list (which hardening/packages to apply).
- Whether the optional scan step is enabled.
- The approval gate's GitHub Environment reviewers.
- The reference tag-based launch-restriction policy.

---

## Testing & CI

Labs are validated to the maximum extent possible **without** cloud credentials:

1. **Lint** — `golangci`-equivalent for shell (`shellcheck`), HCL formatting (`packer fmt`/`terraform fmt -check`), YAML lint, and Atmos manifest schema validation.
2. **Atmos static checks** — `atmos validate stacks`, `atmos describe stacks`, `atmos describe component`, and template rendering for each Lab in a CI job, ensuring the Atmos wiring is valid.
3. **Custom-command resolution** — assert the `atmos ami` command tree loads and help renders (no execution of cloud calls).
4. **Workflow lint** — GitHub Actions workflow syntax validation.
5. **Docs build** — `cd website && npm run build` to verify the walkthrough, `EmbedFile` references, and file-browser registration resolve.
6. **Cloud-dependent steps** — clearly marked as not run in CI; the Lab README documents how to run them in a real account.

A Lab CI matrix entry mirrors how Examples are wired for CI (see the Examples testing strategy and the example-creator agent).

---

## References

- Examples model and conventions: `examples/`, `.claude/agents/example-creator.md`
- Existing browsable content plugins: `website/plugins/file-browser/`, `/examples`, `/gists`
- Site navigation & plugin config: `website/docusaurus.config.js`
- Atmos Packer commands: `/cli/commands/packer/*`
- Custom commands: `/cli/configuration/commands`
- CI/CD integration: `/ci`
