import crawler from "./atmos-tools.crawler.js";

const { createAtmosToolsCrawlerConfig } = crawler;

const CRAWLER_API_BASE_URL = "https://crawler.algolia.com/api/1";
const TRUE_VALUES = new Set(["1", "true", "yes"]);

const dryRun = process.argv.includes("--dry-run");
const reindexRequested =
  process.argv.includes("--reindex") ||
  TRUE_VALUES.has(String(process.env.ALGOLIA_CRAWLER_REINDEX).toLowerCase());

function requireEnv(name) {
  const value = process.env[name];

  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }

  return value;
}

function serializeCrawlerConfig(value) {
  if (typeof value === "function") {
    return {
      __type: "function",
      source: value.toString(),
    };
  }

  if (Array.isArray(value)) {
    return value.map(serializeCrawlerConfig);
  }

  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, nestedValue]) => [
        key,
        serializeCrawlerConfig(nestedValue),
      ]),
    );
  }

  return value;
}

function redactSecrets(value) {
  if (Array.isArray(value)) {
    return value.map(redactSecrets);
  }

  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, nestedValue]) => [
        key,
        key.toLowerCase().includes("apikey") || key.toLowerCase() === "apikey"
          ? "<redacted>"
          : redactSecrets(nestedValue),
      ]),
    );
  }

  return value;
}

async function requestJson(label, url, options) {
  const response = await fetch(url, options);
  const body = await response.text();
  const data = body ? JSON.parse(body) : {};

  if (!response.ok) {
    throw new Error(`${label} failed with ${response.status}: ${body}`);
  }

  return data;
}

function getCrawlerAuthHeader() {
  const crawlerUserId = requireEnv("ALGOLIA_CRAWLER_USER_ID");
  const crawlerApiKey = requireEnv("ALGOLIA_CRAWLER_API_KEY");

  return `Basic ${Buffer.from(`${crawlerUserId}:${crawlerApiKey}`).toString("base64")}`;
}

async function main() {
  const apiKey = dryRun
    ? undefined
    : requireEnv("ALGOLIA_CRAWLER_INDEXING_API_KEY");
  const config = createAtmosToolsCrawlerConfig({ apiKey });
  const serializedConfig = serializeCrawlerConfig(config);
  const indexName = config.actions[0].indexName;
  const indexSettings = config.initialIndexSettings[indexName];

  if (dryRun) {
    console.log(JSON.stringify(redactSecrets(serializedConfig), null, 2));
    return;
  }

  const crawlerId = requireEnv("ALGOLIA_CRAWLER_ID");
  const indexingApiKey = requireEnv("ALGOLIA_CRAWLER_INDEXING_API_KEY");
  const crawlerAuthorization = getCrawlerAuthHeader();

  const crawlerResult = await requestJson(
    "Updating crawler config",
    `${CRAWLER_API_BASE_URL}/crawlers/${crawlerId}/config`,
    {
      method: "PATCH",
      headers: {
        Authorization: crawlerAuthorization,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializedConfig),
    },
  );

  const settingsResult = await requestJson(
    "Updating index settings",
    `https://${config.appId}.algolia.net/1/indexes/${encodeURIComponent(indexName)}/settings`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        "X-Algolia-API-Key": indexingApiKey,
        "X-Algolia-Application-Id": config.appId,
      },
      body: JSON.stringify(indexSettings),
    },
  );

  console.log(`Updated crawler config for ${crawlerId}.`);
  console.log(`Updated index settings for ${indexName}.`);
  console.log(`Crawler task: ${crawlerResult.taskId || "not provided"}`);
  console.log(
    `Settings task: ${
      settingsResult.taskID || settingsResult.taskId || "not provided"
    }`,
  );

  if (reindexRequested) {
    const reindexResult = await requestJson(
      "Starting crawler reindex",
      `${CRAWLER_API_BASE_URL}/crawlers/${crawlerId}/reindex`,
      {
        method: "POST",
        headers: {
          Authorization: crawlerAuthorization,
        },
      },
    );

    console.log(
      `Started crawler reindex task: ${reindexResult.taskId || "not provided"}`,
    );
  }
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
