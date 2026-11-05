'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { Resvg } = require('@resvg/resvg-js');
const jpeg = require('jpeg-js');

const WIDTH = 1200;
const HEIGHT = 630;
const DOMAIN = 'atmos.tools';
const CONTENT_WIDTH = WIDTH - 144;
const FONT_FILES = ['400Regular', '500Medium', '600SemiBold'];

// Geist is the site font; the TTFs ship in @expo-google-fonts/geist (resvg
// cannot read WOFF/WOFF2).
const fontPackageDir = path.dirname(require.resolve('@expo-google-fonts/geist/package.json'));
const fontFiles = FONT_FILES.map((name) => path.join(fontPackageDir, name, `Geist_${name}.ttf`));
const font = { fontFiles, loadSystemFonts: false, defaultFontFamily: 'Geist' };

// Official lockups frozen with outlined lettering: no substituted brand fonts
// and no dependency on the build machine's installed fonts. Rasterize once at
// high resolution: resvg drops the filtered alien base layer when this SVG is
// nested as an image inside another SVG.
const logos = Object.fromEntries(
  [['atmos', 'atmos-logo-gradient'], ['ci', 'atmos-ci-lockup-gradient']].map(([key, name]) => [
    key,
    new Resvg(fs.readFileSync(path.join(__dirname, 'assets', `${name}.svg`)), {
      font: { loadSystemFonts: false },
      fitTo: { mode: 'zoom', value: 3 },
    }).render().asPng().toString('base64'),
  ]),
);
/** Escape text for SVG text nodes and attributes. */
const escapeXml = (text) =>
  text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

