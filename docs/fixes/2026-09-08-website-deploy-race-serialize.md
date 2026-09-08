# Fix: atmos.tools went blank after two concurrent prod deploys raced on `s3 sync --delete`

**Date:** 2026-09-08

## Summary

atmos.tools rendered only the navbar and hero shell. The live `index.html`
referenced `/assets/js/runtime~main.99cb7223.js` and `/assets/js/main.7524adc2.js`,
and both returned 404. CSS loaded, so the page painted, but the React bundle never
booted. Two `Website Deploy Prod` runs had executed at the same time and the second
one's `--delete` pass removed the bundles the first one's `index.html` pointed at.

The fix serializes the deploy workflow with a GitHub Actions `concurrency` group,
adds the same per-PR guard to the preview deploy, and separately corrects the Sentry
loader key that was producing a CORS error in the same console.

## Context

The merge queue landed #3024 (`2b4c827269`) and #3058 (`82f5413e85`) on `main` three
minutes apart. Each push to `main` triggers `.github/workflows/website-deploy-prod.yml`,
which had no `concurrency:` block. Both runs built the site and then ran
`.github/scripts/s3-deploy-with-charset.sh`, i.e. `aws s3 sync --delete` against the
single shared origin prefix `s3://cplive-plat-ue2-prod-atmos-docs-origin/`.

Reconstructed from the two run logs (34179330617 = run A for #3024, 34179504121 = run B
for #3058), all times UTC on 2026-09-08:

| Time     | Run | Action |
|----------|-----|--------|
| 02:26:47 | A   | sync starts (computes its plan against the S3 listing) |
| 02:27:30 | A   | uploads `main.7524adc2.js`, `runtime~main.99cb7223.js`; deletes the previous deploy's bundles |
| 02:27:55 | A   | uploads `index.html` (references A's bundles) |
| 02:27:55 | B   | sync starts; its plan was computed against a listing that predates A's `index.html`, so B never re-uploads `index.html` |
| 02:27:58 | B   | `--delete` removes A's `main.7524adc2.js` and `runtime~main.99cb7223.js` (not in B's build); uploads B's `main.dff532b8.js`, `runtime~main.3038533a.js` |

End state on S3: A's `index.html` pointing at bundles B deleted. Both runs reported
success. The site stayed broken until another deploy rewrote `index.html`.

Docusaurus content-hashes every bundle, so any two builds from different commits
produce different asset names, and `sync --delete` on a shared prefix is only safe
when exactly one deploy runs at a time.

A second, unrelated error was visible in the same browser console: the Sentry loader
`<script>` was blocked by CORS. `docusaurus-plugin-sentry` v2 builds the tag as
`https://js.sentry-cdn.com/${opts.DSN}.min.js`, so the `DSN` option must be the Sentry
Loader Script public key. `website/docusaurus.config.js` passed the full DSN
(`https://<key>@o56155.ingest.us.sentry.io/4507472203087872`), which produced a bogus
URL that 404s. This did not cause the blank page; it just meant Sentry never loaded.

## Changes

- `.github/workflows/website-deploy-prod.yml`: add a workflow-level `concurrency`
  group keyed on `${{ github.workflow }}` with `cancel-in-progress: false`, plus a
  comment explaining the race. The in-flight deploy always finishes; the newest
  pending run queues behind it; older pending runs are superseded. Cancelling
  in-progress runs was deliberately rejected because aborting mid-sync leaves a
  half-uploaded site, which is the same failure class.
- `.github/workflows/website-preview-deploy.yml`: same guard keyed per PR
  (`${{ github.workflow }}-pr-<number>`), since each preview deploys with
  `sync --delete` into its own `pr-<N>/` prefix and two builds of one PR can race
  the same way.
- `website/docusaurus.config.js`: pass the Sentry Loader public key
  (`b022344b0e7cc96f803033fff3b377ee`) instead of the full DSN, with a comment
  explaining why the plugin needs the key.
- No change to `s3-deploy-with-charset.sh`: reordering uploads and deletes inside one
  run cannot fix a race between two runs.

## Validation

- Confirmed the race from both run logs (upload and delete lines quoted in the table
  above) and from the live site: `index.html` `last-modified` 02:30:50, both referenced
  bundles 404, `styles.4266732d.css` 200.
- Confirmed the Sentry fix without a deploy: the corrected URL
  `https://js.sentry-cdn.com/b022344b0e7cc96f803033fff3b377ee.min.js` returns 200
  `text/javascript`; the previous URL returns 404.
- Immediate remediation: `Website Deploy Prod` re-run from `main` via
  `workflow_dispatch` by the maintainer so a single run rewrites `index.html` and
  assets consistently (the automated session was not permitted to dispatch it).
  Merging this PR triggers the same push-to-main deploy as a fallback.
- `actionlint` on both modified workflows.
- Rendered the Sentry head tag locally through the installed plugin
  (`docusaurus-plugin-sentry/lib/tagsGenerator.js`) with the new option and confirmed
  the generated `src` is the corrected loader URL.

## Follow-ups

None.
