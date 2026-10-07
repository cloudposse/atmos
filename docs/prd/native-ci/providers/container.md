# Native CI Integration - Container Component

> Related: [Overview](../overview.md) | [Generic Provider](./generic.md) | [Interfaces](../framework/interfaces.md)

## Overview (IMPLEMENTED)

The container component reports every image it builds or pushes through the `ci.Reporter` seam. It does not detect providers or check the `ci.*` destination gates itself; the reporter routes each write to the detected provider, or renders it locally.

Two call sites report images:

- The container component executor (`pkg/component/container`) after `build` and `push`.
- The container workflow step handler (`pkg/runner/step`) after a `build` or `push` step.

Both are best-effort. A reporter or template failure is logged and never fails the build or push.

## What is reported

For each image Atmos inspects the image and renders one Markdown document, then:

1. Calls `Reporter.Summary(markdown)` to append it to the job summary.
2. When the summary reached a CI provider (the receipt is not `Local`), calls `Reporter.Comment` with the same Markdown, so each image gets one pull request comment. The reporter gates the comment on `ci.comments.enabled`, which is off by default.

The comment key is `container:image:<image>`, where `<image>` is the image reference passed to the summary, or the first repo tag when the reference is empty. The key makes the comment an upsert: a later run for the same image updates the existing comment instead of adding another. Pushing to several registries produces one comment per pushed reference.

Before inspecting an image, both call sites check `ci.SummaryEnabled` so that no inspection runs when nothing would be reported.

## Template

The Markdown comes from the embedded default template `pkg/ci/templates/container_default.md`, loaded through `templates.Loader` with component type `container` and command `image`. Override order:

1. `ci.templates.container.image`, a file name resolved under `ci.templates.base_path`.
2. `<ci.templates.base_path>/container/image.md`.
3. The embedded default.

The default template output is byte-identical to the renderer it replaced, and golden files under `pkg/container/testdata/image_summary/` lock that in.

### Template data

The template receives `container.ImageSummaryData`. Every string field is already Markdown-ready: escaped for its position and, for table cells, wrapped in a code span or link. Missing values render as `` `n/a` ``.

| Field | Content |
|-------|---------|
| `Image` | Escaped image reference for the heading. |
| `Badges` | Size, license, architecture, and OS values, safe inside a code span. |
| `Description` | Escaped OCI description label, empty when unset. |
| `Tag`, `Digest`, `ID`, `Revision`, `Source` | Table cells. |
| `Entrypoint`, `Command`, `StopSignal`, `StorageDriver`, `ExposedPorts` | Table cells. |
| `Env` | Rows (`Name`, `Value`) sorted by variable name. |
| `Labels` | Rows (`Name`, `Value`) sorted by label name. |
| `Layers` | Rows (`Index`, `Digest`) in image order. |
| `RawJSON` | Unescaped inspect output, empty when unavailable. |

## Configuration

`ci.templates.container` is a map from a container operation name to a template file name. Only `image` is defined. The schema field is `CITemplatesConfig.Container`.
