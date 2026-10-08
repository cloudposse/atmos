# Native CI Integration - Container Component

> Related: [Overview](../overview.md) | [Generic Provider](./generic.md) | [Interfaces](../framework/interfaces.md)

## Overview (IMPLEMENTED)

The container component reports every image it builds or pushes through the `ci.Reporter` seam. It does not detect providers or check the destination gates of the job summary itself; the reporter routes each write to the detected provider, or renders it locally.

Two call sites report images, and both delegate to one implementation, `pkg/component/container/imagesummary`:

- The container component executor (`pkg/component/container`) after `build` and `push`.
- The container workflow step handler (`pkg/runner/step`) after a `build` or `push` step.

Both are best-effort. A reporter or template failure is logged and never fails the build or push.

## What is reported

For each image Atmos inspects the image and renders one Markdown document, then:

1. Calls `Reporter.Summary(markdown)` to append it to the job summary. The job summary keeps the full report.
2. Calls `Reporter.Comment` only when the summary reached a CI provider (the receipt is not `Local`) and `ci.CommentsEnabled` is true. `ci.comments.enabled` is off by default.

The caller checks `ci.CommentsEnabled` itself instead of relying on the reporter's gate. A reporter call that is gated off renders the whole body as a local "PR comment preview" in the log, so calling it with comments off would dump the full report (including the raw inspect JSON) into the log on every build and push. With comments off, nothing is previewed.

The comment is posted with the reporter's default target, so it lands on the pull request when its number is known and on the run's commit otherwise (`target=auto`).

### Comment key

The comment key is `container:image:<registry/repository>`: the image reference with its tag and digest removed. A registry port is kept (`localhost:5000/app:1` becomes `localhost:5000/app`). The reference is the one passed to the summary, or the first repo tag when it is empty; with neither, no comment is posted because the key would not identify the image.

The key makes the comment an upsert with one comment per image. A SHA or version tag no longer creates a comment per push, and several tags of one image share a comment.

### Several refs in one run

Each ref is appended to the job summary as it is added. Pull request comments are queued and posted once per image repository when the run ends (a deferred flush, so refs pushed before a later failure are still posted). The comment body lists the summary of every ref pushed for that image in the run, separated by horizontal rules. Pushing to several registries produces one comment per registry repository. Separate build and push operations each update the same comment in turn, so the comment shows the latest operation; the job summary keeps all of them.

### Size limit

GitHub rejects comment bodies over 65,536 characters with a 422. The comment body is truncated at 65,000 characters (counted in characters, never splitting one) and ends with `Comment truncated; see the job summary for the full report.` A code fence or collapsible section left open by the cut is closed first. The job summary keeps the full content.

### Failures

A comment that cannot be posted logs at Warn with the image repository and the error. It never fails the build or push.

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