// Use the same text shaping engine and fonts for measurement and rendering.
// Bound the cache because a site build generates thousands of distinct cards.
const widthCache = new Map();
/** Measure the rendered horizontal extent of Geist text in pixels. */
function measureText(text, fontSize = 64, fontWeight = 600) {
  if (!text) return 0;
  const key = `${fontSize}:${fontWeight}:${text}`;
  if (widthCache.has(key)) return widthCache.get(key);
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="200"><text y="100" font-family="Geist" font-size="${fontSize}" font-weight="${fontWeight}" xml:space="preserve">${escapeXml(text)}</text></svg>`;
  const bounds = new Resvg(svg, { font }).getBBox();
  const width = bounds ? bounds.width + Math.max(0, bounds.x) : 0;
  if (widthCache.size >= 4096) widthCache.clear();
  widthCache.set(key, width);
  return width;
}

/** Wrap by rendered width, splitting long tokens and ellipsizing overflow. */
function wrapText(text, maxWidth, maxLines, { fontSize = 64, fontWeight = 600 } = {}) {
  if (!text.trim() || maxLines < 1 || maxWidth <= 0) return [];
  const fits = (value) => measureText(value, fontSize, fontWeight) <= maxWidth;
  const ellipsize = (value) => {
    const chars = Array.from(value.replace(/[\s.,;:]+$/, ''));
    while (chars.length && !fits(`${chars.join('')}…`)) chars.pop();
    return fits('…') ? `${chars.join('')}…` : '';
  };
  const lines = [];
  let current = '';
  for (const word of text.split(/\s+/).filter(Boolean)) {
    const candidate = current ? `${current} ${word}` : word;
    if (fits(candidate)) {
      current = candidate;
      continue;
    }
    if (current) {
      lines.push(current);
      current = '';
    }
    if (lines.length === maxLines) {
      lines[maxLines - 1] = ellipsize(lines[maxLines - 1]);
      return lines;
    }
    let rest = Array.from(word);
    while (!fits(rest.join(''))) {
      // Find the longest fitting prefix without measuring every character.
      let low = 0;
      let high = rest.length;
      while (low < high) {
        const mid = Math.ceil((low + high) / 2);
        if (fits(rest.slice(0, mid).join(''))) low = mid;
        else high = mid - 1;
      }
      if (!low) return [...lines, ellipsize('')];
      const chunk = rest.splice(0, low).join('');
      if (lines.length === maxLines - 1) return [...lines, ellipsize(chunk)];
      lines.push(chunk);
    }
    current = rest.join('');
  }
  if (current) lines.push(current);
  return lines;
}

/**
 * Build the 1200x630 card SVG for a page.
 *
 * @param {{title: string, subtitle?: string, description?: string, url?: string, brand?: 'atmos'|'ci'}} card
 *   `url` is the footer text, e.g. "atmos.tools/ci"; defaults to the bare domain.
 * @returns {string}
 */
function cardSvg({ title, subtitle = '', description = '', url = DOMAIN, brand }) {
  const logo = brand || (subtitle === 'Atmos CI' ? 'ci' : 'atmos');
  const isCi = logo === 'ci';
  // Preserve the official aspect ratio and align the visible mark with the text.
  const lockupSvg = `<image x="57.6" y="35.2" width="${isCi ? 328 : 264}" height="97.6" href="data:image/png;base64,${logos[logo] || logos.atmos}"/>`;
  if (isCi && subtitle === 'Atmos CI') subtitle = '';

  const titleSize = measureText(title, 76) <= CONTENT_WIDTH ? 76 : 72;
  const titleLines = wrapText(title, CONTENT_WIDTH, 3, { fontSize: titleSize });
  const lineHeight = titleSize + 2;
  const titleTop = titleLines.length === 3 ? 210 : 236;
  const titleSvg = titleLines.map((line, i) =>
    `<text x="72" y="${titleTop + i * lineHeight}" font-size="${titleSize}" font-weight="600" fill="url(#metal)">${escapeXml(line)}</text>`,
  ).join('');

  const lastBaseline = titleTop + Math.max(0, titleLines.length - 1) * lineHeight;
  // A softer version of the landing page's metallic stone gradient keeps the
  // final headline line bright at social-preview sizes.
  const metalTop = titleTop - Math.round(titleSize * 0.8);
  const metalBottom = lastBaseline + Math.round(titleSize * 0.25);
  const metalGradient = `<linearGradient id="metal" gradientUnits="userSpaceOnUse" x1="0" y1="${metalTop}" x2="0" y2="${metalBottom}">
      <stop offset="0" stop-color="#ffffff"/><stop offset=".45" stop-color="#f5f5f4"/><stop offset="1" stop-color="#d6d3d1"/>
    </linearGradient>`;

  let y = lastBaseline;
  let subtitleSvg = '';
  if (subtitle) {
    y += 48;
    const line = wrapText(subtitle, CONTENT_WIDTH, 1, { fontSize: 28, fontWeight: 500 })[0] || '';
    subtitleSvg = `<text x="72" y="${y}" font-size="28" font-weight="500" fill="#538ce9">${escapeXml(line)}</text>`;
  }

  const descTop = y + 52;
  // Reserve the footer area even with three headline lines and a subtitle.
  const descMaxLines = Math.min(3, Math.max(0, Math.floor((508 - descTop) / 36) + 1));
  const descLines = wrapText(description, CONTENT_WIDTH, descMaxLines, { fontSize: 27, fontWeight: 400 });
  const descSvg = descLines.map((line, i) =>
    `<text x="72" y="${descTop + i * 36}" font-size="27" fill="#9aa4b2">${escapeXml(line)}</text>`,
  ).join('');
  const footer = wrapText(url, CONTENT_WIDTH, 1, { fontSize: 20, fontWeight: 500 })[0] || '';

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${HEIGHT}" font-family="Geist">
  <defs>
    <!-- Full-bleed site background with a restrained blue glow and a faint cyan glow behind the brand. -->
    <radialGradient id="glow" cx="0.88" cy="1" r="0.95">
      <stop offset="0" stop-color="#3578e5" stop-opacity="0.12"/>
      <stop offset="1" stop-color="#3578e5" stop-opacity="0"/>
    </radialGradient>
    <radialGradient id="brandGlow" cx="0.12" cy="0.08" r="0.6">
      <stop offset="0" stop-color="#2ec8ff" stop-opacity="0.06"/>
      <stop offset="1" stop-color="#2ec8ff" stop-opacity="0"/>
    </radialGradient>
    ${metalGradient}
  </defs>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="#030711"/>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#glow)"/>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#brandGlow)"/>
  ${lockupSvg}
  ${titleSvg}${subtitleSvg}${descSvg}
  <text x="72" y="574" font-size="20" font-weight="500" fill="#9aa4b2">${escapeXml(footer)}</text>
</svg>`;
}

/** Rasterize a card with the bundled fonts for consistent build output. */
function rasterize(card) {
  return new Resvg(cardSvg(card), {
    font,
    fitTo: { mode: 'width', value: WIDTH },
  }).render();
}

/** @returns {Buffer} PNG bytes for a 1200x630 social card. */
function renderCardPng(card) {
  return rasterize(card).asPng();
}

/**
 * JPEG keeps the thousands of per-page cards small.
 * @param {Parameters<typeof cardSvg>[0]} card
 * @param {number} [quality]
 * @returns {Buffer} JPEG bytes.
 */
function renderCardJpeg(card, quality = 75) {
  const rendered = rasterize(card);
  return jpeg.encode(
    { data: rendered.pixels, width: rendered.width, height: rendered.height },
    quality,
  ).data;
}

module.exports = { WIDTH, HEIGHT, measureText, wrapText, cardSvg, renderCardPng, renderCardJpeg };
