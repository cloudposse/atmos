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

### Config Deployment

`.github/workflows/algolia.yaml` validates and deploys crawler configuration.

- Pull requests run `pnpm run test:algolia` and `pnpm run algolia:deploy:dry-run`.
- Pull requests do not upload crawler config or use Algolia write secrets.
- Pushes to `main` deploy crawler config and index settings from the `algolia` GitHub environment.

### Production Content Reindex

`.github/workflows/website-deploy-prod.yml` deploys the website to S3 from the `production` environment. After that job succeeds, a separate `algolia-reindex-prod` job runs in the `algolia` environment and triggers the Algolia Crawler for `https://atmos.tools`.

This split keeps AWS production deployment credentials in the `production` environment and Algolia crawler credentials in the `algolia` environment.

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

1. Confirm `.github/workflows/algolia.yaml` deployed the crawler config and index settings after merge.
2. Confirm `.github/workflows/website-deploy-prod.yml` completed the `algolia-reindex-prod` job.
3. Run the opt-in live relevance test from `website/`.

## References

- [Algolia Crawler Documentation](https://www.algolia.com/doc/tools/crawler/getting-started/overview)
- [Algolia Crawler API](https://www.algolia.com/doc/rest-api/crawler/)
- [DocSearch Documentation](https://docsearch.algolia.com/)
- [Docusaurus Search Documentation](https://docusaurus.io/docs/search)
