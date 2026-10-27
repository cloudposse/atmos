const fs = require('node:fs');
const path = require('node:path');
const matter = require('gray-matter');

const journalPath = path.resolve(__dirname, '../../../pkg/edition/journal.yaml');
const fields = ['date', 'key', 'kind', 'old', 'new', 'description', 'ref'];

function readJournal(file = journalPath) {
  const entries = matter.engines.yaml.parse(fs.readFileSync(file, 'utf8'));
  if (!Array.isArray(entries) || entries.length === 0) throw new Error('Edition journal must be a nonempty list');
  const seen = new Set();
  for (const entry of entries) {
    if (!entry || fields.some(key => !Object.hasOwn(entry, key)) || Object.keys(entry).some(key => !fields.includes(key))) {
      throw new Error('Edition journal entry has missing or unknown fields');
    }
    if (typeof entry.date !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(entry.date)
        || !Number.isFinite(Date.parse(entry.date)) || new Date(entry.date).toISOString().slice(0, 10) !== entry.date) {
      throw new Error('Edition dates must be quoted YYYY-MM-DD strings');
    }
    if (!['value', 'behavior'].includes(entry.kind)) throw new Error('Unknown edition kind');
    for (const key of ['key', 'description', 'ref']) {
      if (typeof entry[key] !== 'string' || !entry[key].trim()) throw new Error(`Edition ${key} must be a nonempty string`);
    }
    for (const key of ['old', 'new']) {
      if (!['string', 'boolean', 'number'].includes(typeof entry[key])
          || (typeof entry[key] === 'number' && !Number.isSafeInteger(entry[key]))) {
        throw new Error('Edition values must be strings, booleans, or safe integers');
      }
    }
    if (!/^https:\/\/[^\s<>]+$/.test(entry.ref)) throw new Error('Edition source must be an HTTPS URL');
    const id = `${entry.date}/${entry.key}`;
    if (seen.has(id)) throw new Error(`Duplicate edition transition: ${id}`);
    seen.add(id);
  }
  return entries.sort((a, b) => b.date.localeCompare(a.date) || a.key.localeCompare(b.key));
}

function prose(value) {
  return value.replace(/\r?\n/g, ' ').replace(/[\\`*_{}[\]<>|]/g, '\\$&');
}

function code(value, quoteStrings = true) {
  const text = (quoteStrings ? JSON.stringify(value) : value).replace(/\|/g, '\\|');
  const fence = '`'.repeat(Math.max(0, ...Array.from(text.matchAll(/`+/g), match => match[0].length)) + 1);
  return `${fence} ${text} ${fence}`;
}

function renderIndex(entries) {
  const lines = [
    '---',
    'title: Configuration Edition History',
    'sidebar_label: Editions',
    'description: Browse every journaled Atmos default and behavior change by date.',
    'slug: /cli/editions',
    'toc_max_heading_level: 2',
    'release_source: ../pkg/edition/journal.yaml',
    'custom_edit_url: https://github.com/cloudposse/atmos/edit/main/pkg/edition/journal.yaml',
    '---',
    '',
    'Atmos occasionally changes its configuration defaults. By default, upgrading Atmos adopts those newer defaults.',
    'A **configuration edition** lets you pin your project to a specific date, preserving the defaults from that date',
    'until you choose to adopt newer ones by advancing the pin.',
    '',
    'Use [`atmos config set`](/cli/commands/config/config-set) to save your chosen edition in `atmos.yaml`.',
    'For example, to keep the defaults as of October 25, 2026:',
    '',
    '```shell',
    'atmos config set edition 2026-10-25',
    '```',
    '',
    'Set a later date when you are ready to adopt newer defaults. Values you explicitly configure take precedence over edition defaults.',
    '',
    '- [Configure your edition](/cli/configuration/edition) — date formats, precedence, and how pinning works.',
    '- [`atmos list editions`](/cli/commands/list/editions) — list changes or compare dates with `--from` and `--to`.',
    '- [`atmos describe edition`](/cli/commands/describe/edition) — inspect your active edition and the defaults it preserves.',
    '',
    'The history below lists every recorded default and behavior change. Your installed Atmos version includes the entries available when it was built.',
    'Quoted values are strings; unquoted values are booleans or numbers.',
    '',
  ];
  let date;
  for (const entry of entries) {
    if (entry.date !== date) {
      date = entry.date;
      lines.push('', `## ${date}`, '');
    }
    lines.push(`### ${code(entry.key, false)}`, '',
      entry.kind === 'value' ? '**Default change.**' : '**Behavior transition.**', '',
      prose(entry.description), '',
      '| Before | After |', '| --- | --- |',
      `| ${code(entry.old)} | ${code(entry.new)} |`, '',
      `[Source](<${entry.ref}>)`, '');
  }
  return `${lines.join('\n')}\n`;
}

function generateEditionIndex(siteDir) {
  const output = path.join(siteDir, 'docs/cli/editions.mdx');
  const content = renderIndex(readJournal());
  fs.mkdirSync(path.dirname(output), {recursive: true});
  if (!fs.existsSync(output) || fs.readFileSync(output, 'utf8') !== content) fs.writeFileSync(output, content);
  return output;
}

module.exports = {readJournal, renderIndex, generateEditionIndex};
