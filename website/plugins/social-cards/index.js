'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { WIDTH, HEIGHT, renderCardJpeg } = require('./render-card');

const OUTPUT_DIR = path.join('img', 'social');
// Bump when the card design changes so social crawlers receive a new image URL.
const DESIGN_VERSION = '3';
const DEFAULT_SUBTITLE = 'The runtime for infrastructure';

// First path segment -> label shown in the card's accent line.
const SECTION_LABELS = {
  ai: 'Atmos AI',
  blog: 'Changelog',
  changelog: 'Changelog',
  ci: 'Atmos CI',
  cli: 'CLI Reference',
  'core-concepts': 'Core Concepts',
  deprecated: 'Deprecated',
  examples: 'Examples',
  experimental: 'Experimental',
  functions: 'YAML & Template Functions',
  gists: 'Gists',
  learn: 'Learn',
};

/** Decode the HTML entities emitted in Docusaurus metadata. */
const decodeEntities = (text) =>
  text
    .replace(/&quot;/g, '"')
    .replace(/&#x27;|&#39;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');

// Match complete tags without treating a quoted greater-than sign as their end.
const META_TAG = /<meta\b(?:[^"'<>]|"[^"]*"|'[^']*')*>/gi;

/** Read attributes and their value offsets so updates preserve the rest of a tag. */
function metaAttributes(tag) {
  const attributes = new Map();
  const pattern = /([^\s=<>/'"]+)(\s*=\s*)("[^"]*"|'[^']*'|[^\s"'=<>`]+)/g;
  for (const match of tag.matchAll(pattern)) {
    const raw = match[3];
    const quoted = raw.startsWith('"') || raw.startsWith("'");
    const start = match.index + match[1].length + match[2].length;
    attributes.set(match[1].toLowerCase(), {
      value: decodeEntities(quoted ? raw.slice(1, -1) : raw),
      start,
      end: start + raw.length,
    });
  }
  return attributes;
}

/** Find a meta value independently of attribute order, quoting, or HTML case. */
function metaContent(html, attr, name) {
  for (const [tag] of html.matchAll(META_TAG)) {
    const attributes = metaAttributes(tag);
    if (attributes.get(attr)?.value.toLowerCase() === name) {
      return attributes.get('content')?.value || '';
    }
  }
  return '';
}

/**
 * True when the subtitle only restates the title, e.g. the "Atmos CI" label on
 * the page titled "Native CI". Brand words are ignored so "Atmos" alone never
 * counts as new information.
 *
 * @param {string} title
 * @param {string} subtitle
 * @returns {boolean}
 */
function isRedundantSubtitle(title, subtitle) {
  const words = (text) => text.toLowerCase().split(/[^a-z0-9]+/).filter(Boolean);
  const titleWords = new Set(words(title));
  const significant = words(subtitle).filter((word) => word !== 'atmos');
  return significant.length === 0 || significant.every((word) => titleWords.has(word));
}

/**
 * Derive the card content for a built page from its meta tags and route.
 *
 * @param {string} html Built page HTML.
 * @param {string} route Route such as "/ci" or "/changelog/native-ci-integration".
 * @param {string} host Site host shown in the card footer, e.g. "atmos.tools".
 * @returns {{title: string, subtitle: string, description: string, url: string, brand: 'atmos'|'ci'}|null}
 */
function cardForPage(html, route, host) {
  const rawTitle = metaContent(html, 'property', 'og:title');
  if (!rawTitle) return null;
  const title = rawTitle.replace(/\s*\|\s*atmos$/i, '').trim();
  const section = route.split('/').filter(Boolean)[0];
  const label = Object.hasOwn(SECTION_LABELS, section) ? SECTION_LABELS[section] : DEFAULT_SUBTITLE;
  return {
    title,
    brand: section === 'ci' ? 'ci' : 'atmos',
    subtitle: isRedundantSubtitle(title, label) ? '' : label,
    description: metaContent(html, 'property', 'og:description'),
    url: `${host}${route}`,
  };
}

/**
 * Point og:image / twitter:image at the per-page card and synchronize dimensions.
 *
 * @param {string} html
 * @param {string} imageUrl Absolute URL of the card.
 * @returns {string}
 */
function rewriteImageMeta(html, imageUrl) {
  const encodedUrl = imageUrl.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
  const sizes = new Map([['og:image:width', String(WIDTH)], ['og:image:height', String(HEIGHT)]]);
  const properties = new Set([...html.matchAll(META_TAG)]
    .map(([tag]) => metaAttributes(tag).get('property')?.value.toLowerCase()));
  let dimensions = [...sizes]
    .filter(([property]) => !properties.has(property))
    .map(([property, size]) => `<meta data-rh="true" property="${property}" content="${size}">`)
    .join('');
  return html.replace(META_TAG, (tag) => {
    const attributes = metaAttributes(tag);
    const property = attributes.get('property')?.value.toLowerCase();
    const isOgImage = property === 'og:image';
    const isTwitterImage = attributes.get('name')?.value.toLowerCase() === 'twitter:image';
    const value = isOgImage || isTwitterImage ? encodedUrl : sizes.get(property);
    if (value === undefined) return tag;
    const content = attributes.get('content');
    let updated = content
      ? `${tag.slice(0, content.start)}"${value}"${tag.slice(content.end)}`
      : tag.replace(/\/?>$/, (end) => ` content="${value}"${end}`);
    if (isOgImage) {
      updated += dimensions;
      dimensions = '';
    }
    return updated;
  });
}

/** Walk generated HTML pages recursively without loading the entire tree. */
function* htmlFiles(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* htmlFiles(full);
    else if (entry.name.endsWith('.html')) yield full;
  }
}

/** Generate per-page JPEG cards and link them from the built social metadata. */
module.exports = function socialCardsPlugin() {
  return {
    name: 'social-cards',
    async postBuild({ outDir, siteConfig }) {
      const baseUrl = `${siteConfig.url}${siteConfig.baseUrl}`.replace(/\/?$/, '/');
      const host = new URL(siteConfig.url).host;
      const cardsDir = path.join(outDir, OUTPUT_DIR);
      fs.mkdirSync(cardsDir, { recursive: true });

      const written = new Set();
      let pages = 0;
      for (const file of htmlFiles(outDir)) {
        const html = fs.readFileSync(file, 'utf8');
        const route = `/${path.relative(outDir, file).split(path.sep).join('/')}`.replace(
          /\/?index\.html$/,
          '',
        );
        const card = cardForPage(html, route, host);
        if (!card || !metaContent(html, 'property', 'og:image')) continue;

        // Pages sharing title + subtitle + description + URL share one card.
        const hash = crypto
          .createHash('sha256')
          .update(JSON.stringify([DESIGN_VERSION, card.title, card.subtitle, card.description, card.url, card.brand]))
          .digest('hex')
          .slice(0, 16);
        if (!written.has(hash)) {
          fs.writeFileSync(path.join(cardsDir, `${hash}.jpg`), renderCardJpeg(card));
          written.add(hash);
        }
        const imageUrl = `${baseUrl}${OUTPUT_DIR.split(path.sep).join('/')}/${hash}.jpg`;
        fs.writeFileSync(file, rewriteImageMeta(html, imageUrl));
        pages += 1;
      }
      console.log(`[social-cards] Wrote ${written.size} cards for ${pages} pages`);
    },
  };
};

module.exports.cardForPage = cardForPage;
module.exports.isRedundantSubtitle = isRedundantSubtitle;
module.exports.rewriteImageMeta = rewriteImageMeta;
