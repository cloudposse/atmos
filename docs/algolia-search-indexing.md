# Algolia Search Indexing

This document describes how Algolia search indexing is configured for the atmos.tools documentation site.

## Overview

The atmos.tools documentation uses [Algolia DocSearch](https://docsearch.algolia.com/) for search functionality. Search indexing is managed via the **Algolia Crawler**, which is triggered automatically on every deployment.

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│  GitHub Actions     │────▶│   Algolia Crawler    │────▶│  Algolia Index  │
│  (website-deploy)   │     │   (cloud-hosted)     │     │  (atmos.tools)  │
└─────────────────────┘     └──────────────────────┘     └─────────────────┘
```

### Components

1. **GitHub Actions Workflow** (`.github/workflows/website-deploy-prod.yml`)
   - Triggers the Algolia Crawler after deploying the website to S3.
   - Uses the official `algolia/algoliasearch-crawler-github-actions` action.

2. **Algolia Crawler** (dashboard.algolia.com)
   - Cloud-hosted crawler that fetches and indexes the documentation.
   - Uses the official Docusaurus v3 template for optimal indexing.
   - Runs on-demand (CI-triggered) and weekly (scheduled backup).

3. **Docusaurus Frontend** (`website/docusaurus.config.js`)
   - Integrates with Algolia via the DocSearch plugin.
   - Supports DocSearch v4 with Ask AI integration.

## Configuration

### GitHub Secrets Required

The following secrets must be configured in the GitHub repository:

| Secret | Description | Source |
|--------|-------------|--------|
| `ALGOLIA_CRAWLER_USER_ID` | Crawler authentication user ID | Algolia Crawler > Account Settings |
| `ALGOLIA_CRAWLER_API_KEY` | Crawler authentication API key | Algolia Crawler > Account Settings |
| `ALGOLIA_API_KEY` | Admin API key for writing to index | Algolia Dashboard > API Keys |

### Algolia Crawler Configuration

The crawler is configured in the Algolia dashboard (dashboard.algolia.com) with these settings:

- **Template**: Docusaurus v3
- **Start URL**: `https://atmos.tools/`
- **Sitemap URL**: `https://atmos.tools/sitemap.xml`
- **Index Name**: `atmos.tools`
- **Schedule**: Weekly (as backup to CI-triggered crawls)

### Frontend Configuration

The Docusaurus frontend is configured in `website/docusaurus.config.js`:

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

## Triggering a Crawl

### Automatic (CI/CD) - Primary Method

Crawls are automatically triggered via the `algolia/algoliasearch-crawler-github-actions` GitHub Action on:
- Push to `main` branch
- Release published
- Manual workflow dispatch

This is the primary indexing method. The GitHub Action is configured in `.github/workflows/website-deploy-prod.yml`.

### Manual (Dashboard)

For debugging or one-off reindexing:

1. Log into dashboard.algolia.com.
2. Navigate to the Crawler section.
3. Select the `atmos.tools` crawler.
4. Click "Start crawl" or "Restart crawling".

### Manual (API)

For scripting or automation outside of GitHub Actions:

```bash
curl -X POST "https://crawler.algolia.com/api/1/crawlers/{CRAWLER_ID}/reindex" \
  -H "Authorization: Basic $(echo -n '{USER_ID}:{API_KEY}' | base64)"
```

## Troubleshooting

### Search Not Returning Results

1. **Check index status**: Log into Algolia dashboard and verify the index has records.
2. **Check crawler logs**: Review the crawler run logs for errors.
3. **Verify sitemap**: Ensure `https://atmos.tools/sitemap.xml` is accessible and complete.
4. **Test selectors**: Use the URL Tester in the crawler dashboard to verify content extraction.

### CI Workflow Failing

1. **Verify secrets**: Ensure all required GitHub secrets are configured.
2. **Check credentials**: Verify the Crawler User ID and API Key are correct.
3. **Review action logs**: Check the GitHub Actions logs for specific error messages.

### Low Record Count

If the index has significantly fewer records than expected:

1. **Check sitemap**: Verify all pages are included in the sitemap.
2. **Review crawler config**: Ensure the start URL and sitemap URL are correct.
3. **Check for blocked pages**: Review robots.txt for any blocked paths.
4. **Verify page linking**: Ensure all pages are linked from the sitemap or other indexed pages.

## Migration History

**January 2026**: Migrated from deprecated `algolia/docsearch-scraper` Docker image to the official Algolia Crawler with GitHub Actions integration. The legacy scraper was deprecated in February 2022 and was causing indexing failures.

## References

- [Algolia Crawler Documentation](https://www.algolia.com/doc/tools/crawler/getting-started/overview/)
- [Algolia Crawler GitHub Action](https://github.com/algolia/algoliasearch-crawler-github-actions)
- [DocSearch Documentation](https://docsearch.algolia.com/)
- [Docusaurus Search Documentation](https://docusaurus.io/docs/search)
