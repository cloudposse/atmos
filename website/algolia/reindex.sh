#!/usr/bin/env bash

ALGOLIA_INDEX_NAME=${ALGOLIA_INDEX_NAME:-test}
DEPLOYMENT_HOST=${DEPLOYMENT_HOST:-'atmos.tools'}
ALGOLIA_APP_ID=${ALGOLIA_APP_ID:-'32YOERUX83'}

[ -z "$ALGOLIA_SCRAPER_API_KEY" ] && echo "Need to set ALGOLIA_SCRAPER_API_KEY" && exit 1;

# prepare algolia config
cat website/algolia/template.json \
  | jq '.index_name="'${ALGOLIA_INDEX_NAME}'"' \
  | jq '.start_urls[0]="https://'${DEPLOYMENT_HOST}'/"' \
  | jq '.sitemap_urls[0]="https://'${DEPLOYMENT_HOST}'/sitemap.xml"' \
  > algolia.index.json

cat algolia.index.json

# do actual scraping
docker run \
  --env APPLICATION_ID="${ALGOLIA_APP_ID}" \
  --env API_KEY="${ALGOLIA_SCRAPER_API_KEY}" \
  --env "CONFIG=$(cat algolia.index.json | jq -r tostring)" \
  algolia/docsearch-scraper
