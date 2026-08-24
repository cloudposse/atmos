import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Resvg } from "@resvg/resvg-js";
import jpeg from "jpeg-js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, "..");
const staticDir = path.join(rootDir, "static");
const outputDir = path.join(staticDir, "downloads");
const rasterDir = path.join(outputDir, "media-kit");
const outputPath = path.join(outputDir, "atmos-media-kit.zip");

const assets = [
  {
    source: "img/atmos-logo.svg",
    destinationBase: "atmos-mark-color",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/atmos-logo-bw.svg",
    destinationBase: "atmos-mark-monochrome",
    jpgBackground: "#171717",
  },
  {
    source: "img/atmos-docs-logo-dark.svg",
    destinationBase: "atmos-wordmark-dark",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/atmos-docs-logo-light.svg",
    destinationBase: "atmos-wordmark-light",
    jpgBackground: "#171717",
  },
  {
    source: "img/atmos-logo-gradient.svg",
    destinationBase: "atmos-logo-gradient",
    jpgBackground: "#171717",
  },
  {
    source: "img/atmos-logo-gradient-on-light.svg",
    destinationBase: "atmos-logo-gradient-on-light",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/atmos-ci-gradient.svg",
    destinationBase: "atmos-ci-gradient",
    jpgBackground: "#171717",
  },
  {
    source: "img/atmos-ci-gradient-on-light.svg",
    destinationBase: "atmos-ci-gradient-on-light",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/atmos-ci-lockup-gradient.svg",
    destinationBase: "atmos-ci-lockup-gradient",
    jpgBackground: "#171717",
  },
  {
    source: "img/atmos-ci-lockup-gradient-on-light.svg",
    destinationBase: "atmos-ci-lockup-gradient-on-light",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/atmos-ai-gradient.svg",
    destinationBase: "atmos-ai-gradient",
    jpgBackground: "#171717",
  },
  {
    source: "img/atmos-ai-gradient-on-light.svg",
    destinationBase: "atmos-ai-gradient-on-light",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/powered-by-atmos-dark.svg",
    destinationBase: "powered-by-atmos-dark",
    jpgBackground: "#ffffff",
  },
  {
    source: "img/powered-by-atmos-light.svg",
    destinationBase: "powered-by-atmos-light",
    jpgBackground: "#171717",
  },
  {
    source: "img/powered-by-atmos-gradient.svg",
    destinationBase: "powered-by-atmos-gradient",
    jpgBackground: "#171717",
  },
  {
    source: "img/powered-by-atmos-gradient-on-light.svg",
    destinationBase: "powered-by-atmos-gradient-on-light",
    jpgBackground: "#ffffff",
  },
];

// Render rasters at 2x the SVG's intrinsic size for crisp exports.
const rasterZoom = 2;

const crcTable = new Uint32Array(256);
for (let i = 0; i < 256; i += 1) {
  let value = i;
  for (let bit = 0; bit < 8; bit += 1) {
    value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
  }
  crcTable[i] = value >>> 0;
}

function crc32(buffer) {
  let crc = 0xffffffff;
  for (const byte of buffer) {
    crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function dosDateTime(date = new Date()) {
  const year = Math.max(date.getFullYear(), 1980);
  const dosTime =
    (date.getHours() << 11) |
    (date.getMinutes() << 5) |
    (date.getSeconds() >> 1);
  const dosDate =
    ((year - 1980) << 9) | ((date.getMonth() + 1) << 5) | date.getDate();
  return { dosDate, dosTime };
}

function uint16(value) {
  const buffer = Buffer.alloc(2);
  buffer.writeUInt16LE(value);
  return buffer;
}

function uint32(value) {
  const buffer = Buffer.alloc(4);
  buffer.writeUInt32LE(value);
  return buffer;
}

function createZip(entries) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;
  const { dosDate, dosTime } = dosDateTime();

  for (const entry of entries) {
    const fileName = Buffer.from(entry.name, "utf8");
    const data = entry.data;
    const checksum = crc32(data);
    const size = data.length;

    const localHeader = Buffer.concat([
      uint32(0x04034b50),
      uint16(20),
      uint16(0x0800),
      uint16(0),
      uint16(dosTime),
      uint16(dosDate),
      uint32(checksum),
      uint32(size),
      uint32(size),
      uint16(fileName.length),
      uint16(0),
      fileName,
    ]);

    localParts.push(localHeader, data);

    centralParts.push(
      Buffer.concat([
        uint32(0x02014b50),
        uint16(20),
        uint16(20),
        uint16(0x0800),
        uint16(0),
        uint16(dosTime),
        uint16(dosDate),
        uint32(checksum),
        uint32(size),
        uint32(size),
        uint16(fileName.length),
        uint16(0),
        uint16(0),
        uint16(0),
        uint16(0),
        uint32(0),
        uint32(offset),
        fileName,
      ]),
    );

    offset += localHeader.length + data.length;
  }

  const centralDirectory = Buffer.concat(centralParts);
  const endOfCentralDirectory = Buffer.concat([
    uint32(0x06054b50),
    uint16(0),
    uint16(0),
    uint16(entries.length),
    uint16(entries.length),
    uint32(centralDirectory.length),
    uint32(offset),
    uint16(0),
  ]);

  return Buffer.concat([
    ...localParts,
    centralDirectory,
    endOfCentralDirectory,
  ]);
}

mkdirSync(outputDir, { recursive: true });
rmSync(rasterDir, { force: true, recursive: true });
mkdirSync(rasterDir, { recursive: true });

function rasterizeAsset(asset) {
  const sourcePath = path.join(staticDir, asset.source);
  const svg = readFileSync(sourcePath);
  const svgString = svg.toString("utf8");

  // PNG keeps a transparent background.
  const png = new Resvg(svgString, {
    fitTo: { mode: "zoom", value: rasterZoom },
    font: { loadSystemFonts: true },
  })
    .render()
    .asPng();

  // JPEG has no alpha channel, so composite the artwork over the asset's
  // background color, then encode the resulting RGBA pixmap.
  const rendered = new Resvg(svgString, {
    fitTo: { mode: "zoom", value: rasterZoom },
    font: { loadSystemFonts: true },
    background: asset.jpgBackground,
  }).render();
  const jpg = jpeg.encode(
    {
      data: Buffer.from(rendered.pixels),
      width: rendered.width,
      height: rendered.height,
    },
    92,
  ).data;

  writeFileSync(path.join(rasterDir, `${asset.destinationBase}.png`), png);
  writeFileSync(path.join(rasterDir, `${asset.destinationBase}.jpg`), jpg);

  return [
    {
      name: `atmos-media-kit/${asset.destinationBase}.svg`,
      data: svg,
    },
    {
      name: `atmos-media-kit/${asset.destinationBase}.png`,
      data: png,
    },
    {
      name: `atmos-media-kit/${asset.destinationBase}.jpg`,
      data: jpg,
    },
  ];
}

const entries = assets.map(rasterizeAsset).flat();

writeFileSync(outputPath, createZip(entries));
console.log(
  `Generated ${path.relative(rootDir, outputPath)} and ${path.relative(rootDir, rasterDir)}`,
);
