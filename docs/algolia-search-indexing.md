# Algolia Search Indexing

This document describes how Algolia search indexing is configured for the `atmos.tools` documentation site.

## Overview

The `atmos.tools` documentation uses [Algolia DocSearch](https://docsearch.algolia.com/) for search. Search records are produced by the Algolia Crawler. The crawler configuration is managed in this repository, validated on pull requests, and deployed only after changes merge to `main`.

## Architecture

```text
┌──────────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│ website/algolia      │────▶│   Algolia Crawler    │────▶│  Algolia Index  │
│ crawler config       │     │   (cloud-hosted)     │     │  (atmos.tools)  │
└──────────────────────┘     └──────────────────────┘     └─────────────────┘
           │                             ▲
           │                             │
           ▼                             │
┌──────────────────────┐     ┌──────────────────────┐
│ Algolia workflow     │     │ Website prod deploy  │
│ config/settings      │     │ content reindex      │
└──────────────────────┘     └──────────────────────┘
```

## Responsibilities

### Crawler Configuration

`website/algolia/` owns the crawler configuration, index settings, tests, and deploy script.

- `atmos-tools.crawler.js`: canonical crawler config.
- `deploy-crawler-config.mjs`: deploys crawler config and live index settings.
- `atmos-tools.crawler.test.mjs`: local config mechanics tests.
- `search-relevance.test.mjs`: opt-in live relevance test.
- `atmos-tools.crawler.dashboard.js`: paste-ready dashboard fallback.

### Config Deployment and Reindex

`.github/workflows/algolia.yaml` validates the crawler config on pull requests and, after every successful production website deploy, deploys the crawler config + index settings AND triggers a content reindex — in that order.

- Pull requests run `pnpm run test:algolia` and `pnpm run algolia:deploy:dry-run`. No Algolia secrets are used.
- The `deploy` job is triggered by GitHub Actions `workflow_run` on completion of `Website Deploy Prod`, gated on `workflow_run.conclusion == 'success'`. This guarantees the site on S3 is fresh **before** any Algolia work begins.
- The `deploy` job runs in the `algolia` GitHub environment. It PATCHes the crawler config, PUTs the index settings, and then POSTs the reindex — all from a single Node script (`deploy-crawler-config.mjs`) so the reindex cannot race the config update.
- `workflow_dispatch` is available to manually re-run the deploy from `main` only. Dispatches from pull request branches still run validation, but skip deploy and do not access Algolia secrets.

This split keeps AWS production deployment credentials in the `production` environment and Algolia crawler credentials in the `algolia` environment, while still enforcing strict ordering across the two workflows.

### Frontend Search

`website/docusaurus.config.js` configures the DocSearch frontend:

```javascript
algolia: {
    appId: process.env.ALGOLIA_APP_ID || '32YOERUX83',
    apiKey: process.env.ALGOLIA_SEARCH_API_KEY || '557985309adf0e4df9dcf3cb29c61928',
    indexName: process.env.ALGOLIA_INDEX_NAME || 'atmos.tools',
    contextualSearch: false,
    askAi: {
        assistantId: process.env.ALGOLIA_ASKAI_ASSISTANT_ID || 'xzgtsIXZSf7V',
        // ... additional Ask AI config
    }
}
```

The frontend key is search-only and is safe to expose.

## GitHub Environment Secrets

Configure these secrets in the `algolia` GitHub environment:

| Secret                             | Description                                                            | Source                           |
| ---------------------------------- | ---------------------------------------------------------------------- | -------------------------------- |
| `ALGOLIA_CRAWLER_ID`               | Crawler UUID used by the config deploy script                          | Algolia Crawler URL              |
| `ALGOLIA_CRAWLER_USER_ID`          | Crawler authentication user ID                                         | Algolia Crawler account settings |
| `ALGOLIA_CRAWLER_API_KEY`          | Crawler authentication API key                                         | Algolia Crawler account settings |
| `ALGOLIA_CRAWLER_INDEXING_API_KEY` | Indexing/Admin API key used to update index settings and write crawler records | Algolia application API keys     |

The indexing/admin API key must have permission to update index settings and write crawler records for the `atmos.tools` index. Do not use a search-only key here.

## Common Commands

Run from `website/`:

```shell
pnpm run test:algolia
pnpm run algolia:deploy:dry-run
ALGOLIA_LIVE_RELEVANCE_TESTS=1 pnpm run test:algolia:live
```

Use the live relevance test only after the crawler config has been deployed and the crawler has completed a reindex.

## Removed Preview Indexing Path

Preview deployments do not run Algolia indexing. The old preview reindex path has been removed, including the `reindex` label behavior, scraper API key, and stale preview reindex script call.

## Troubleshooting

### Search Not Returning Results

1. Check that the `atmos.tools` index exists and has records in Algolia.
2. Review the latest Algolia Crawler logs.
3. Verify `https://atmos.tools/sitemap.xml` is accessible.
4. Use the crawler URL tester to verify selectors and extracted records.

### Config Deployment Failing

1. Verify the `algolia` environment secrets are configured.
2. Run `pnpm run algolia:deploy:dry-run` locally from `website/`.
3. Check whether the failure is from the crawler config API or the index settings API.

### Ranking Changes Not Visible

1. Confirm `.github/workflows/website-deploy-prod.yml` succeeded on the merge commit (site is on S3).
2. Confirm the `Algolia` workflow ran via `workflow_run` after that, with both `validate` and `deploy` green. The `deploy` step both PATCHes the crawler config and POSTs the reindex.
3. Run the opt-in live relevance test from `website/` once the crawler reports the reindex complete.

## References

- [Algolia Crawler Documentation](https://www.algolia.com/doc/tools/crawler/getting-started/overview)
- [Algolia Crawler API](https://www.algolia.com/doc/rest-api/crawler/)
- [DocSearch Documentation](https://docsearch.algolia.com/)
- [Docusaurus Search Documentation](https://docusaurus.io/docs/search)
