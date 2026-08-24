# Atmos Algolia Crawler

This directory contains the repo-managed crawler configuration for the `atmos.tools` DocSearch index.

## Pull requests

Pull requests only validate the crawler configuration. They run:

```shell
pnpm run test:algolia
pnpm run algolia:deploy:dry-run
```

The dry run serializes and redacts the crawler payload. It does not require Algolia secrets and does not upload anything.

## Main deployment

After changes merge to `main`, `.github/workflows/algolia.yaml` deploys from the GitHub Actions environment named `algolia`.

Configure these environment secrets in `algolia`:

- `ALGOLIA_CRAWLER_ID`: crawler UUID from the crawler URL.
- `ALGOLIA_CRAWLER_USER_ID`: crawler API user ID from Crawler settings.
- `ALGOLIA_CRAWLER_API_KEY`: crawler API key from Crawler settings.
- `ALGOLIA_CRAWLER_INDEXING_API_KEY`: Algolia Search API key used by the crawler to write `atmos.tools`.

The deploy script updates both the crawler configuration and the existing index settings. This matters because Algolia Crawler `initialIndexSettings` are only applied automatically when an index is first created.

Use `algolia:deploy -- --reindex` or set `ALGOLIA_CRAWLER_REINDEX=true` to start a reindex after deploying the config. The workflow enables reindexing on `main`.

## Manual dashboard paste

Use `atmos-tools.crawler.dashboard.js` when you need to paste the crawler config into the Algolia Crawler editor by hand. Replace `PASTE_INDEXING_API_KEY_HERE` with the crawler indexing key from the existing Crawler config before saving.

Manual dashboard paste applies extractor changes such as URL taxonomy, canonical URL handling, definition-term weighting, and `pageRank` records. It does not update `customRanking` for an existing index unless you also update the `atmos.tools` index settings in Algolia.

## Live relevance test

`pnpm run test:algolia:live` queries the live `atmos.tools` index and fails unless:

- `atmos auth` returns an Atmos Auth command page as the top result.
- Auth config slash/non-slash duplicate URLs are gone from the top results.

Run the live test after saving the crawler config and completing a reindex. It will fail against the old live index by design.
