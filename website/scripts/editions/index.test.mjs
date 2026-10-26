import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {compile} from '@mdx-js/mdx';
import generator from './index.cjs';

const {readJournal, renderIndex, generateEditionIndex} = generator;

function temporaryDirectory(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'atmos-editions-'));
  t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
  return directory;
}

test('the public index includes every canonical transition with typed values and source links', async t => {
  const entries = readJournal();
  const page = fs.readFileSync(generateEditionIndex(temporaryDirectory(t)), 'utf8');
  for (const entry of entries) {
    assert.ok(page.includes(entry.key), entry.key);
    assert.ok(page.includes(`## ${entry.date}`), entry.date);
    assert.ok(page.includes(entry.ref), entry.ref);
  }
  assert.equal(page.split('\n').filter(line => line.startsWith('| `')).length, entries.length);
  assert.ok(page.indexOf('## 2026-10-26') < page.indexOf('## 2025-02-11'));
  assert.match(page, /\| ` "true" ` \| ` "false" ` \|/);
  assert.match(page, /\| ` false ` \| ` true ` \|/);
  assert.match(page, /\| ` 1 ` \| ` 4 ` \|/);
  await compile(page.replace(/^---\n[\s\S]*?\n---\n/, ''));
});

test('journal edits are read again rather than cached across builds', t => {
  const file = path.join(temporaryDirectory(t), 'journal.yaml');
  const entry = readJournal()[0];
  fs.writeFileSync(file, JSON.stringify([entry]));
  assert.equal(readJournal(file)[0].description, entry.description);
  fs.writeFileSync(file, JSON.stringify([{...entry, description: 'Updated description'}]));
  assert.match(renderIndex(readJournal(file)), /Updated description/);
});

test('invalid journal entries fail rather than publishing incomplete history', t => {
  const file = path.join(temporaryDirectory(t), 'journal.yaml');
  const entry = readJournal()[0];
  for (const entries of [[], [{...entry, date: '2026-02-31'}], [{...entry, kind: 'unknown'}],
    [{...entry, ref: 'javascript:alert(1)'}], [{...entry, old: {unexpected: true}}],
    [{...entry, extra: 'typo'}], [entry, entry]]) {
    fs.writeFileSync(file, JSON.stringify(entries));
    assert.throws(() => readJournal(file));
  }
});

test('template braces and markdown characters remain literal in generated MDX', async () => {
  const entry = {...readJournal()[0], description: 'Use {{ .value }} | <tag> [text] `literal`', old: '`a` | {{ .x }}', new: 'done'};
  const source = renderIndex([entry]).replace(/^---\n[\s\S]*?\n---\n/, '');
  await compile(source);
  assert.ok(source.includes('\\{\\{'));
  assert.ok(source.includes('\\|'));
});
