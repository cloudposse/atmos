# CastProArtifact

URL builders and a polling hook for the Atmos Pro cast-rendering service, shared by
`CastProDownload`, `CastProEmbed`, and `CastShareLink`.

## The service

`GET https://atmos-pro.com/casts/{owner}/{repo}/{ref}/{path}.cast.{gif|mp4|svg|webm}` renders any
`.cast` (asciicast) file in a public GitHub repo on demand and is CORS-enabled for browser `fetch`.
Poll it with `Accept: application/json` so it answers immediately instead of holding the
connection open, and never sends artifact bytes while still rendering:

- **202** + `Retry-After` (ignored — see below) + JSON body while queued/rendering:
  `data.artifacts[0].status` is `"queued" | "processing" | "ready" | "failed"`; an optional
  `data.artifacts[0].progress` (`{ percent, stage }`) may appear once the service starts sending it.
- **200** + JSON body once ready — `data.artifacts[0].status === "ready"` is the readiness
  signal, using the same `data.artifacts[0]` shape as the 202 case above. `artifacts[0].url` is a
  tokened direct link to the artifact (unused here); `blobUrl` is always `null`. Some responses
  may still come back as the *rendered artifact itself* (`Content-Type: video/mp4`, `image/gif`,
  etc.) rather than this JSON body — `polling.mjs` checks `Content-Type` and, for that non-JSON
  shape, aborts the fetch as soon as the status line arrives so a poll never transfers the full
  file, treating it as ready without reading it.
- **400/404/500** with a JSON `{ success: false, error }` body — terminal failure, readable
  cross-origin; `error` is human-readable and safe to show verbatim (e.g. a 404 reads "Cast file
  was not found at the requested commit and path").

A sibling `.cast` URL (no format suffix) returns an HTML page with an embedded player, for
`<iframe>` use.

**Download** — once ready, navigate to the same URL with `?download=1`; it responds `200` with
`Content-Disposition: attachment`, so `window.location.href = …` downloads without leaving the
page.

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

## `polling.mjs`

The framework-agnostic polling engine (`createPoller`), kept free of React so it's directly
unit-testable with `node --test`, a mocked `fetch`, and fake timers (`polling.test.mjs`). Polls
every 3s while rendering — long casts can take several minutes, so it deliberately ignores the
service's 30s `Retry-After` on the cheap JSON path. Past 13 minutes of total wait it slows to
every 10s and marks the state "taking longer than usual"; it gives up only at a hard 30-minute
ceiling, with a message that says the render may still be running rather than that it failed.

## `useCastArtifact.ts`

A thin React wrapper around `createPoller` for a single `owner`/`repo`/`ref`/`path`/`format`
combination: calling `start()` kicks off polling and, once the artifact is ready, navigates the
browser to a forced download (`?download=1`). Each call to `start()` gets its own request
generation, so a stale request (e.g. from a `path`/`ref` that changed mid-poll) can never apply its
result — or trigger a download — after a newer one has started.

## Components that use this

- **`../CastProDownload`** — a "Download ▾" split button offering GIF/MP4/SVG/WEBM.
- **`../CastProEmbed`** — an `<iframe>` wrapping the hosted player, via `buildEmbedUrl`.
- **`../CastShareLink`** — copies a page link, or the embed URL from `buildEmbedUrl`, to the
  clipboard.

See `website/src/pages/cast-pro-demo.tsx` for a runnable example of `CastProDownload` and
`CastProEmbed`, and `website/src/components/FileBrowser/DirectoryPage.tsx` for how the example
browser wires all three — `CastProDownload` and `CastShareLink` next to a rendered `CastPlayer`.
