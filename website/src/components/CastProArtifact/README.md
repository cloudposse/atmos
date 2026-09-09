# CastProArtifact

URL builders and a polling hook for the Atmos Pro cast-rendering service, shared by
`CastProDownload`, `CastProEmbed`, and `CastShareLink`.

## The service

`GET https://atmos-pro.com/casts/{owner}/{repo}/{ref}/{path}.cast.{gif|mp4|svg|webm}` renders any
`.cast` (asciicast) file in a public GitHub repo on demand and is CORS-enabled for browser `fetch`.
It responds one of three ways:

- **302** once the artifact is ready.
- **202** + `Retry-After` while still rendering.
- **400/500** with a JSON error body otherwise.

A sibling `.cast` URL (no format suffix) returns an HTML page with an embedded player, for
`<iframe>` use.

## `url.mjs`

Pure, framework-agnostic URL builders (unit-tested in `url.test.mjs` via `node --test`):

```js
import { buildArtifactUrl, buildEmbedUrl, CAST_FORMATS } from './url.mjs';

buildArtifactUrl({ ref: 'main', path: 'examples/quickstart.cast', format: 'gif' });
// => "https://atmos-pro.com/casts/cloudposse/atmos/main/examples/quickstart.cast.gif"

buildEmbedUrl({ ref: 'main', path: 'examples/quickstart.cast' });
// => "https://atmos-pro.com/casts/cloudposse/atmos/main/examples/quickstart.cast"
```

`owner`/`repo` default to `cloudposse/atmos`. A `ref` containing a slash (e.g. `feature/foo`) is
passed via `?ref=` instead of the path, since it can't be embedded there unambiguously.

## `useCastArtifact.ts`

Drives the download flow for a single `owner`/`repo`/`ref`/`path`/`format` combination: calling
`start()` checks the artifact, polls on `Retry-After` while rendering (capped at ~60s total), and
navigates the browser to a forced download once ready. Each call to `start()` gets its own request
generation, so a stale request (e.g. from a `path`/`ref` that changed mid-poll) can never apply its
result after a newer one has started.

## Components that use this

- **`../CastProDownload`** — a "Download ▾" split button offering GIF/MP4/SVG/WEBM.
- **`../CastProEmbed`** — an `<iframe>` wrapping the hosted player, via `buildEmbedUrl`.
- **`../CastShareLink`** — copies a page link, or the embed URL from `buildEmbedUrl`, to the
  clipboard.

See `website/src/pages/cast-pro-demo.tsx` for a runnable example of all three, and
`website/src/components/FileBrowser/DirectoryPage.tsx` for how the example browser wires
`CastProDownload`/`CastShareLink` next to a rendered `CastPlayer`.
