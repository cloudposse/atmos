import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const staticDir = path.join(rootDir, 'static');
const outputDir = path.join(staticDir, 'downloads');
const outputPath = path.join(outputDir, 'atmos-media-kit.zip');

const files = [
  {
    source: 'img/atmos-logo.svg',
    destination: 'atmos-media-kit/atmos-mark-color.svg',
  },
  {
    source: 'img/atmos-logo-bw.svg',
    destination: 'atmos-media-kit/atmos-mark-monochrome.svg',
  },
  {
    source: 'img/atmos-logo.png',
    destination: 'atmos-media-kit/atmos-mark-color.png',
  },
  {
    source: 'img/atmos-docs-logo-dark.svg',
    destination: 'atmos-media-kit/atmos-wordmark-dark.svg',
  },
  {
    source: 'img/atmos-docs-logo-light.svg',
    destination: 'atmos-media-kit/atmos-wordmark-light.svg',
  },
  {
    source: 'img/atmos-logo-gradient.svg',
    destination: 'atmos-media-kit/atmos-logo-gradient.svg',
  },
  {
    source: 'img/atmos-logo-gradient-on-light.svg',
    destination: 'atmos-media-kit/atmos-logo-gradient-on-light.svg',
  },
  {
    source: 'img/atmos-ci-gradient.svg',
    destination: 'atmos-media-kit/atmos-ci-gradient.svg',
  },
  {
    source: 'img/atmos-ci-gradient-on-light.svg',
    destination: 'atmos-media-kit/atmos-ci-gradient-on-light.svg',
  },
  {
    source: 'img/atmos-ai-gradient.svg',
    destination: 'atmos-media-kit/atmos-ai-gradient.svg',
  },
  {
    source: 'img/atmos-ai-gradient-on-light.svg',
    destination: 'atmos-media-kit/atmos-ai-gradient-on-light.svg',
  },
  {
    source: 'img/powered-by-atmos-gradient.svg',
    destination: 'atmos-media-kit/powered-by-atmos-gradient.svg',
  },
];

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
    (date.getHours() << 11) | (date.getMinutes() << 5) | (date.getSeconds() >> 1);
  const dosDate = ((year - 1980) << 9) | ((date.getMonth() + 1) << 5) | date.getDate();
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
    const fileName = Buffer.from(entry.name, 'utf8');
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

  return Buffer.concat([...localParts, centralDirectory, endOfCentralDirectory]);
}

mkdirSync(outputDir, { recursive: true });

const entries = files.map((file) => ({
  name: file.destination,
  data: readFileSync(path.join(staticDir, file.source)),
}));

writeFileSync(outputPath, createZip(entries));
console.log(`Generated ${path.relative(rootDir, outputPath)}`);
