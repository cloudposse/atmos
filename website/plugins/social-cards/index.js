'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { renderCardJpeg } = require('./render-card');

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

const decodeEntities = (text) =>
  text
    .replace(/&quot;/g, '"')
    .replace(/&#x27;|&#39;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');

function metaContent(html, attr, name) {
  const match = html.match(new RegExp(`<meta[^>]*${attr}="${name}"[^>]*content="([^"]*)"`, 'i'));
  return match ? decodeEntities(match[1]) : '';
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
  const label = SECTION_LABELS[section] || DEFAULT_SUBTITLE;
  return {
    title,
    brand: section === 'ci' ? 'ci' : 'atmos',
    subtitle: isRedundantSubtitle(title, label) ? '' : label,
    description: metaContent(html, 'property', 'og:description'),
    url: `${host}${route}`,
  };
}

/**
 * Point og:image / twitter:image (and add dimensions) at the per-page card.
 *
 * @param {string} html
 * @param {string} imageUrl Absolute URL of the card.
 * @returns {string}
 */
function rewriteImageMeta(html, imageUrl) {
  const replaceContent = (source, attr, name) =>
    source.replace(
      new RegExp(`(<meta[^>]*${attr}="${name}"[^>]*content=")[^"]*(")`, 'i'),
      `$1${imageUrl}$2`,
    );
  let result = replaceContent(html, 'property', 'og:image');
  result = replaceContent(result, 'name', 'twitter:image');
  if (!/property="og:image:width"/.test(result)) {
    result = result.replace(
      /(<meta[^>]*property="og:image"[^>]*>)/i,
      '$1<meta data-rh="true" property="og:image:width" content="1200"><meta data-rh="true" property="og:image:height" content="630">',
    );
  }
  return result;
}

function* htmlFiles(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* htmlFiles(full);
    else if (entry.name.endsWith('.html')) yield full;
  }
}

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
        if (!card || !/property="og:image"/i.test(html)) continue;

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
